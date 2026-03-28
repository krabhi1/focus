package domain

import "time"

type Phase string

const (
	PhaseIdle            Phase = "idle"
	PhaseActive          Phase = "active"
	PhaseBreak           Phase = "break"
	PhasePendingCooldown Phase = "pending_cooldown"
	PhaseCooldown        Phase = "cooldown"
)

type Task struct {
	ID        int
	Title     string
	Duration  time.Duration
	StartTime time.Time
}

type State struct {
	Phase        Phase
	ScreenLocked bool

	CurrentTask *Task

	BreakUntil         time.Time
	CooldownStartUntil time.Time
	CooldownUntil      time.Time
	CooldownDuration   time.Duration
}

func InitialState() State {
	return State{Phase: PhaseIdle}
}

func (s State) NextWakeAt() time.Time {
	switch s.Phase {
	case PhasePendingCooldown:
		return s.CooldownStartUntil
	case PhaseCooldown:
		return s.CooldownUntil
	case PhaseBreak:
		return s.BreakUntil
	default:
		return time.Time{}
	}
}
