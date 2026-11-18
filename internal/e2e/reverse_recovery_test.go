//go:build linux

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// An explicit opt-in: a real one-hour comparison, not an accelerated timer.
func TestReversePCKRecoveryComparedWithWSSMux(t *testing.T) {
	if os.Getenv("RAVAN_REVERSE_RECOVERY_LIVE") == "" {
		t.Skip("set RAVAN_REVERSE_RECOVERY_LIVE=1 on a Linux lab host")
	}
	seconds := 3600
	if s := os.Getenv("RAVAN_REVERSE_RECOVERY_SECONDS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 7200 {
			t.Fatal("recovery duration must be 1..7200 seconds")
		}
		seconds = n
	}
	netNS, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	mountNS, err := os.Readlink("/proc/self/ns/mnt")
	if err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs("testdata/reverse_recovery_lab.py")
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(t.TempDir(), "run")
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(seconds+300)*time.Second)
	defer cancel()
	args := []string{"--net", "--mount", "--fork"}
	if os.Geteuid() != 0 {
		args = append([]string{"--map-auto", "--map-root-user"}, args...)
	}
	args = append(args, "python3", script, "--binary", currentBinary(t), "--work", work, "--seconds", strconv.Itoa(seconds), "--parent-net", netNS, "--parent-mount", mountNS)
	cmd := exec.CommandContext(ctx, "unshare", args...)
	if previous := os.Getenv(prevBinaryEnv); previous != "" {
		cmd.Args = append(cmd.Args, "--client-binary", previous)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("reverse recovery comparison: %v\n%s", err, out)
	}
	var result struct {
		Complete bool
		Elapsed  float64 `json:"elapsed_seconds"`
		Faults   int     `json:"fault_count"`
		Modes    map[string]struct {
			Corrupt int
			Bytes   uint64
			LastOK  float64 `json:"last_ok"`
		}
	}
	data, err := os.ReadFile(filepath.Join(work, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Elapsed < float64(seconds) {
		t.Fatal("comparison did not complete the requested wall-clock duration")
	}
	for _, tr := range []string{"pck", "wssmux"} {
		s, ok := result.Modes[tr]
		if !ok || s.Corrupt != 0 || s.Bytes == 0 {
			t.Fatalf("%s did not carry intact data", tr)
		}
		if float64(time.Now().UnixNano())/1e9-s.LastOK > 10 {
			t.Fatalf("%s ended without recent successful traffic", tr)
		}
	}
	if seconds >= 3600 && result.Faults != 10 {
		t.Fatalf("completed %d faults, want 10", result.Faults)
	}
	if result.Faults > 0 {
		data, err := os.ReadFile(filepath.Join(work, "faults.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range bytes.Split(bytes.TrimSpace(data), []byte("\n")) {
			var fault struct {
				Kind  string
				Modes map[string]struct {
					Lost     bool     `json:"lost_seen"`
					Recovery *float64 `json:"application_recovery_seconds"`
					Before   int      `json:"control_claims_before"`
					After    int      `json:"control_claims_after"`
				}
			}
			if err := json.Unmarshal(line, &fault); err != nil {
				t.Fatal(err)
			}
			for tr, s := range fault.Modes {
				if s.Lost && (s.Recovery == nil || *s.Recovery < 0) {
					t.Fatalf("%s did not recover after %s", tr, fault.Kind)
				}
			}
			if fault.Kind == "outgoing" {
				s := fault.Modes["pck"]
				if s.After <= s.Before {
					t.Fatal("PCK retained the blackholed control flow")
				}
			}
		}
	}
	t.Log(string(out))
}
