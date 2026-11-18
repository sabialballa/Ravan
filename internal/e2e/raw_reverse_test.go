//go:build linux

package e2e

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/backpack/backpack/internal/client"
	"github.com/backpack/backpack/internal/server"
)

// Runs the actual reverse XDI/PCK client and server in different namespaces;
// no raw packets or firewall rules leave the enclosing network namespace.
func TestRawReverseTransports(t *testing.T) {
	if os.Getenv("RAVAN_RAW_LIVE") == "" {
		t.Skip("set RAVAN_RAW_LIVE=1")
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	parentNet, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	parentMount, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("testdata/reverse_raw.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range []string{"xdi", "pck"} {
		t.Run(tr, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			cmd := exec.CommandContext(ctx, "unshare", "--map-auto", "--map-root-user", "--net", "--mount", "--fork", "--", "bash", script, tr)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
			cmd.Env = append(os.Environ(), "BIN="+binary, "W="+t.TempDir(), "RAVAN_PARENT_NETNS="+parentNet, "RAVAN_PARENT_MNTNS="+parentMount)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s: %v\n%s", tr, err, output)
			}
			t.Log(string(output))
		})
	}
}
func TestRawReverseNode(t *testing.T) {
	role, tr := os.Getenv("RAVAN_RAW_ROLE"), os.Getenv("RAVAN_RAW_TRANSPORT")
	if role == "" {
		t.Skip("raw test subprocess only")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()
	const token = "isolated-raw-perf-test-0123456789abcdef"
	if role == "server" {
		cfg := baseServerConfig(tr, 9000, 11001, "127.0.0.1:10001", token)
		cfg.BindAddr = "10.99.0.1:9000"
		cfg.PckInterface = "vi"
		server.NewServer(cfg, ctx).Start()
	} else {
		cfg := baseClientConfig(tr, "10.99.0.1:9000", token, nil)
		cfg.PckInterface = "vk"
		client.NewClient(cfg, ctx).Start()
	}
}
