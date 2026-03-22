package core

import (
	"testing"
	"time"
)

func TestRuntimeProcessesEvents(t *testing.T) {
	rt := NewRuntime(InitialState())
	defer rt.Close()

	now := time.Now()
	rt.Publish(Event{Type: EventTaskStarted, At: now})

	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		if rt.Snapshot().Phase == PhaseActive {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}

	t.Fatalf("phase = %s, want %s", rt.Snapshot().Phase, PhaseActive)
}

func TestRuntimeTickTransitionsPendingCooldown(t *testing.T) {
	rt := NewRuntime(InitialState())
	defer rt.Close()

	start := time.Now()
	rt.Publish(Event{Type: EventTaskStarted, At: start})
	rt.Publish(Event{
		Type:             EventTaskCompleted,
		At:               start,
		CooldownStartAt:  start.Add(5 * time.Millisecond),
		CooldownDuration: 5 * time.Millisecond,
	})

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if rt.Snapshot().Phase == PhaseIdle {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}

	t.Fatalf("phase = %s, want %s", rt.Snapshot().Phase, PhaseIdle)
}
