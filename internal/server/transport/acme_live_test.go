//go:build acmelive

package transport_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
	"github.com/backpack/backpack/internal/client"
	transports "github.com/backpack/backpack/internal/server/transport"
	"github.com/backpack/backpack/internal/testport"
	"github.com/backpack/backpack/internal/utils"
)

// Explicit opt-in only. Uses the real public CA and a persistent, isolated
// cache supplied by the operator; ordinary go test never issues certificates.
func TestRavanLiveACMECertificate(t *testing.T) {
	domain, cache, mode := os.Getenv("RAVAN_ACME_LIVE_DOMAIN"), os.Getenv("RAVAN_ACME_LIVE_CACHE"), os.Getenv("RAVAN_ACME_LIVE_MODE")
	if domain == "" || cache == "" {
		t.Skip("explicit live domain/cache required")
	}
	if !filepath.IsAbs(cache) {
		t.Fatal("an absolute isolated cache is required")
	}
	if mode != "wss" && mode != "wssmux" {
		t.Fatal("mode must be wss or wssmux")
	}
	bind := os.Getenv("RAVAN_ACME_LIVE_BIND")
	if bind == "" {
		bind = "0.0.0.0:18443"
	}
	_, port, err := net.SplitHostPort(bind)
	if err != nil {
		t.Fatal(err)
	}
	remote := net.JoinHostPort("127.0.0.1", port)
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	go func() {
		for {
			c, e := backend.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	entryPort := testport.Free(t)
	entry := net.JoinHostPort("127.0.0.1", fmt.Sprint(entryPort))
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal(err)
	}
	token := hex.EncodeToString(secret[:])
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	defer func() {
		cancel()
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("transport did not stop")
		}
	}()
	logger := utils.NewLogger("info")
	var start func()
	if mode == "wss" {
		s := transports.NewWSServer(ctx, &transports.WsConfig{BindAddr: bind, Mode: config.WSS, Token: token, Ports: []string{fmt.Sprintf("%d=%s", entryPort, backend.Addr())}, ChannelSize: 32, Heartbeat: time.Second, KeepAlive: 10 * time.Second, Nodelay: true, ACMEDomain: domain, ACMECacheDir: cache, MaxConnections: 16}, logger)
		start = s.Start
	} else {
		s := transports.NewWSMuxServer(ctx, &transports.WsMuxConfig{BindAddr: bind, Mode: config.WSSMUX, Token: token, Ports: []string{fmt.Sprintf("%d=%s", entryPort, backend.Addr())}, ChannelSize: 32, Heartbeat: time.Second, KeepAlive: 10 * time.Second, Nodelay: true, ACMEDomain: domain, ACMECacheDir: cache, MuxCon: 4, MuxVersion: 2, MaxFrameSize: 32768, MaxReceiveBuffer: 4194304, MaxStreamBuffer: 65536, MaxConnections: 16}, logger)
		start = s.Start
	}
	wg.Add(1)
	go func() { defer wg.Done(); start() }()
	var issued *x509.Certificate
	for _, sni := range []string{domain, ""} {
		deadline := time.Now().Add(10 * time.Minute)
		var last error
		for time.Now().Before(deadline) {
			cfg := &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}, ServerName: sni}
			// Only the no-SNI probe performs verification manually below. It still
			// requires a chain trusted by the system and the configured DNS name.
			if sni == "" {
				cfg.InsecureSkipVerify = true
			}
			c, e := tls.DialWithDialer(&net.Dialer{Timeout: 20 * time.Second}, "tcp", remote, cfg)
			if e != nil {
				last = e
				time.Sleep(time.Second)
				continue
			}
			state := c.ConnectionState()
			c.Close()
			intermediates := x509.NewCertPool()
			for _, crt := range state.PeerCertificates[1:] {
				intermediates.AddCert(crt)
			}
			leaf := state.PeerCertificates[0]
			if _, e := leaf.Verify(x509.VerifyOptions{DNSName: domain, Intermediates: intermediates}); e != nil {
				t.Fatalf("certificate chain/domain verification: %v", e)
			}
			if issued != nil && !bytes.Equal(issued.Raw, leaf.Raw) {
				t.Fatal("SNI and IP probes received different certificates")
			}
			issued = leaf
			last = nil
			break
		}
		if last != nil || issued == nil {
			t.Fatalf("certificate not usable with SNI=%q: %v", sni, last)
		}
	}
	bad, err := tls.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}, "tcp", remote, &tls.Config{ServerName: "not-configured.invalid", InsecureSkipVerify: true})
	if err == nil {
		bad.Close()
		t.Fatal("unconfigured SNI accepted")
	}
	cli := client.NewClient(&config.ClientConfig{RemoteAddr: remote, Transport: config.TransportType(mode), Token: token, ConnectionPool: 2, RetryInterval: 1, Nodelay: true, Keepalive: 10, LogLevel: "error", MuxSession: 4, MuxVersion: 2, MaxFrameSize: 32768, MaxReceiveBuffer: 4194304, MaxStreamBuffer: 65536, DialTimeout: 5, SkipOptz: true}, ctx)
	wg.Add(1)
	go func() { defer wg.Done(); cli.Start() }()
	payload := bytes.Repeat([]byte("real-acme-tunnel-"), 4096)
	for n := 0; n < 4; n++ {
		deadline := time.Now().Add(40 * time.Second)
		var last error
		for time.Now().Before(deadline) {
			c, e := net.DialTimeout("tcp", entry, time.Second)
			if e != nil {
				last = e
				time.Sleep(200 * time.Millisecond)
				continue
			}
			c.SetDeadline(time.Now().Add(10 * time.Second))
			done := make(chan error, 1)
			go func() { _, e := c.Write(payload); done <- e }()
			got := make([]byte, len(payload))
			_, e = io.ReadFull(c, got)
			c.Close()
			writeErr := <-done
			if e == nil && writeErr == nil && bytes.Equal(payload, got) {
				last = nil
				break
			}
			last = fmt.Errorf("echo read=%v write=%v", e, writeErr)
			time.Sleep(200 * time.Millisecond)
		}
		if last != nil {
			t.Fatal(last)
		}
	}
	fingerprint := sha256.Sum256(issued.Raw)
	result := map[string]any{"domain": domain, "mode": mode, "issuer": issued.Issuer.String(), "serial": issued.SerialNumber.String(), "not_before": issued.NotBefore, "not_after": issued.NotAfter, "sha256": hex.EncodeToString(fingerprint[:]), "system_chain_verified": true, "domain_and_no_sni_verified": true, "wrong_sni_rejected": true, "round_trips": 4, "bytes_per_round_trip": len(payload)}
	raw, _ := json.Marshal(result)
	t.Logf("LIVE_ACME_RESULT %s", raw)
}
