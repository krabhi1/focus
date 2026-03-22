package core

import "time"

type Phase string

const (
	PhaseIdle            Phase = "idle"
	PhaseActive          Phase = "active"
	PhaseBreak           Phase = "break"
	PhasePendingCooldown Phase = "pending_cooldown"
	PhaseCooldown        Phase = "cooldown"
)

type EventType string

const (
	EventTaskStarted   EventType = "task_started"
	EventTaskCancelled EventType = "task_cancelled"
	EventBreakStarted  EventType = "break_started"
	EventBreakEnded    EventType = "break_ended"
	EventTaskCompleted EventType = "task_completed"
	EventIdleEntered   EventType = "idle_entered"
	EventIdleExited    EventType = "idle_exited"
	EventScreenLocked  EventType = "screen_locked"
	EventScreenUnlock  EventType = "screen_unlocked"
	EventTick          EventType = "tick"
)

type Event struct {
	Type EventType
	At   time.Time

	// Task completion metadata for pending-cooldown -> cooldown transition.
	CooldownStartAt  time.Time
	CooldownDuration time.Duration

	// Break metadata.
	BreakUntil time.Time
}

type State struct {
	Phase        Phase
	IdleActive   bool
	ScreenLocked bool

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
	default:
		return time.Time{}
	}
}
