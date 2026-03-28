package domain

import "time"

type Result struct {
	State     State
	Deadlines []Deadline
	Actions   []Action
}

func Reduce(prev State, ev Event) Result {
	next := prev
	actions := make([]Action, 0, 2)
	deadlines := make([]Deadline, 0, 4)

	switch ev.Type {
	case EventTaskStarted:
		next.Phase = PhaseActive
		next.CurrentTask = ev.Task
		next.BreakUntil = time.Time{}
		next.CooldownStartUntil = time.Time{}
		next.CooldownUntil = time.Time{}
		next.CooldownDuration = 0
	case EventTaskCancelled:
		next = InitialState()
		next.CurrentTask = nil
	case EventBreakStarted:
		next.Phase = PhaseBreak
		next.BreakUntil = ev.BreakUntil
		actions = append(actions, Action{Type: ActionLockScreen, Title: "Break started"})
	case EventBreakEnded:
		if next.Phase == PhaseBreak {
			next.Phase = PhaseActive
		}
		next.BreakUntil = time.Time{}
		actions = append(actions, Action{Type: ActionUnlockScreen, Title: "Break ended"})
	case EventTaskCompleted:
		next.Phase = PhasePendingCooldown
		next.CurrentTask = nil
		next.CooldownStartUntil = ev.CooldownStartAt
		next.CooldownDuration = ev.CooldownDuration
		next.CooldownUntil = time.Time{}
		next.BreakUntil = time.Time{}
		actions = append(actions, Action{Type: ActionNotify, Title: "Task Complete", Message: "Task finished; cooldown pending."})
	case EventIdleEntered:
		next.IdleActive = true
	case EventIdleExited:
		next.IdleActive = false
	case EventScreenLocked:
		next.ScreenLocked = true
	case EventScreenUnlock:
		next.ScreenLocked = false
	case EventTick:
		// deadline reconciliation happens below
	}

	now := ev.At
	if now.IsZero() {
		now = time.Now()
	}

	switch next.Phase {
	case PhaseBreak:
		if !next.BreakUntil.IsZero() && !now.Before(next.BreakUntil) {
			next.Phase = PhaseActive
			next.BreakUntil = time.Time{}
			actions = append(actions, Action{Type: ActionUnlockScreen, Title: "Break ended"})
		}
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
			next.CurrentTask = nil
		}
	}

	return Result{State: next, Deadlines: deadlines, Actions: actions}
}
