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

func TestReduceCooldownStartEmitsTaskEndAction(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	base := InitialState()
	base.Phase = PhasePendingCooldown
	base.CooldownStartUntil = now
	base.CooldownDuration = 30 * time.Second
	base.TaskEndAction = "sleep"

	res := Reduce(base, Event{Type: EventTick, At: now})
	if res.State.Phase != PhaseCooldown {
		t.Fatalf("phase = %s, want %s", res.State.Phase, PhaseCooldown)
	}
	foundSleep := false
	for _, action := range res.Actions {
		if action.Type == ActionSleep {
			foundSleep = true
		}
	}
	if !foundSleep {
		t.Fatal("expected sleep action on cooldown start")
	}

	base.TaskEndAction = "lock"
	res = Reduce(base, Event{Type: EventTick, At: now})
	foundLock := false
	for _, action := range res.Actions {
		if action.Type == ActionLockScreen {
			foundLock = true
		}
	}
	if !foundLock {
		t.Fatal("expected lock action on cooldown start")
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

func TestReduceLockFlags(t *testing.T) {
	s := InitialState()
	res := Reduce(s, Event{Type: EventScreenLocked, At: time.Now()})
	if !res.State.ScreenLocked {
		t.Fatal("screen locked flag should be true")
	}

	res = Reduce(res.State, Event{Type: EventScreenUnlock, At: time.Now()})
	if res.State.ScreenLocked {
		t.Fatal("screen locked flag should be false")
	}
}
