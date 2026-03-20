package state

import (
	"fmt"
	"focus/internal/sys"
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

// global is private to enforce using methods
var global = &DaemonState{
	taskHistory:    []*Task{},
	isSystemLocked: false,
}

// Get returns the singleton instance
func Get() *DaemonState {
	return global
}

func (s *DaemonState) SetSystemLocked(locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isSystemLocked = locked
}
func (s *DaemonState) IsSystemLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSystemLocked
}

func (s *DaemonState) NewTask(title string, duration time.Duration) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTask != nil {
		return nil, fmt.Errorf("a task '%s' is already active", s.currentTask.Title)
	}

	if remaining := s.cooldownRemainingLocked(time.Now()); remaining > 0 {
		return nil, fmt.Errorf("cooldown active, wait %s before creating a new task", remaining.Round(time.Second))
	}

	task := &Task{
		ID:        len(s.taskHistory) + 1,
		Title:     title,
		Duration:  duration,
		StartTime: time.Now(),
		Status:    StatusActive,
	}

	s.currentTask = task
	s.taskHistory = append(s.taskHistory, task)

	// Setup Timers
	s.setupTimers(task)

	return task, nil
}

func (s *DaemonState) setupTimers(task *Task) {
	expireTime := task.StartTime.Add(task.Duration)

	// Warning before the task ends.
	warningTime := time.Until(expireTime.Add(-5 * time.Minute))
	if warningTime > 0 {
		s.beforeExpireTimer = time.AfterFunc(warningTime, func() {
			sys.Notify("Task expiring soon", fmt.Sprintf("'%s' ends in 5m", task.Title))
			time.Sleep(2 * time.Second)
			sys.PlaySound("assets/task-ending.mp3")
		})
	}

	// Final Expiration
	s.expireTimer = time.AfterFunc(task.Duration, func() {
		s.completeCurrentTask(task.Title)
	})
}

func (s *DaemonState) CancelCurrentTask() (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTask == nil {
		return nil, fmt.Errorf("no active task to cancel")
	}

	// Logic: If current time is AFTER (StartTime + 2m), it is locked.
	if time.Now().After(s.currentTask.StartTime.Add(TaskLockedWaitDuration)) {
		return nil, fmt.Errorf("task is locked (grace period of %v expired)", TaskLockedWaitDuration)
	}

	task := s.currentTask
	task.Status = StatusCancelled
	s.cleanupTask()

	return task, nil
}

func (s *DaemonState) completeCurrentTask(title string) {
	s.mu.Lock()
	// Validation: Ensure the task wasn't already replaced or cancelled
	if s.currentTask == nil || s.currentTask.Title != title {
		s.mu.Unlock()
		return
	}

	task := s.currentTask
	task.Status = StatusCompleted
	s.beginCooldownLocked(task, time.Now())
	s.cleanupTask()
	s.mu.Unlock()

	// Perform UI actions outside the lock to keep daemon responsive
	sys.Notify("Task Complete", fmt.Sprintf("'%s' has finished.", title))
	time.Sleep(5 * time.Second)
	// sys.LockScreen()
}

// Internal helper to reset state and stop timers
func (s *DaemonState) cleanupTask() {
	s.currentTask = nil
	if s.beforeExpireTimer != nil {
		s.beforeExpireTimer.Stop()
		s.beforeExpireTimer = nil
	}
	if s.expireTimer != nil {
		s.expireTimer.Stop()
		s.expireTimer = nil
	}
}

func (s *DaemonState) GetStatus() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTask == nil {
		if remaining := s.cooldownRemainingLocked(time.Now()); remaining > 0 {
			return fmt.Sprintf("Cooldown active | Remaining: %s", remaining.Round(time.Second))
		}
		return "Idle"
	}
	remaining := time.Until(s.currentTask.StartTime.Add(s.currentTask.Duration))
	return fmt.Sprintf("Task: %s | Remaining: %s", s.currentTask.Title, remaining.Round(time.Second))
}

func (s *DaemonState) beginCooldownLocked(task *Task, now time.Time) {
	s.cooldownUntil = now.Add(cooldownDurationFor(task.Duration))
}

func (s *DaemonState) cooldownRemainingLocked(now time.Time) time.Duration {
	if s.cooldownUntil.IsZero() {
		return 0
	}
	if !now.Before(s.cooldownUntil) {
		s.cooldownUntil = time.Time{}
		return 0
	}
	return s.cooldownUntil.Sub(now)
}

func cooldownDurationFor(duration time.Duration) time.Duration {
	switch {
	case duration >= 90*time.Minute:
		return DeepBreakDuration
	case duration >= 60*time.Minute:
		return LongBreakDuration
	default:
		return BreakDuration
	}
}

func (s *DaemonState) StartIdleMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()

		// 1. Reset if busy
		if s.currentTask != nil || s.isSystemLocked {
			s.idleSince = time.Time{} // Reset timestamp
			s.notified = false        // Reset notification flag
			s.mu.Unlock()
			continue
		}

		// 2. Mark start of idleness
		if s.idleSince.IsZero() {
			s.idleSince = time.Now()
		}

		elapsed := time.Since(s.idleSince)

		// 3. Logic Check
		if elapsed >= 5*time.Minute {
			sys.LockScreen()
			s.idleSince = time.Time{} // Stop checking until unlock
		} else if elapsed >= 3*time.Minute && !s.notified {
			sys.Notify("Idle Warning", "No task active. Locking in 2 minute.")
			s.notified = true
		}

		s.mu.Unlock()
	}
}
