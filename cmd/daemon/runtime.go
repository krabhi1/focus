package main

import (
	"fmt"
	"focus/internal/core"
	"focus/internal/state"
	"focus/internal/sys"
	"log"
	"os"
	"sync"
	"time"
)

type DaemonRuntime struct {
	mu      sync.Mutex
	actions sys.Actions
	core    *core.Runtime
	clock   runtimeClock
	trace   bool

	history  []*state.Task
	current  *state.Task
	nextID   int
	lockedAt time.Time

	taskExpireTimer    runtimeTimer
	breakWarnTimer     runtimeTimer
	breakStartTimer    runtimeTimer
	breakEndTimer      runtimeTimer
	relockTimer        runtimeTimer
	cooldownStartTimer runtimeTimer
	cooldownEndTimer   runtimeTimer
	idleWarnTimer      runtimeTimer
	idleLockTimer      runtimeTimer

	idleActive          bool
	systemLocked        bool
	idleSince           time.Time
	idleWarned          bool
	relockUntil         time.Time
	completionAlertStop chan struct{}
}

func NewDaemonRuntime(actions sys.Actions) *DaemonRuntime {
	if actions == nil {
		actions = sys.RealActions{}
	}
	return &DaemonRuntime{
		actions: actions,
		core:    core.NewRuntime(core.InitialState()),
		clock:   realRuntimeClock{},
		trace:   os.Getenv("FOCUS_TRACE_FLOW") == "1",
		nextID:  1,
	}
}

func NewDaemonRuntimeWithClock(actions sys.Actions, clock runtimeClock) *DaemonRuntime {
	rt := NewDaemonRuntime(actions)
	if clock != nil {
		rt.clock = clock
	}
	return rt
}

func (r *DaemonRuntime) SetTraceForTest(enabled bool) {
	r.trace = enabled
}

func (r *DaemonRuntime) Close() {
	r.mu.Lock()
	r.stopAllTimersLocked()
	r.stopCompletionAlertLocked()
	r.mu.Unlock()
	r.core.Close()
}

func (r *DaemonRuntime) CoreSnapshot() core.State {
	return r.core.Snapshot()
}

func (r *DaemonRuntime) Now() time.Time {
	return r.clock.Now()
}

func (r *DaemonRuntime) PublishCoreEvent(ev core.Event) {
	r.tracef("core_event type=%s at=%s", ev.Type, ev.At.Format(time.RFC3339Nano))
	r.core.Publish(ev)
}

func (r *DaemonRuntime) LoadHistoryFromDisk() error {
	tasks, err := state.LoadTodayHistory()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = tasksToPointers(tasks)
	r.nextID = 1
	for _, t := range r.history {
		if t != nil && t.ID >= r.nextID {
			r.nextID = t.ID + 1
		}
	}
	r.tracef("history_loaded count=%d", len(r.history))
	return nil
}

func (r *DaemonRuntime) HistoryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHistoryToTodayLocked(r.clock.Now())
	return len(r.history)
}

func (r *DaemonRuntime) StartTask(title string, duration time.Duration) (*state.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := startDecisionFromCore(r.core.Snapshot()); err != nil {
		return nil, err
	}

	now := r.clock.Now()
	task := &state.Task{
		ID:        r.nextID,
		Title:     title,
		Duration:  duration,
		StartTime: now,
		Status:    state.StatusActive,
	}
	r.nextID++
	r.current = task
	r.history = append(r.history, task)
	r.pruneHistoryToTodayLocked(now)

	r.stopTaskTimersLocked()
	r.armTaskTimersLocked(task)

	r.tracef("task_started id=%d title=%q duration=%s", task.ID, task.Title, task.Duration)
	r.PublishCoreEvent(core.Event{Type: core.EventTaskStarted, At: now})
	return task, nil
}

func (r *DaemonRuntime) CancelCurrentTask() (*state.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil, fmt.Errorf("no active task to cancel")
	}
	if r.clock.Since(r.current.StartTime) > state.TaskLockedWaitDuration {
		return nil, fmt.Errorf("task is locked (grace period of %v expired)", state.TaskLockedWaitDuration)
	}

	task := r.current
	task.Status = state.StatusCancelled
	r.current = nil
	r.stopTaskTimersLocked()
	r.relockUntil = time.Time{}
	r.stopCompletionAlertLocked()

	now := r.clock.Now()
	r.tracef("task_cancelled id=%d title=%q", task.ID, task.Title)
	r.PublishCoreEvent(core.Event{Type: core.EventTaskCancelled, At: now})
	r.resumeIdleTrackingIfNeededLocked(now)
	return task, nil
}

func (r *DaemonRuntime) Status() string {
	r.mu.Lock()
	current := r.current
	relockUntil := r.relockUntil
	r.mu.Unlock()

	now := r.clock.Now()
	snap := r.core.Snapshot()
	switch snap.Phase {
	case core.PhasePendingCooldown:
		remaining := snap.CooldownStartUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown starting | Remaining: %s", remaining.Round(time.Second))
	case core.PhaseCooldown:
		remaining := snap.CooldownUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown active | Remaining: %s", remaining.Round(time.Second))
	case core.PhaseBreak:
		if current == nil {
			return "Status: break"
		}
		taskRemaining := current.StartTime.Add(current.Duration).Sub(now)
		if taskRemaining < 0 {
			taskRemaining = 0
		}
		breakRemaining := snap.BreakUntil.Sub(now)
		if breakRemaining < 0 {
			breakRemaining = 0
		}
		status := fmt.Sprintf("Task: %s | Status: break | Break remaining: %s | Task remaining: %s", current.Title, breakRemaining.Round(time.Second), taskRemaining.Round(time.Second))
		if !relockUntil.IsZero() && now.Before(relockUntil) {
			status += fmt.Sprintf(" | Re-lock in: %s", relockUntil.Sub(now).Round(time.Second))
		}
		return status
	case core.PhaseActive:
		if current == nil {
			return "Task active"
		}
		remaining := current.StartTime.Add(current.Duration).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Task: %s | Remaining: %s", current.Title, remaining.Round(time.Second))
	default:
		return "Idle"
	}
}

func (r *DaemonRuntime) History() []state.Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHistoryToTodayLocked(r.clock.Now())
	out := make([]state.Task, 0, len(r.history))
	for _, task := range r.history {
		if task != nil {
			out = append(out, *task)
		}
	}
	return out
}

func (r *DaemonRuntime) SetSystemLocked(locked bool) {
	r.mu.Lock()
	r.systemLocked = locked
	r.mu.Unlock()
}

func (r *DaemonRuntime) OnIdleEntered() {
	r.mu.Lock()
	now := r.clock.Now()
	r.idleActive = true
	phase := r.core.Snapshot().Phase

	if r.current != nil || r.systemLocked || phase == core.PhasePendingCooldown {
		r.stopIdleTimersLocked()
		r.idleSince = time.Time{}
		r.idleWarned = false
		r.mu.Unlock()
		return
	}
	if phase == core.PhaseCooldown {
		r.stopIdleTimersLocked()
		r.idleSince = time.Time{}
		r.idleWarned = false
		r.mu.Unlock()
		r.lockScreen()
		return
	}

	r.armIdleTimersLocked(now)
	r.mu.Unlock()
}

func (r *DaemonRuntime) OnIdleExited() {
	r.mu.Lock()
	r.idleActive = false
	r.stopIdleTimersLocked()
	r.stopCompletionAlertLocked()
	r.idleSince = time.Time{}
	r.idleWarned = false
	r.mu.Unlock()
}

func (r *DaemonRuntime) OnScreenLocked() {
	r.mu.Lock()
	if r.relockTimer != nil {
		r.relockTimer.Stop()
		r.relockTimer = nil
	}
	r.relockUntil = time.Time{}
	r.stopIdleTimersLocked()
	r.idleSince = time.Time{}
	r.idleWarned = false
	r.mu.Unlock()
}

func (r *DaemonRuntime) OnScreenUnlocked() {
	r.mu.Lock()
	r.stopCompletionAlertLocked()
	now := r.clock.Now()
	snap := r.core.Snapshot()
	cfg := state.GetRuntimeConfig()

	if r.current == nil {
		if snap.Phase == core.PhasePendingCooldown {
			r.mu.Unlock()
			return
		}
		if snap.Phase == core.PhaseCooldown {
			relockDelay := cfg.RelockDelay
			if r.relockTimer != nil {
				r.relockTimer.Stop()
			}
			r.relockUntil = now.Add(relockDelay)
			r.relockTimer = r.clock.AfterFunc(relockDelay, func() {
				r.lockScreen()
			})
			r.mu.Unlock()
			r.notify("Cooldown Active", fmt.Sprintf("Cooldown active. Locking again in %s.", relockDelay.Round(time.Second)))
			return
		}
		r.resumeIdleTrackingIfNeededLocked(now)
		r.mu.Unlock()
		return
	}

	if snap.Phase != core.PhaseBreak {
		r.mu.Unlock()
		return
	}

	notifyUser := r.relockTimer == nil
	if r.relockTimer != nil {
		r.relockTimer.Stop()
	}
	relockDelay := cfg.RelockDelay
	r.relockUntil = now.Add(relockDelay)
	r.relockTimer = r.clock.AfterFunc(relockDelay, func() {
		r.relockIfBreak()
	})
	r.mu.Unlock()

	if notifyUser {
		r.notify("Break Active", fmt.Sprintf("Break is active. Locking again in %s.", relockDelay.Round(time.Second)))
	}
}

func (r *DaemonRuntime) armTaskTimersLocked(task *state.Task) {
	cfg := state.GetRuntimeConfig()
	expireAt := task.StartTime.Add(task.Duration)
	remaining := r.clock.Until(expireAt)
	if remaining < 0 {
		remaining = 0
	}
	r.taskExpireTimer = r.clock.AfterFunc(remaining, func() {
		r.completeCurrentTask(task.ID)
	})
	r.tracef("timer_set name=task_expire in=%s task_id=%d", remaining, task.ID)

	if plan, ok := breakPlanForDuration(task.Duration, cfg); ok {
		warnAt := plan.startOffset - cfg.BreakWarning
		if warnAt > 0 {
			r.breakWarnTimer = r.clock.AfterFunc(warnAt, func() {
				r.notifyBreakComing(task.ID)
			})
			r.tracef("timer_set name=break_warn in=%s task_id=%d", warnAt, task.ID)
		}
		r.breakStartTimer = r.clock.AfterFunc(plan.startOffset, func() {
			r.startBreak(task.ID, plan.duration)
		})
		r.tracef("timer_set name=break_start in=%s task_id=%d break_duration=%s", plan.startOffset, task.ID, plan.duration)
	}
}

func (r *DaemonRuntime) completeCurrentTask(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID {
		r.mu.Unlock()
		return
	}
	task := r.current
	task.Status = state.StatusCompleted
	r.current = nil
	r.stopTaskTimersLocked()
	r.relockUntil = time.Time{}

	now := r.clock.Now()
	cfg := state.GetRuntimeConfig()
	cooldownDuration := cooldownDurationFor(task.Duration, cfg)
	cooldownStartAt := now.Add(cfg.CooldownStartDelay)
	r.tracef("task_completed id=%d title=%q cooldown_start=%s cooldown_duration=%s", task.ID, task.Title, cooldownStartAt.Format(time.RFC3339Nano), cooldownDuration)
	r.PublishCoreEvent(core.Event{Type: core.EventTaskCompleted, At: now, CooldownStartAt: cooldownStartAt, CooldownDuration: cooldownDuration})

	if r.cooldownStartTimer != nil {
		r.cooldownStartTimer.Stop()
	}
	r.cooldownStartTimer = r.clock.AfterFunc(cfg.CooldownStartDelay, func() {
		r.lockScreen()
	})
	r.tracef("timer_set name=cooldown_start in=%s", cfg.CooldownStartDelay)
	if r.cooldownEndTimer != nil {
		r.cooldownEndTimer.Stop()
	}
	r.cooldownEndTimer = r.clock.AfterFunc(cfg.CooldownStartDelay+cooldownDuration, func() {
		r.startCompletionAlert()
	})
	r.tracef("timer_set name=cooldown_end in=%s", cfg.CooldownStartDelay+cooldownDuration)
	r.mu.Unlock()

	if err := state.AppendCompletedTask(*task); err != nil {
		fmt.Printf("failed to persist completed task: %v\n", err)
	}
	r.notify("Task Complete", fmt.Sprintf("'%s' has finished. Cooldown starts in %s; locking screen.", task.Title, cfg.CooldownStartDelay.Round(time.Second)))
}

func (r *DaemonRuntime) notifyBreakComing(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID || r.core.Snapshot().Phase != core.PhaseActive {
		r.mu.Unlock()
		return
	}
	title := r.current.Title
	warning := state.GetRuntimeConfig().BreakWarning
	r.mu.Unlock()
	r.notify("Break Reminder", fmt.Sprintf("Break starts in %s for '%s'", warning, title))
}

func (r *DaemonRuntime) startBreak(taskID int, breakDuration time.Duration) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID || r.core.Snapshot().Phase != core.PhaseActive {
		r.mu.Unlock()
		return
	}
	now := r.clock.Now()
	breakUntil := now.Add(breakDuration)
	r.tracef("break_started task_id=%d until=%s", taskID, breakUntil.Format(time.RFC3339Nano))
	r.PublishCoreEvent(core.Event{Type: core.EventBreakStarted, At: now, BreakUntil: breakUntil})
	r.breakEndTimer = r.clock.AfterFunc(breakDuration, func() {
		r.endBreak(taskID)
	})
	r.tracef("timer_set name=break_end in=%s task_id=%d", breakDuration, taskID)
	r.mu.Unlock()
	r.notify("Break Started", fmt.Sprintf("Break for %s has started", breakDuration.Round(time.Second)))
	r.lockScreen()
}

func (r *DaemonRuntime) endBreak(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID || r.core.Snapshot().Phase != core.PhaseBreak {
		r.mu.Unlock()
		return
	}
	if r.breakEndTimer != nil {
		r.breakEndTimer.Stop()
		r.breakEndTimer = nil
	}
	if r.relockTimer != nil {
		r.relockTimer.Stop()
		r.relockTimer = nil
	}
	r.relockUntil = time.Time{}
	r.tracef("break_ended task_id=%d", taskID)
	r.PublishCoreEvent(core.Event{Type: core.EventBreakEnded, At: r.clock.Now()})
	r.mu.Unlock()
	r.unlockScreen()
	r.notify("Break Complete", "Break period ended. Continue your task.")
	r.startCompletionAlert()
}

func (r *DaemonRuntime) relockIfBreak() {
	r.mu.Lock()
	if r.core.Snapshot().Phase != core.PhaseBreak {
		r.mu.Unlock()
		return
	}
	if r.systemLocked {
		r.mu.Unlock()
		return
	}
	r.relockUntil = time.Time{}
	r.mu.Unlock()
	r.lockScreen()
}

func (r *DaemonRuntime) startCompletionAlert() {
	r.mu.Lock()
	if !r.idleActive {
		r.mu.Unlock()
		r.playSound("assets/task-ending.mp3")
		return
	}
	if r.completionAlertStop != nil {
		r.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	r.completionAlertStop = stopCh
	repeatInterval := state.GetRuntimeConfig().CompletionAlertRepeatInterval
	r.tracef("completion_alert_loop_start interval=%s", repeatInterval)
	r.mu.Unlock()

	go r.runCompletionAlertLoop(stopCh, repeatInterval)
}

func (r *DaemonRuntime) runCompletionAlertLoop(stopCh <-chan struct{}, repeatInterval time.Duration) {
	defer func() {
		r.mu.Lock()
		if r.completionAlertStop == stopCh {
			r.completionAlertStop = nil
		}
		r.mu.Unlock()
		r.tracef("completion_alert_loop_stop")
	}()

	for {
		select {
		case <-stopCh:
			return
		default:
		}
		r.playSound("assets/task-ending.mp3")
		timer := r.clock.NewTimer(repeatInterval)
		select {
		case <-stopCh:
			if !timer.Stop() {
				<-timer.C()
			}
			return
		case <-timer.C():
		}
		r.mu.Lock()
		running := r.completionAlertStop == stopCh && r.idleActive
		r.mu.Unlock()
		if !running {
			return
		}
	}
}

func (r *DaemonRuntime) armIdleTimersLocked(now time.Time) {
	cfg := state.GetRuntimeConfig()
	r.stopIdleTimersLocked()
	r.idleSince = now
	r.idleWarned = false
	idleSince := r.idleSince
	if cfg.IdleWarnAfter > 0 {
		r.idleWarnTimer = r.clock.AfterFunc(cfg.IdleWarnAfter, func() {
			r.notifyIfStillIdle(idleSince)
		})
	}
	if cfg.IdleLockAfter > 0 {
		r.idleLockTimer = r.clock.AfterFunc(cfg.IdleLockAfter, func() {
			r.lockIfStillIdle(idleSince)
		})
	}
}

func (r *DaemonRuntime) notifyIfStillIdle(idleSince time.Time) {
	r.mu.Lock()
	snap := r.core.Snapshot()
	if !r.idleActive || !r.idleSince.Equal(idleSince) || r.current != nil || r.systemLocked || snap.Phase == core.PhaseCooldown || snap.Phase == core.PhasePendingCooldown {
		r.mu.Unlock()
		return
	}
	if r.idleWarned {
		r.mu.Unlock()
		return
	}
	r.idleWarned = true
	cfg := state.GetRuntimeConfig()
	r.mu.Unlock()
	remaining := (cfg.IdleLockAfter - cfg.IdleWarnAfter).Round(time.Second)
	r.notify("Idle Warning", "No task active. Locking in "+remaining.String()+".")
}

func (r *DaemonRuntime) lockIfStillIdle(idleSince time.Time) {
	r.mu.Lock()
	snap := r.core.Snapshot()
	if !r.idleActive || !r.idleSince.Equal(idleSince) || r.current != nil || r.systemLocked || snap.Phase == core.PhaseCooldown || snap.Phase == core.PhasePendingCooldown {
		r.mu.Unlock()
		return
	}
	r.stopIdleTimersLocked()
	r.idleSince = time.Time{}
	r.idleWarned = false
	r.mu.Unlock()
	r.lockScreen()
}

func (r *DaemonRuntime) resumeIdleTrackingIfNeededLocked(now time.Time) {
	snap := r.core.Snapshot()
	if !r.idleActive || r.current != nil || r.systemLocked || snap.Phase == core.PhaseCooldown || snap.Phase == core.PhasePendingCooldown {
		return
	}
	if r.idleSince.IsZero() || r.idleWarnTimer == nil || r.idleLockTimer == nil {
		r.armIdleTimersLocked(now)
	}
}

func (r *DaemonRuntime) stopIdleTimersLocked() {
	if r.idleWarnTimer != nil {
		r.idleWarnTimer.Stop()
		r.idleWarnTimer = nil
	}
	if r.idleLockTimer != nil {
		r.idleLockTimer.Stop()
		r.idleLockTimer = nil
	}
}

func (r *DaemonRuntime) stopTaskTimersLocked() {
	if r.taskExpireTimer != nil {
		r.taskExpireTimer.Stop()
		r.taskExpireTimer = nil
	}
	if r.breakWarnTimer != nil {
		r.breakWarnTimer.Stop()
		r.breakWarnTimer = nil
	}
	if r.breakStartTimer != nil {
		r.breakStartTimer.Stop()
		r.breakStartTimer = nil
	}
	if r.breakEndTimer != nil {
		r.breakEndTimer.Stop()
		r.breakEndTimer = nil
	}
	if r.relockTimer != nil {
		r.relockTimer.Stop()
		r.relockTimer = nil
	}
	if r.cooldownStartTimer != nil {
		r.cooldownStartTimer.Stop()
		r.cooldownStartTimer = nil
	}
	if r.cooldownEndTimer != nil {
		r.cooldownEndTimer.Stop()
		r.cooldownEndTimer = nil
	}
}

func (r *DaemonRuntime) stopAllTimersLocked() {
	r.stopTaskTimersLocked()
	r.stopIdleTimersLocked()
}

func (r *DaemonRuntime) stopCompletionAlertLocked() {
	if r.completionAlertStop != nil {
		close(r.completionAlertStop)
		r.completionAlertStop = nil
	}
}

func (r *DaemonRuntime) lockScreen() {
	r.tracef("action lock_screen")
	r.actions.LockScreen()
}

func (r *DaemonRuntime) unlockScreen() {
	r.tracef("action unlock_screen")
	r.actions.UnlockScreen()
}

func (r *DaemonRuntime) playSound(path string) {
	r.tracef("action play_sound path=%s", path)
	r.actions.PlaySound(path)
}

func (r *DaemonRuntime) notify(title, message string) {
	r.tracef("action notify title=%q", title)
	r.actions.Notify(title, message)
}

func (r *DaemonRuntime) tracef(format string, args ...any) {
	if !r.trace {
		return
	}
	log.Printf("flow "+format, args...)
}

type breakPlan struct {
	startOffset time.Duration
	duration    time.Duration
}

func breakPlanForDuration(duration time.Duration, cfg state.RuntimeConfig) (breakPlan, bool) {
	switch {
	case duration >= cfg.TaskDeep:
		return breakPlan{startOffset: cfg.BreakDeepStart, duration: cfg.BreakDeepDuration}, true
	case duration >= cfg.TaskLong:
		return breakPlan{startOffset: cfg.BreakLongStart, duration: cfg.BreakLongDuration}, true
	default:
		return breakPlan{}, false
	}
}

func cooldownDurationFor(duration time.Duration, cfg state.RuntimeConfig) time.Duration {
	switch {
	case duration >= cfg.TaskDeep:
		return cfg.CooldownDeep
	case duration >= cfg.TaskLong:
		return cfg.CooldownLong
	default:
		return cfg.CooldownShort
	}
}

func tasksToPointers(tasks []state.Task) []*state.Task {
	if len(tasks) == 0 {
		return nil
	}
	result := make([]*state.Task, 0, len(tasks))
	for i := range tasks {
		taskCopy := tasks[i]
		result = append(result, &taskCopy)
	}
	return result
}

func (r *DaemonRuntime) pruneHistoryToTodayLocked(now time.Time) {
	if len(r.history) == 0 {
		return
	}
	todayStart := startOfToday(now)
	todayEnd := todayStart.Add(24 * time.Hour)
	filtered := make([]*state.Task, 0, len(r.history))
	for _, task := range r.history {
		if task == nil {
			continue
		}
		if task.StartTime.Before(todayStart) || !task.StartTime.Before(todayEnd) {
			continue
		}
		filtered = append(filtered, task)
	}
	r.history = filtered
}

func startOfToday(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
