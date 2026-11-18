package network

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/backpack/backpack/internal/testacme"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

func controlledACME(t *testing.T, ca *testacme.CAServer, cache string, offline ...bool) (*tls.Config, *autocert.Manager) {
	t.Helper()
	m := &autocert.Manager{Prompt: autocert.AcceptTOS, HostPolicy: autocert.HostWhitelist("tunnel.test"), Cache: autocert.DirCache(cache), Client: &acme.Client{DirectoryURL: ca.URL(), HTTPClient: &http.Client{Timeout: 5 * time.Second}}}
	if len(offline) > 0 && offline[0] {
		m.Client.HTTPClient = &http.Client{Transport: rejectACMETransport{}}
	}
	a := &acmeHTTPResponder{addr: "127.0.0.1:0"}
	ca.ResolveGetCertificate("tunnel.test", m.GetCertificate)
	ca.ResolveHandler("tunnel.test", http.HandlerFunc(a.serve))
	cfg, err := configureACMETLS(TLSSettings{ACMEDomain: "tunnel.test"}, m, a, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		a.mu.Lock()
		ln := a.listener
		a.mu.Unlock()
		if ln != nil {
			ln.Close()
		}
	})
	pinHTTP11ALPN(cfg)
	ca.ResolveGetCertificate("tunnel.test", cfg.GetCertificate)
	ca.ResolveHandler("tunnel.test", http.HandlerFunc(a.serve))
	return cfg, m
}

func verifiedLeaf(t *testing.T, cfg *tls.Config, roots *x509.CertPool, noSNI bool) *x509.Certificate {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	done := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			defer c.Close()
			err = c.(*tls.Conn).Handshake()
		}
		done <- err
	}()
	cc := &tls.Config{RootCAs: roots, ServerName: "tunnel.test", NextProtos: []string{"h2", "http/1.1"}, MinVersion: tls.VersionTLS12}
	if noSNI {
		cc.ServerName = ""
		cc.InsecureSkipVerify = true
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", ln.Addr().String(), cc)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	leaf := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, cert := range state.PeerCertificates[1:] {
		intermediates.AddCert(cert)
	}
	if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, Intermediates: intermediates, DNSName: "tunnel.test"}); err != nil {
		t.Fatal(err)
	}
	if state.NegotiatedProtocol != "http/1.1" {
		t.Fatalf("WebSocket ALPN: %q", state.NegotiatedProtocol)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	return leaf
}

func TestACMEIssuanceCacheAndSNI(t *testing.T) {
	for _, challenge := range []string{"http-01", "tls-alpn-01"} {
		t.Run(challenge, func(t *testing.T) {
			ca := testacme.NewCAServer(t).ChallengeTypes(challenge).Start()
			cache := t.TempDir()
			cfg, _ := controlledACME(t, ca, cache)
			first := verifiedLeaf(t, cfg, ca.Roots(), false)
			second := verifiedLeaf(t, cfg, ca.Roots(), true)
			if !bytes.Equal(first.Raw, second.Raw) {
				t.Fatal("no-SNI used another certificate")
			}
			if _, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "unconfigured.test"}); err == nil {
				t.Fatal("wrong SNI accepted")
			}
			// A new manager loads the issued key pair from disk. A dead HTTP client
			// makes any attempt to reissue fail, proving cache reuse across restarts.
			fresh, _ := controlledACME(t, ca, cache, true)
			got := verifiedLeaf(t, fresh, ca.Roots(), true)
			if !bytes.Equal(first.Raw, got.Raw) {
				t.Fatal("cached leaf changed")
			}
			if ca.CertificatesIssued() != 1 {
				t.Fatalf("issued %d certificates", ca.CertificatesIssued())
			}
		})
	}
}

type rejectACMETransport struct{}

func (rejectACMETransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, os.ErrPermission
}

func TestACMEAutomaticRenewal(t *testing.T) {
	ca := testacme.NewCAServer(t).ChallengeTypes("http-01").FirstNearExpiry().Start()
	cfg, _ := controlledACME(t, ca, t.TempDir())
	first := verifiedLeaf(t, cfg, ca.Roots(), false)
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		next := verifiedLeaf(t, cfg, ca.Roots(), true)
		if ca.CertificatesIssued() >= 2 && next.NotAfter.After(time.Now().Add(30*24*time.Hour)) && (first.NotAfter.After(time.Now().Add(30*24*time.Hour)) || !bytes.Equal(next.Raw, first.Raw)) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("renewed certificate was not served without restart")
}

func TestCertificateReloadAtomicAndPartialRotation(t *testing.T) {
	dir := t.TempDir()
	cert, key := writeTestPair(t, dir)
	cfg, err := fileTLSConfig(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := cfg.GetCertificate(nil)
	nextCert, nextKey := writeTestPair(t, t.TempDir())
	certData, _ := os.ReadFile(nextCert)
	keyData, _ := os.ReadFile(nextKey)
	st, _ := os.Stat(cert)
	stamp := st.ModTime().Add(-time.Hour)
	// Key-first renewal initially mismatches: keep the last good pair.
	if err := os.WriteFile(key, keyData, 0600); err != nil {
		t.Fatal(err)
	}
	partial, err := cfg.GetCertificate(nil)
	if err != nil || !bytes.Equal(partial.Certificate[0], original.Certificate[0]) {
		t.Fatal("partial rotation lost last good certificate")
	}
	// Atomic certificate replacement with an older timestamp must still reload.
	temporary := filepath.Join(dir, "renewed.crt")
	os.WriteFile(temporary, certData, 0600)
	os.Chtimes(temporary, stamp, stamp)
	if err := os.Rename(temporary, cert); err != nil {
		t.Fatal(err)
	}
	renewed, err := cfg.GetCertificate(nil)
	if err != nil || bytes.Equal(renewed.Certificate[0], original.Certificate[0]) {
		t.Fatal("replacement not loaded")
	}
	// Malformed later writes must not break existing service.
	os.WriteFile(cert, []byte("incomplete"), 0600)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := cfg.GetCertificate(nil)
			if err != nil || !bytes.Equal(got.Certificate[0], renewed.Certificate[0]) {
				t.Error("concurrent reload lost last good pair")
			}
		}()
	}
	wg.Wait()
}

func TestACMEConcurrentFallbackAndTunnelFailure(t *testing.T) {
	cert, key := writeTestPair(t, t.TempDir())
	manager := func() *autocert.Manager {
		return &autocert.Manager{Prompt: autocert.AcceptTOS, HostPolicy: autocert.HostWhitelist("allowed.test"), Cache: autocert.DirCache(t.TempDir()), Client: &acme.Client{HTTPClient: &http.Client{Transport: rejectACMETransport{}}}}
	}
	a := &acmeHTTPResponder{addr: "127.0.0.1:0"}
	defer func() {
		a.mu.Lock()
		ln := a.listener
		a.mu.Unlock()
		if ln != nil {
			ln.Close()
		}
	}()
	var mu sync.Mutex
	warnings := 0
	cfg, err := configureACMETLS(TLSSettings{ACMEDomain: "not-allowed.test", FallbackCertFile: cert, FallbackKeyFile: key}, manager(), a, func(format string, args ...any) {
		if len(format) >= 7 && format[:7] == "serving" {
			mu.Lock()
			warnings++
			mu.Unlock()
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := cfg.GetCertificate(&tls.ClientHelloInfo{ServerName: "not-allowed.test"}); err != nil || got == nil {
				t.Errorf("fallback failed: %v", err)
			}
		}()
	}
	wg.Wait()
	mu.Lock()
	n := warnings
	mu.Unlock()
	if n != 1 {
		t.Fatalf("fallback warnings=%d", n)
	}
	tunnel, err := configureACMETLS(TLSSettings{ACMEDomain: "not-allowed.test"}, manager(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tunnel.GetCertificate(&tls.ClientHelloInfo{ServerName: "not-allowed.test"}); err == nil {
		t.Fatal("ACME tunnel silently accepted self-signed fallback")
	}
}

// Legacy error-path tests must not contact the public CA or keep issuance
// goroutines running into unrelated allocation/transport tests.
func unavailableACMETLS(t *testing.T, s TLSSettings) (*tls.Config, error) {
	t.Helper()
	m := &autocert.Manager{Prompt: autocert.AcceptTOS, Cache: autocert.DirCache(t.TempDir()), HostPolicy: autocert.HostWhitelist(s.ACMEDomain), Client: &acme.Client{HTTPClient: &http.Client{Transport: rejectACMETransport{}}, RetryBackoff: func(int, *http.Request, *http.Response) time.Duration { return 0 }}}
	a := &acmeHTTPResponder{addr: "127.0.0.1:0"}
	primed := make(chan struct{}, 1)
	cfg, err := configureACMETLS(s, m, a, func(string, ...any) {
		select {
		case primed <- struct{}{}:
		default:
		}
	})
	if err == nil {
		select {
		case <-primed:
		case <-time.After(5 * time.Second):
			t.Fatal("offline priming did not finish")
		}
	}
	t.Cleanup(func() {
		a.mu.Lock()
		ln := a.listener
		a.mu.Unlock()
		if ln != nil {
			ln.Close()
		}
	})
	return cfg, err
}

func TestACMERateLimitIsNotRetriedInsideHandshake(t *testing.T) {
	if got := acmeRetryBackoff(1, nil, &http.Response{StatusCode: http.StatusTooManyRequests}); got != 0 {
		t.Fatalf("rate-limit retried after %s", got)
	}
	for _, n := range []int{1, 2, 3, 4, 100} {
		if got := acmeRetryBackoff(n, nil, &http.Response{StatusCode: http.StatusServiceUnavailable}); got <= 0 || got > 10*time.Second {
			t.Fatalf("transient backoff %s", got)
		}
	}
}
