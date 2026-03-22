package core

import (
	"testing"
	"time"
)

func TestReduceTaskCompletedToPendingThenCooldownThenIdle(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	s := InitialState()
	s = Reduce(s, Event{Type: EventTaskStarted, At: now})
	if s.Phase != PhaseActive {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseActive)
	}

	startAt := now.Add(10 * time.Second)
	s = Reduce(s, Event{
		Type:             EventTaskCompleted,
		At:               now,
		CooldownStartAt:  startAt,
		CooldownDuration: 30 * time.Second,
	})
	if s.Phase != PhasePendingCooldown {
		t.Fatalf("phase = %s, want %s", s.Phase, PhasePendingCooldown)
	}

	s = Reduce(s, Event{Type: EventTick, At: now.Add(5 * time.Second)})
	if s.Phase != PhasePendingCooldown {
		t.Fatalf("phase = %s, want still %s before deadline", s.Phase, PhasePendingCooldown)
	}

	s = Reduce(s, Event{Type: EventTick, At: startAt})
	if s.Phase != PhaseCooldown {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseCooldown)
	}

	s = Reduce(s, Event{Type: EventTick, At: startAt.Add(30 * time.Second)})
	if s.Phase != PhaseIdle {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseIdle)
	}
}

func TestReduceBreakExpiryReturnsToActive(t *testing.T) {
	now := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)
	s := InitialState()
	s = Reduce(s, Event{Type: EventTaskStarted, At: now})

	breakUntil := now.Add(20 * time.Second)
	s = Reduce(s, Event{Type: EventBreakStarted, At: now, BreakUntil: breakUntil})
	if s.Phase != PhaseBreak {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseBreak)
	}

	s = Reduce(s, Event{Type: EventTick, At: breakUntil})
	if s.Phase != PhaseActive {
		t.Fatalf("phase = %s, want %s", s.Phase, PhaseActive)
	}
}

func TestReduceLockAndIdleFlags(t *testing.T) {
	s := InitialState()
	s = Reduce(s, Event{Type: EventScreenLocked, At: time.Now()})
	if !s.ScreenLocked {
		t.Fatal("screen locked flag should be true")
	}
	s = Reduce(s, Event{Type: EventScreenUnlock, At: time.Now()})
	if s.ScreenLocked {
		t.Fatal("screen locked flag should be false")
	}
	s = Reduce(s, Event{Type: EventIdleEntered, At: time.Now()})
	if !s.IdleActive {
		t.Fatal("idle flag should be true")
	}
	s = Reduce(s, Event{Type: EventIdleExited, At: time.Now()})
	if s.IdleActive {
		t.Fatal("idle flag should be false")
	}
}
