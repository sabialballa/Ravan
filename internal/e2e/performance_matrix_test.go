//go:build linux

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/backpack/backpack/internal/manage/tunnelspec"
)

// Opt-in comparative measurement; software-emulated loopback is not an uplink
// speed certification. Run this identical harness on baseline and candidate.
func TestPerformanceMatrix(t *testing.T) {
	if os.Getenv("RAVAN_PERFORMANCE") == "" {
		t.Skip("set RAVAN_PERFORMANCE=1")
	}
	mib, passes := 4, 2
	for name, dst := range map[string]*int{"RAVAN_PERFORMANCE_MIB": &mib, "RAVAN_PERFORMANCE_PASSES": &passes} {
		if raw := os.Getenv(name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > 64 {
				t.Fatalf("%s must be 1..64", name)
			}
			*dst = n
		}
	}
	payload := randomPayload(t, mib<<20)
	received := make([]byte, len(payload))
	for _, tr := range []string{"tcp", "tcpmux", "ws", "wss", "wsmux", "wssmux", "stealth", "kcp", "quic"} {
		t.Run(tr, func(t *testing.T) {
			backend := startEchoBackend(t)
			tp, ep := freePort(t), freePort(t)
			token := "perf-" + tr + "-0123456789abcdefghijklmnop"
			s := baseServerConfig(tr, tp, ep, backend.addr, token)
			c := baseClientConfig(tr, fmt.Sprintf("127.0.0.1:%d", tp), token, nil)
			var preset tunnelspec.Spec
			tunnelspec.ApplyPreset(&preset, tunnelspec.PresetTurbo)
			s.MaxFrameSize, s.MaxReceiveBuffer, s.MaxStreamBuffer = preset.MuxFrameSize, preset.MuxRecvBuffer, preset.MuxStreamBuffer
			c.MaxFrameSize, c.MaxReceiveBuffer, c.MaxStreamBuffer = s.MaxFrameSize, s.MaxReceiveBuffer, s.MaxStreamBuffer
			if tr == "wss" || tr == "wssmux" {
				s.TLSCertFile, s.TLSKeyFile = testCert(t)
			}
			tun := runPair(t, s, c, ep, tp)
			if err := tun.waitReady(60 * time.Second); err != nil {
				t.Fatal(err)
			}
			if err := perfRoundTrip(tun.Entry, payload[:64<<10]); err != nil {
				t.Fatal(err)
			}
			latencies := make([]float64, 8)
			for i := range latencies {
				start := time.Now()
				if err := perfRoundTrip(tun.Entry, payload[:128]); err != nil {
					t.Fatal(err)
				}
				latencies[i] = float64(time.Since(start).Microseconds()) / 1000
			}
			sort.Float64s(latencies)
			runtime.GC()
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			var cpuBefore, cpuAfter syscall.Rusage
			syscall.Getrusage(syscall.RUSAGE_SELF, &cpuBefore)
			start := time.Now()
			for i := 0; i < passes; i++ {
				if err := perfRoundTripInto(tun.Entry, payload, received); err != nil {
					t.Fatal(err)
				}
			}
			seconds := time.Since(start).Seconds()
			syscall.Getrusage(syscall.RUSAGE_SELF, &cpuAfter)
			runtime.ReadMemStats(&after)
			cpu := func(r syscall.Rusage) float64 {
				return float64(r.Utime.Sec+r.Stime.Sec) + float64(r.Utime.Usec+r.Stime.Usec)/1e6
			}
			result := map[string]any{"transport": tr, "payload_bytes": len(payload), "round_trips": passes, "mbit_s": float64(len(payload)) * 2 * float64(passes) * 8 / seconds / 1e6, "seconds": seconds, "process_cpu_seconds": cpu(cpuAfter) - cpu(cpuBefore), "allocated_bytes": after.TotalAlloc - before.TotalAlloc, "allocations": after.Mallocs - before.Mallocs, "gc_cycles": after.NumGC - before.NumGC, "latency_p50_ms": latencies[4], "latency_p95_ms": latencies[7], "mux_stream_buffer": s.MaxStreamBuffer, "forwarded_tcp_verified": true}
			raw, _ := json.Marshal(result)
			t.Log("PERFORMANCE_RESULT " + string(raw))
		})
	}
}
func perfRoundTrip(entry string, payload []byte) error {
	return perfRoundTripInto(entry, payload, make([]byte, len(payload)))
}
func perfRoundTripInto(entry string, payload, received []byte) error {
	conn, err := net.DialTimeout("tcp", entry, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(120 * time.Second))
	done := make(chan error, 1)
	go func() { _, err := io.Copy(conn, bytes.NewReader(payload)); done <- err }()
	if _, err = io.ReadFull(conn, received); err != nil {
		return err
	}
	if err = <-done; err != nil {
		return err
	}
	if !bytes.Equal(payload, received) {
		return fmt.Errorf("data corrupted")
	}
	return nil
}
