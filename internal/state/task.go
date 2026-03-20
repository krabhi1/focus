package state

import (
	"fmt"
	"focus/internal/sys"
	"time"
)

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

	s.setupTimers(task)

	return task, nil
}

func (s *DaemonState) setupTimers(task *Task) {
	expireTime := task.StartTime.Add(task.Duration)
	actions := s.actions
	if actions == nil {
		actions = sys.RealActions{}
	}

	warningTime := time.Until(expireTime.Add(-5 * time.Minute))
	if warningTime > 0 {
		s.beforeExpireTimer = time.AfterFunc(warningTime, func() {
			actions.Notify("Task expiring soon", fmt.Sprintf("'%s' ends in 5m", task.Title))
			time.Sleep(2 * time.Second)
			actions.PlaySound("assets/task-ending.mp3")
		})
	}

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
	if s.currentTask == nil || s.currentTask.Title != title {
		s.mu.Unlock()
		return
	}

	task := s.currentTask
	task.Status = StatusCompleted
	s.beginCooldownLocked(task, time.Now())
	actions := s.actions
	if actions == nil {
		actions = sys.RealActions{}
	}
	s.cleanupTask()
	s.mu.Unlock()

	actions.Notify("Task Complete", fmt.Sprintf("'%s' has finished.", title))
	time.Sleep(5 * time.Second)
}

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

func (s *DaemonState) ResetForTest() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.beforeExpireTimer != nil {
		s.beforeExpireTimer.Stop()
		s.beforeExpireTimer = nil
	}
	if s.expireTimer != nil {
		s.expireTimer.Stop()
		s.expireTimer = nil
	}

	s.currentTask = nil
	s.taskHistory = []*Task{}
	s.cooldownUntil = time.Time{}
	s.isSystemLocked = false
	s.idleSince = time.Time{}
	s.notified = false
}
