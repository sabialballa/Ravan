package transport

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/backpack/backpack/internal/utils"
)

// Reproduce a half-open channel: Iran can still write into the connection,
// but the kharej never answers. Socket existence must not pin the client's seat.
func TestWebSocketSilentPeerIsRetiredWhileWritesStillWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, restarts, finished := startLoop(t, ctx, func(c *controlLoop) {
		c.beat = 20 * time.Millisecond
		c.readTimeout = 150 * time.Millisecond
	})
	defer client.Close()
	go io.Copy(io.Discard, client) // healthy write direction, no acknowledgements
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for restarts.Load() == 0 {
		select {
		case <-deadline.C:
			t.Fatal("silent WebSocket peer kept its seat despite missing heartbeats")
		case <-time.After(5 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("control loop did not release its channel after cancellation")
	}
}

func TestWebSocketIdlePeerWithAcknowledgementsIsNotRestarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, restarts, finished := startLoop(t, ctx, func(c *controlLoop) {
		c.beat = 20 * time.Millisecond
		c.readTimeout = 150 * time.Millisecond
	})
	defer client.Close()
	go func() {
		for {
			b, err := utils.ReceiveBinaryByte(client)
			if err != nil {
				return
			}
			if b == utils.SG_HB {
				if err := utils.SendBinaryByteWithin(client, b, time.Second); err != nil {
					return
				}
			}
		}
	}()
	time.Sleep(650 * time.Millisecond) // more than four silence deadlines
	if restarts.Load() != 0 {
		t.Fatal("healthy idle channel was restarted")
	}
	cancel()
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("healthy loop did not stop")
	}
}

func TestWebSocketHeartbeatBoundAllowsSeveralMissedBeats(t *testing.T) {
	for _, beat := range []time.Duration{0, time.Second, 10 * time.Second, time.Hour} {
		got := websocketControlTimeout(beat)
		if got < 30*time.Second || got < 4*livenessBeat(beat) || got > time.Minute {
			t.Fatalf("unexpected silence bound %v for beat %v", got, beat)
		}
	}
}
