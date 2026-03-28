package domain

import (
	"testing"
	"time"
)

func TestReduceTaskCompletedToPendingThenCooldownThenIdle(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	s := InitialState()
	task := &Task{ID: 1, Title: "demo", Duration: 30 * time.Minute, StartTime: now}

	res := Reduce(s, Event{Type: EventTaskStarted, At: now, Task: task})
	if res.State.Phase != PhaseActive {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseActive)
	}

	startAt := now.Add(10 * time.Second)
	res = Reduce(res.State, Event{
		Type:             EventTaskCompleted,
		At:               now,
		CooldownStartAt:  startAt,
		CooldownDuration: 30 * time.Second,
	})
	if res.State.Phase != PhasePendingCooldown {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhasePendingCooldown)
	}

	res = Reduce(res.State, Event{Type: EventTick, At: now.Add(5 * time.Second)})
	if res.State.Phase != PhasePendingCooldown {
		t.Fatalf("phase = %s, want still %s before deadline", res.State.Phase, PhasePendingCooldown)
	}

	res = Reduce(res.State, Event{Type: EventTick, At: startAt})
	if res.State.Phase != PhaseCooldown {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseCooldown)
	}

	res = Reduce(res.State, Event{Type: EventTick, At: startAt.Add(30 * time.Second)})
	if res.State.Phase != PhaseIdle {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseIdle)
	}
}

func TestReduceBreakExpiryReturnsToActive(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	s := InitialState()
	task := &Task{ID: 1, Title: "demo", Duration: time.Hour, StartTime: now}

	res := Reduce(s, Event{Type: EventTaskStarted, At: now, Task: task})
	if res.State.Phase != PhaseActive {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseActive)
	}

	breakUntil := now.Add(20 * time.Second)
	res = Reduce(res.State, Event{Type: EventBreakStarted, At: now, BreakUntil: breakUntil})
	if res.State.Phase != PhaseBreak {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseBreak)
	}

	res = Reduce(res.State, Event{Type: EventTick, At: breakUntil})
	if res.State.Phase != PhaseActive {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseActive)
	}
}

func TestReduceLockAndIdleFlags(t *testing.T) {
	s := InitialState()
	res := Reduce(s, Event{Type: EventScreenLocked, At: time.Now()})
	if !res.State.ScreenLocked {
		t.Fatal("screen locked flag should be true")
	}

	res = Reduce(res.State, Event{Type: EventScreenUnlock, At: time.Now()})
	if res.State.ScreenLocked {
		t.Fatal("screen locked flag should be false")
	}

	res = Reduce(res.State, Event{Type: EventIdleEntered, At: time.Now()})
	if !res.State.IdleActive {
		t.Fatal("idle flag should be true")
	}

	res = Reduce(res.State, Event{Type: EventIdleExited, At: time.Now()})
	if res.State.IdleActive {
		t.Fatal("idle flag should be false")
	}
}
