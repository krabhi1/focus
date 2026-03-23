package state

import (
	"time"
)

type TaskStatus string

const (
	StatusActive    TaskStatus = "active"
	StatusBreak     TaskStatus = "break"
	StatusCompleted TaskStatus = "completed"
	StatusCancelled TaskStatus = "cancelled"

	ShortCooldownDuration = 5 * time.Minute
	LongCooldownDuration  = 10 * time.Minute
	DeepCooldownDuration  = 15 * time.Minute

	LongTaskBreakStartOffset = 25 * time.Minute
	DeepTaskBreakStartOffset = 45 * time.Minute
	BreakWarningOffset       = 2 * time.Minute
	RelockDelay              = 0 * time.Second
	CooldownStartDelay       = 2 * time.Minute

	IdleWarningAfter = 30 * time.Second
	IdleLockAfter    = 2 * time.Minute

	TaskLockedWaitDuration = 2 * time.Minute
)

type Task struct {
	ID        int
	Title     string
	Duration  time.Duration
	StartTime time.Time
	Status    TaskStatus
}
