package network

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/backpack/backpack/config"
)

func TestWebSocketRetryStopsWhenContextIsCancelled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
			cancel()
		}
	}()
	done := make(chan error, 1)
	go func() {
		conn, err := WebSocketDialer(ctx, nil, ln.Addr().String(), "", "/channel", time.Second, 0, true, "test-token", config.WS, false, 3, 0, 0, 0)
		if conn != nil {
			conn.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wanted cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled WebSocket dial is still sleeping between retries")
	}
}
