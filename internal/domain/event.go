package domain

import "time"

type EventType string

const (
	EventTaskStarted   EventType = "task_started"
	EventTaskCancelled EventType = "task_cancelled"
	EventBreakStarted  EventType = "break_started"
	EventBreakEnded    EventType = "break_ended"
	EventTaskCompleted EventType = "task_completed"
	EventScreenLocked  EventType = "screen_locked"
	EventScreenUnlock  EventType = "screen_unlocked"
	EventTick          EventType = "tick"
)

type Event struct {
	Type EventType
	At   time.Time

	Task *Task

	CooldownStartAt  time.Time
	CooldownDuration time.Duration

	BreakUntil time.Time
}

type ActionType string

const (
	ActionLockScreen   ActionType = "lock_screen"
	ActionUnlockScreen ActionType = "unlock_screen"
	ActionNotify       ActionType = "notify"
	ActionPlaySound    ActionType = "play_sound"
	ActionStopSound    ActionType = "stop_sound"
)

type Action struct {
	Type    ActionType
	Title   string
	Message string
}

type Deadline struct {
	At   time.Time
	Type EventType
}
