package transport

import (
	"testing"
	"time"
)

func TestPCKLearnsPeriodicProbesWithoutChangingLegacyHeartbeatPolicy(t *testing.T) {
	now := time.Now()
	beats := newBeatClock(now)
	probes := newBeatClock(time.Time{})
	for i := 1; i <= 5; i++ {
		beats.beat(now.Add(time.Duration(i) * 10 * time.Second))
	}
	loop := controlLoop{keepAlive: 75 * time.Second, learnProbes: true}
	probes.beat(now)
	if loop.deadlineClock(beats, probes) != beats {
		t.Fatal("a legacy startup probe shortened the deadline")
	}
	for i := 1; i <= 3; i++ {
		probes.beat(now.Add(time.Duration(i) * 5 * time.Second))
	}
	if got := loop.deadlineClock(beats, probes).deadline(loop.keepAlive); got != 15*time.Second {
		t.Fatalf("learned deadline=%s", got)
	}
	loop.learnProbes = false
	if got := loop.deadlineClock(beats, probes).deadline(loop.keepAlive); got != 30*time.Second {
		t.Fatalf("ordinary carrier changed deadline=%s", got)
	}
	loop.learnProbes = true
	for i := 4; i <= 8; i++ {
		probes.beat(now.Add(time.Duration(i) * 10 * time.Second))
	}
	if loop.deadlineClock(beats, probes) != beats {
		t.Fatal("slow probes retained the faster deadline")
	}
}
