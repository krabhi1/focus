package state

import (
	"fmt"
	"focus/internal/sys"
	"time"
)

type breakPlan struct {
	startOffset time.Duration
	duration    time.Duration
}

func (s *DaemonState) NewTask(title string, duration time.Duration) (*Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTask != nil {
		if s.currentTask.Status == StatusBreak {
			remaining := s.breakRemainingLocked(time.Now())
			if remaining > 0 {
				return nil, fmt.Errorf("break active, wait %s before creating a new task", remaining.Round(time.Second))
			}
		}
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
	s.pruneHistoryToTodayLocked(time.Now())
	s.setupTimers(task)

	return task, nil
}

func (s *DaemonState) setupTimers(task *Task) {
	expireTime := task.StartTime.Add(task.Duration)
	actions := s.actionsLocked()
	cfg := GetRuntimeConfig()

	warningTime := time.Until(expireTime.Add(-5 * time.Minute))
	if warningTime > 0 {
		s.beforeExpireTimer = time.AfterFunc(warningTime, func() {
			actions.Notify("Task expiring soon", fmt.Sprintf("'%s' ends in 5m", task.Title))
			time.Sleep(2 * time.Second)
			actions.PlaySound("assets/task-ending.mp3")
		})
	}

	s.expireTimer = time.AfterFunc(task.Duration, func() {
		s.completeCurrentTask(task.ID)
	})

	if plan, ok := breakPlanForDuration(task.Duration); ok {
		warnAt := plan.startOffset - cfg.BreakWarning
		if warnAt > 0 {
			s.breakWarnTimer = time.AfterFunc(warnAt, func() {
				s.notifyBreakComing(task.ID)
			})
		}
		s.breakStartTimer = time.AfterFunc(plan.startOffset, func() {
			s.startBreak(task.ID, plan.duration)
		})
	}
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
	s.resumeIdleTrackingIfNeededLocked(time.Now())

	return task, nil
}

func (s *DaemonState) completeCurrentTask(taskID int) {
	s.mu.Lock()
	if s.currentTask == nil || s.currentTask.ID != taskID {
		s.mu.Unlock()
		return
	}

	task := s.currentTask
	task.Status = StatusCompleted
	s.beginCooldownLocked(task, time.Now())
	actions := s.actionsLocked()
	s.cleanupTask()
	s.mu.Unlock()

	if err := appendCompletedTaskToLog(task); err != nil {
		fmt.Printf("failed to persist completed task: %v\n", err)
	}

	actions.Notify("Task Complete", fmt.Sprintf("'%s' has finished.", task.Title))
	time.Sleep(5 * time.Second)
}

func (s *DaemonState) notifyBreakComing(taskID int) {
	s.mu.Lock()
	if s.currentTask == nil || s.currentTask.ID != taskID || s.currentTask.Status != StatusActive {
		s.mu.Unlock()
		return
	}
	title := s.currentTask.Title
	actions := s.actionsLocked()
	s.mu.Unlock()

	actions.Notify("Break Reminder", fmt.Sprintf("Break starts in %s for '%s'", GetRuntimeConfig().BreakWarning, title))
}

func (s *DaemonState) startBreak(taskID int, breakDuration time.Duration) {
	s.mu.Lock()
	if s.currentTask == nil || s.currentTask.ID != taskID || s.currentTask.Status != StatusActive {
		s.mu.Unlock()
		return
	}

	s.currentTask.Status = StatusBreak
	s.breakUntil = time.Now().Add(breakDuration)
	actions := s.actionsLocked()
	s.breakEndTimer = time.AfterFunc(breakDuration, func() {
		s.endBreak(taskID)
	})
	s.mu.Unlock()

	actions.Notify("Break Started", fmt.Sprintf("Break for %s has started", breakDuration.Round(time.Second)))
	actions.LockScreen()
}

func (s *DaemonState) endBreak(taskID int) {
	s.mu.Lock()
	if s.currentTask == nil || s.currentTask.ID != taskID || s.currentTask.Status != StatusBreak {
		s.mu.Unlock()
		return
	}

	s.currentTask.Status = StatusActive
	s.breakUntil = time.Time{}
	if s.breakEndTimer != nil {
		s.breakEndTimer.Stop()
		s.breakEndTimer = nil
	}
	if s.breakRelockTimer != nil {
		s.breakRelockTimer.Stop()
		s.breakRelockTimer = nil
	}
	s.breakRelockUntil = time.Time{}
	actions := s.actionsLocked()
	s.mu.Unlock()
	actions.UnlockScreen()
	actions.Notify("Break Complete", "Break period ended. Continue your task.")
	actions.PlaySound("assets/task-ending.mp3")
}

func (s *DaemonState) OnScreenUnlocked() {
	s.mu.Lock()
	now := time.Now()
	if s.currentTask == nil {
		if remaining := s.cooldownRemainingLocked(now); remaining > 0 {
			actions := s.actionsLocked()
			s.mu.Unlock()
			actions.Notify("Cooldown Active", fmt.Sprintf("Cooldown active. Wait %s before starting a new task.", remaining.Round(time.Second)))
			actions.LockScreen()
			return
		}
		s.resumeIdleTrackingIfNeededLocked(now)
		s.mu.Unlock()
		return
	}

	if s.currentTask.Status != StatusBreak {
		s.mu.Unlock()
		return
	}

	taskID := s.currentTask.ID
	breakRemaining := s.breakRemainingLocked(now)
	if breakRemaining <= 0 {
		s.mu.Unlock()
		return
	}

	actions := s.actionsLocked()
	notifyUser := s.breakRelockTimer == nil
	if s.breakRelockTimer != nil {
		s.breakRelockTimer.Stop()
	}
	relockDelay := GetRuntimeConfig().BreakRelockDelay
	s.breakRelockUntil = now.Add(relockDelay)
	s.breakRelockTimer = time.AfterFunc(relockDelay, func() {
		s.relockIfBreak(taskID)
	})
	s.mu.Unlock()

	if notifyUser {
		actions.Notify("Break Active", "Break is active. Locking again in 30 seconds.")
	}
}

func (s *DaemonState) OnScreenLocked() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.breakRelockTimer != nil {
		s.breakRelockTimer.Stop()
		s.breakRelockTimer = nil
	}
	s.breakRelockUntil = time.Time{}
	s.stopIdleTimersLocked()
	s.idleSince = time.Time{}
	s.notified = false
}

func (s *DaemonState) relockIfBreak(taskID int) {
	s.mu.Lock()
	if s.currentTask == nil || s.currentTask.ID != taskID || s.currentTask.Status != StatusBreak {
		s.mu.Unlock()
		return
	}
	if s.isSystemLocked {
		s.mu.Unlock()
		return
	}
	s.breakRelockUntil = time.Time{}
	actions := s.actionsLocked()
	s.mu.Unlock()

	actions.LockScreen()
}

func (s *DaemonState) cleanupTask() {
	s.currentTask = nil
	s.breakUntil = time.Time{}
	if s.beforeExpireTimer != nil {
		s.beforeExpireTimer.Stop()
		s.beforeExpireTimer = nil
	}
	if s.expireTimer != nil {
		s.expireTimer.Stop()
		s.expireTimer = nil
	}
	if s.breakWarnTimer != nil {
		s.breakWarnTimer.Stop()
		s.breakWarnTimer = nil
	}
	if s.breakStartTimer != nil {
		s.breakStartTimer.Stop()
		s.breakStartTimer = nil
	}
	if s.breakEndTimer != nil {
		s.breakEndTimer.Stop()
		s.breakEndTimer = nil
	}
	if s.breakRelockTimer != nil {
		s.breakRelockTimer.Stop()
		s.breakRelockTimer = nil
	}
	s.breakRelockUntil = time.Time{}
	s.stopIdleTimersLocked()
	s.idleSince = time.Time{}
	s.notified = false
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

	now := time.Now()
	taskRemaining := s.currentTask.StartTime.Add(s.currentTask.Duration).Sub(now)
	if taskRemaining < 0 {
		taskRemaining = 0
	}
	taskRemaining = taskRemaining.Round(time.Second)
	if s.currentTask.Status == StatusBreak {
		breakRemaining := s.breakRemainingLocked(now).Round(time.Second)
		status := fmt.Sprintf(
			"Task: %s | Status: break | Break remaining: %s | Task remaining: %s",
			s.currentTask.Title,
			breakRemaining,
			taskRemaining,
		)
		if relockRemaining := s.breakRelockRemainingLocked(now); relockRemaining > 0 {
			status += fmt.Sprintf(" | Re-lock in: %s", relockRemaining.Round(time.Second))
		}
		return status
	}

	return fmt.Sprintf("Task: %s | Remaining: %s", s.currentTask.Title, taskRemaining)
}

func (s *DaemonState) History() []Task {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneHistoryToTodayLocked(time.Now())

	history := make([]Task, 0, len(s.taskHistory))
	for _, task := range s.taskHistory {
		if task == nil {
			continue
		}
		history = append(history, *task)
	}
	return history
}

func (s *DaemonState) pruneHistoryToTodayLocked(now time.Time) {
	if len(s.taskHistory) == 0 {
		return
	}
	todayStart := startOfToday(now)
	todayEnd := todayStart.Add(24 * time.Hour)
	filtered := make([]*Task, 0, len(s.taskHistory))
	for _, task := range s.taskHistory {
		if task == nil {
			continue
		}
		if task.StartTime.Before(todayStart) || !task.StartTime.Before(todayEnd) {
			continue
		}
		filtered = append(filtered, task)
	}
	s.taskHistory = filtered
}

func (s *DaemonState) breakRemainingLocked(now time.Time) time.Duration {
	if s.breakUntil.IsZero() {
		return 0
	}
	if !now.Before(s.breakUntil) {
		return 0
	}
	return s.breakUntil.Sub(now)
}

func (s *DaemonState) breakRelockRemainingLocked(now time.Time) time.Duration {
	if s.breakRelockUntil.IsZero() {
		return 0
	}
	if !now.Before(s.breakRelockUntil) {
		return 0
	}
	return s.breakRelockUntil.Sub(now)
}

func (s *DaemonState) actionsLocked() sys.Actions {
	if s.actions == nil {
		return sys.RealActions{}
	}
	return s.actions
}

func breakPlanForDuration(duration time.Duration) (breakPlan, bool) {
	cfg := GetRuntimeConfig()

	switch {
	case duration >= 90*time.Minute:
		return breakPlan{
			startOffset: cfg.BreakDeepStart,
			duration:    cfg.BreakDeepDuration,
		}, true
	case duration >= 60*time.Minute:
		return breakPlan{
			startOffset: cfg.BreakLongStart,
			duration:    cfg.BreakLongDuration,
		}, true
	default:
		return breakPlan{}, false
	}
}
