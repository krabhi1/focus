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

	BreakDuration     = 5 * time.Minute
	LongBreakDuration = 10 * time.Minute
	DeepBreakDuration = 15 * time.Minute

	LongTaskBreakStartOffset = 25 * time.Minute
	DeepTaskBreakStartOffset = 45 * time.Minute
	BreakWarningOffset       = 2 * time.Minute
	BreakRelockDelay         = 30 * time.Second

	SocketPath             = "/tmp/focus.sock"
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
	mu                sync.Mutex
	currentTask       *Task
	taskHistory       []*Task
	beforeExpireTimer *time.Timer
	expireTimer       *time.Timer
	breakWarnTimer    *time.Timer
	breakStartTimer   *time.Timer
	breakEndTimer     *time.Timer
	breakRelockTimer  *time.Timer
	breakRelockUntil  time.Time
	breakUntil        time.Time
	cooldownUntil     time.Time
	cooldownPolicy    func(time.Duration) time.Duration
	isSystemLocked    bool
	idleSince         time.Time
	notified          bool
	actions           sys.Actions
}
