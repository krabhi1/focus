package state

import (
	"sync"
	"time"
)

type TaskStatus string

const (
	StatusActive    TaskStatus = "active"
	StatusCompleted TaskStatus = "completed"
	StatusCancelled TaskStatus = "cancelled"

	BreakDuration     = 5 * time.Minute
	LongBreakDuration = 10 * time.Minute
	DeepBreakDuration = 15 * time.Minute

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
	cooldownUntil     time.Time
	isSystemLocked    bool
	idleSince         time.Time
	notified          bool
}
