package state

import (
	"sync"
	"time"

	"focus/internal/sys"
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
	BreakRelockDelay         = 30 * time.Second

	IdleWarningAfter    = 30 * time.Second
	IdleLockAfter       = 60 * time.Second
	IdleMonitorInterval = 15 * time.Second

	TaskLockedWaitDuration = 2 * time.Minute
)

type Task struct {
	ID        int
	Title     string
	Duration  time.Duration
	StartTime time.Time
	Status    TaskStatus
}

type DaemonState struct {
	mu                  sync.Mutex
	currentTask         *Task
	taskHistory         []*Task
	beforeExpireTimer   *time.Timer
	expireTimer         *time.Timer
	breakWarnTimer      *time.Timer
	breakStartTimer     *time.Timer
	breakEndTimer       *time.Timer
	breakRelockTimer    *time.Timer
	cooldownTimer       *time.Timer
	idleWarnTimer       *time.Timer
	idleLockTimer       *time.Timer
	completionAlertStop chan struct{}
	breakRelockUntil    time.Time
	breakUntil          time.Time
	cooldownUntil       time.Time
	cooldownPolicy      func(time.Duration) time.Duration
	isSystemLocked      bool
	idleActive          bool
	idleSince           time.Time
	notified            bool
	actions             sys.Actions
}
