package core

import "time"

// Reduce applies one event and returns the next deterministic state.
func Reduce(prev State, ev Event) State {
	next := prev

	switch ev.Type {
	case EventTaskStarted:
		next.Phase = PhaseActive
		next.BreakUntil = time.Time{}
		next.CooldownStartUntil = time.Time{}
		next.CooldownUntil = time.Time{}
		next.CooldownDuration = 0
	case EventBreakStarted:
		next.Phase = PhaseBreak
		next.BreakUntil = ev.BreakUntil
	case EventBreakEnded:
		if next.Phase == PhaseBreak {
			next.Phase = PhaseActive
		}
		next.BreakUntil = time.Time{}
	case EventTaskCompleted:
		next.Phase = PhasePendingCooldown
		next.CooldownStartUntil = ev.CooldownStartAt
		next.CooldownDuration = ev.CooldownDuration
		next.CooldownUntil = time.Time{}
		next.BreakUntil = time.Time{}
	case EventTaskCancelled:
		next.Phase = PhaseIdle
		next.BreakUntil = time.Time{}
		next.CooldownStartUntil = time.Time{}
		next.CooldownUntil = time.Time{}
		next.CooldownDuration = 0
	case EventIdleEntered:
		next.IdleActive = true
	case EventIdleExited:
		next.IdleActive = false
	case EventScreenLocked:
		next.ScreenLocked = true
	case EventScreenUnlock:
		next.ScreenLocked = false
	case EventTick:
		// No direct fields; handled below by deadline reconciliation.
	}

	now := ev.At
	if now.IsZero() {
		now = time.Now()
	}

	switch next.Phase {
	case PhasePendingCooldown:
		if !next.CooldownStartUntil.IsZero() && !now.Before(next.CooldownStartUntil) {
			next.Phase = PhaseCooldown
			next.CooldownUntil = now.Add(next.CooldownDuration)
			next.CooldownStartUntil = time.Time{}
		}
	case PhaseCooldown:
		if !next.CooldownUntil.IsZero() && !now.Before(next.CooldownUntil) {
			next.Phase = PhaseIdle
			next.CooldownUntil = time.Time{}
			next.CooldownDuration = 0
		}
	case PhaseBreak:
		if !next.BreakUntil.IsZero() && !now.Before(next.BreakUntil) {
			next.Phase = PhaseActive
			next.BreakUntil = time.Time{}
		}
	}

	return next
}
