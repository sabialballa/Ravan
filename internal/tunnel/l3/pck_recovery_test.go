package l3

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type blockedOutgoingCarrier struct {
	DatagramCarrier
	blocked atomic.Bool
}

func (c *blockedOutgoingCarrier) WriteTo(p []byte, addr net.Addr) (int, error) {
	if c.blocked.Load() {
		return len(p), nil
	}
	return c.DatagramCarrier.WriteTo(p, addr)
}

// Exercise PCK's recovery policy over a real UDP socket so the regression can
// run without raw-socket privileges. The live namespace tests cover raw PCK.
// Incoming authenticated traffic must not conceal a blocked outgoing flow.
func TestPCKDetectsOneWayFailureWithAutoMTUDisabled(t *testing.T) {
	oldCheck := rekeyCheck
	rekeyCheck = 50 * time.Millisecond
	t.Cleanup(func() { rekeyCheck = oldCheck })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := Config{Mode: ModeListen, Addr: "127.0.0.1:0", Token: "pck-recovery-regression",
		Encap: "gre", LocalIP: "10.10.0.2/30", PeerIP: "10.10.0.1", MTU: 1400}
	listener, err := New(cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	dev := newFakeDevice(1400)
	listener.openDevice = func(deviceSpec) (packetDevice, error) { return dev, nil }
	start(t, ctx, cancel, listener, dev)
	cfg.Mode, cfg.Addr = ModeDial, awaitBind(t, listener).String()
	cfg.LocalIP, cfg.PeerIP = "10.10.0.1/30", "10.10.0.2"
	dialer, err := New(cfg, quietLogger())
	if err != nil {
		t.Fatal(err)
	}
	carrier, peer, err := openCarrier(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer carrier.Close()
	blocked := &blockedOutgoingCarrier{DatagramCarrier: carrier}
	dialer.carrier = blocked
	dialer.setPeer(peer)
	dialer.tun = newFakeDevice(1400)
	defer dialer.tun.Close()
	dialer.cfg.Carrier = CarrierPck
	dialer.probe.timeout = 30 * time.Millisecond
	pumpDone, loopDone := make(chan struct{}), make(chan struct{})
	go func() { defer close(pumpDone); dialer.pumpFromCarrier(ctx) }()
	go func() { defer close(loopDone); dialer.handshakeLoop(ctx, func() {}) }()
	defer func() { cancel(); carrier.Close(); <-pumpDone; <-loopDone }()
	awaitSession(t, dialer, 5*time.Second)
	// A healthy idle peer must answer probes without unnecessary rekeys.
	time.Sleep(250 * time.Millisecond)
	if dialer.stats.handshakes.Load() != 1 {
		t.Fatal("healthy idle flow was rekeyed")
	}
	blocked.blocked.Store(true)
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	chatter := time.NewTicker(20 * time.Millisecond)
	defer chatter.Stop()
	for {
		select {
		case <-loopDone:
			if !dialer.flowStuck.Load() {
				t.Fatal("failed flow was not marked for reopening")
			}
			return
		case <-chatter.C:
			dev.inject <- ipv4Packet(1)
		case <-deadline.C:
			t.Fatal("incoming traffic concealed the broken outgoing flow; PCK was not reopened promptly")
		}
	}
}
