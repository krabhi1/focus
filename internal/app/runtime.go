package app

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"focus/internal/domain"
	"focus/internal/effects"
	"focus/internal/scheduler"
	"focus/internal/status"
	"focus/internal/storage"
)

type Runtime struct {
	mu      sync.Mutex
	actions effects.Actions
	clock   runtimeClock
	timers  *scheduler.CallbackLoop
	trace   bool

	state   domain.State
	history []domain.Task
	current *domain.Task
	nextID  int

	idleActive           bool
	systemLocked         bool
	noTaskSince          time.Time
	noTaskWarned         bool
	relockUntil          time.Time
	completionAlertToken uint64
	deadlines            map[string]*scheduler.CallbackHandle
}

func NewRuntime(actions effects.Actions) *Runtime {
	if actions == nil {
		actions = effects.RealActions{}
	}
	rt := &Runtime{
		actions:   actions,
		clock:     realRuntimeClock{},
		timers:    scheduler.NewCallbackLoop(schedulerClockAdapter{clock: realRuntimeClock{}}),
		trace:     os.Getenv("FOCUS_TRACE_FLOW") == "1",
		state:     domain.InitialState(),
		nextID:    1,
		deadlines: map[string]*scheduler.CallbackHandle{},
	}
	rt.mu.Lock()
	rt.resumeNoTaskTrackingIfNeededLocked(rt.clock.Now())
	rt.mu.Unlock()
	return rt
}

func (r *Runtime) SetTraceForTest(enabled bool) {
	r.trace = enabled
}

func (r *Runtime) Close() {
	r.mu.Lock()
	r.stopAllTimersLocked()
	r.stopCompletionAlertLocked()
	r.mu.Unlock()
	if r.timers != nil {
		r.timers.Stop()
	}
}

func (r *Runtime) Snapshot() domain.State {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *Runtime) Now() time.Time {
	return r.clock.Now()
}

func (r *Runtime) LoadHistoryFromDisk() error {
	tasks, err := storage.LoadTodayHistory()
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.history = append([]domain.Task(nil), tasks...)
	r.nextID = 1
	for _, t := range r.history {
		if t.ID >= r.nextID {
			r.nextID = t.ID + 1
		}
	}
	r.tracef("history_loaded count=%d", len(r.history))
	return nil
}

func (r *Runtime) History() []domain.Task {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHistoryToTodayLocked(r.clock.Now())
	out := make([]domain.Task, 0, len(r.history))
	out = append(out, r.history...)
	return out
}

func (r *Runtime) HistoryCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pruneHistoryToTodayLocked(r.clock.Now())
	return len(r.history)
}

func (r *Runtime) StartTask(title string, duration time.Duration) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.startDecisionLocked(); err != nil {
		return nil, err
	}

	now := r.clock.Now()
	task := &domain.Task{
		ID:        r.nextID,
		Title:     title,
		Duration:  duration,
		StartTime: now,
	}
	r.nextID++
	r.current = task
	r.history = append(r.history, *task)
	r.pruneHistoryToTodayLocked(now)

	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTaskStarted, At: now, Task: task}).State
	r.stopTaskTimersLocked()
	r.stopNoTaskTimersLocked()
	r.armTaskTimersLocked(task)
	r.tracef("task_started id=%d title=%q duration=%s", task.ID, task.Title, task.Duration)
	return task, nil
}

func (r *Runtime) CancelCurrentTask() (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.current == nil {
		return nil, fmt.Errorf("no active task to cancel")
	}
	if r.clock.Since(r.current.StartTime) > storage.TaskLockedWaitDuration {
		return nil, fmt.Errorf("task is locked (grace period of %v expired)", storage.TaskLockedWaitDuration)
	}

	task := r.current
	r.current = nil
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTaskCancelled, At: r.clock.Now()}).State
	r.stopTaskTimersLocked()
	r.stopNoTaskTimersLocked()
	r.relockUntil = time.Time{}
	r.stopCompletionAlertLocked()
	r.tracef("task_cancelled id=%d title=%q", task.ID, task.Title)
	r.resumeNoTaskTrackingIfNeededLocked(r.clock.Now())
	return task, nil
}

func (r *Runtime) Status() string {
	r.mu.Lock()
	snapshot := r.state
	now := r.clock.Now()
	noTaskSince := r.noTaskSince
	r.mu.Unlock()

	if snapshot.Phase == domain.PhaseIdle && snapshot.CurrentTask == nil && !noTaskSince.IsZero() && !r.systemLocked {
		remaining := storage.GetRuntimeConfig().IdleLockAfter - r.clock.Since(noTaskSince)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("No task active | Lock in: %s", remaining.Round(time.Second))
	}
	return status.Render(snapshot, now)
}

func (r *Runtime) SetSystemLocked(locked bool) {
	r.mu.Lock()
	r.systemLocked = locked
	r.mu.Unlock()
}

func (r *Runtime) OnIdleEntered() {
	r.mu.Lock()
	r.idleActive = true
	r.mu.Unlock()
}

func (r *Runtime) OnIdleExited() {
	r.mu.Lock()
	r.idleActive = false
	r.stopCompletionAlertLocked()
	r.resumeNoTaskTrackingIfNeededLocked(r.clock.Now())
	r.mu.Unlock()
}

func (r *Runtime) OnScreenLocked() {
	r.mu.Lock()
	r.cancelDeadlineLocked("relock")
	r.relockUntil = time.Time{}
	r.stopNoTaskTimersLocked()
	r.noTaskSince = time.Time{}
	r.noTaskWarned = false
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenLocked, At: r.clock.Now()}).State
	r.mu.Unlock()
}

func (r *Runtime) OnScreenUnlocked() {
	r.mu.Lock()
	r.stopCompletionAlertLocked()
	now := r.clock.Now()
	snap := r.state
	cfg := storage.GetRuntimeConfig()

	if r.current == nil {
		if snap.Phase == domain.PhasePendingCooldown {
			r.mu.Unlock()
			return
		}
		if snap.Phase == domain.PhaseCooldown {
			relockDelay := cfg.RelockDelay
			r.cancelDeadlineLocked("relock")
			r.relockUntil = now.Add(relockDelay)
			r.scheduleDeadlineLocked("relock", now.Add(relockDelay), func() {
				r.lockScreen()
			})
			r.mu.Unlock()
			r.notify("Cooldown Active", fmt.Sprintf("Cooldown active. Locking again in %s.", relockDelay.Round(time.Second)))
			return
		}
		r.resumeNoTaskTrackingIfNeededLocked(now)
		r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenUnlock, At: now}).State
		r.mu.Unlock()
		return
	}

	if snap.Phase != domain.PhaseBreak {
		r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenUnlock, At: now}).State
		r.mu.Unlock()
		return
	}

	notifyUser := !r.hasDeadlineLocked("relock")
	r.cancelDeadlineLocked("relock")
	relockDelay := cfg.RelockDelay
	r.relockUntil = now.Add(relockDelay)
	r.scheduleDeadlineLocked("relock", now.Add(relockDelay), func() {
		r.relockIfBreak()
	})
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenUnlock, At: now}).State
	r.mu.Unlock()

	if notifyUser {
		r.notify("Break Active", fmt.Sprintf("Break is active. Locking again in %s.", relockDelay.Round(time.Second)))
	}
}

func (r *Runtime) armTaskTimersLocked(task *domain.Task) {
	cfg := storage.GetRuntimeConfig()
	expireAt := task.StartTime.Add(task.Duration)
	remaining := r.clock.Until(expireAt)
	if remaining < 0 {
		remaining = 0
	}
	r.scheduleDeadlineLocked("task_expire", expireAt, func() {
		r.completeCurrentTask(task.ID)
	})
	r.tracef("timer_set name=task_expire in=%s task_id=%d", remaining, task.ID)

	if plan, ok := breakPlanForDuration(task.Duration, cfg); ok {
		warnAt := plan.startOffset - cfg.BreakWarning
		if warnAt > 0 {
			r.scheduleDeadlineLocked("break_warn", task.StartTime.Add(warnAt), func() {
				r.notifyBreakComing(task.ID)
			})
			r.tracef("timer_set name=break_warn in=%s task_id=%d", warnAt, task.ID)
		}
		r.scheduleDeadlineLocked("break_start", task.StartTime.Add(plan.startOffset), func() {
			r.startBreak(task.ID, plan.duration)
		})
		r.tracef("timer_set name=break_start in=%s task_id=%d break_duration=%s", plan.startOffset, task.ID, plan.duration)
	}
}

func (r *Runtime) completeCurrentTask(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID {
		r.mu.Unlock()
		return
	}
	task := r.current
	r.current = nil
	r.stopTaskTimersLocked()
	r.relockUntil = time.Time{}

	now := r.clock.Now()
	cfg := storage.GetRuntimeConfig()
	cooldownDuration := cooldownDurationFor(task.Duration, cfg)
	cooldownStartAt := now.Add(cfg.CooldownStartDelay)
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTaskCompleted, At: now, CooldownStartAt: cooldownStartAt, CooldownDuration: cooldownDuration}).State
	r.scheduleDeadlineLocked("cooldown_start", cooldownStartAt, func() {
		r.lockScreen()
	})
	r.scheduleDeadlineLocked("cooldown_end", cooldownStartAt.Add(cooldownDuration), func() {
		r.startCompletionAlert()
		r.mu.Lock()
		r.resumeNoTaskTrackingAfterCooldownLocked(r.clock.Now())
		r.mu.Unlock()
	})
	r.mu.Unlock()

	if err := storage.AppendCompletedTask(*task); err != nil {
		fmt.Printf("failed to persist completed task: %v\n", err)
	}
	r.notify("Task Complete", fmt.Sprintf("'%s' has finished. Cooldown starts in %s", task.Title, cfg.CooldownStartDelay.Round(time.Second)))
}

func (r *Runtime) notifyBreakComing(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID {
		r.mu.Unlock()
		return
	}
	title := r.current.Title
	warning := storage.GetRuntimeConfig().BreakWarning
	r.mu.Unlock()
	r.notify("Break Reminder", fmt.Sprintf("Break starts in %s for '%s'", warning, title))
}

func (r *Runtime) startBreak(taskID int, breakDuration time.Duration) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID || r.state.Phase != domain.PhaseActive {
		r.mu.Unlock()
		return
	}
	now := r.clock.Now()
	breakUntil := now.Add(breakDuration)
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventBreakStarted, At: now, BreakUntil: breakUntil}).State
	r.scheduleDeadlineLocked("break_end", breakUntil, func() {
		r.endBreak(taskID)
	})
	r.mu.Unlock()
	r.notify("Break Started", fmt.Sprintf("Break for %s has started", breakDuration.Round(time.Second)))
	r.lockScreen()
}

func (r *Runtime) endBreak(taskID int) {
	r.mu.Lock()
	if r.current == nil || r.current.ID != taskID || r.state.Phase != domain.PhaseBreak {
		r.mu.Unlock()
		return
	}
	r.cancelDeadlineLocked("break_end")
	r.cancelDeadlineLocked("relock")
	r.relockUntil = time.Time{}
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventBreakEnded, At: r.clock.Now()}).State
	r.mu.Unlock()
	r.unlockScreen()
	r.notify("Break Complete", "Break period ended. Continue your task.")
	r.startCompletionAlert()
}

func (r *Runtime) relockIfBreak() {
	r.mu.Lock()
	if r.state.Phase != domain.PhaseBreak {
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

func (r *Runtime) startCompletionAlert() {
	r.mu.Lock()
	if !r.idleActive {
		r.mu.Unlock()
		r.playSound("assets/task-ending.mp3")
		return
	}
	if r.hasDeadlineLocked("completion_alert") {
		r.mu.Unlock()
		return
	}
	repeatInterval := storage.GetRuntimeConfig().CompletionAlertRepeatInterval
	r.completionAlertToken++
	token := r.completionAlertToken
	r.mu.Unlock()

	r.playSound("assets/task-ending.mp3")

	r.mu.Lock()
	if token != r.completionAlertToken || !r.idleActive {
		r.mu.Unlock()
		return
	}
	r.scheduleCompletionAlertTickLocked(token, repeatInterval)
	r.mu.Unlock()
}

func (r *Runtime) scheduleCompletionAlertTickLocked(token uint64, repeatInterval time.Duration) {
	r.cancelDeadlineLocked("completion_alert")
	nextAt := r.clock.Now().Add(repeatInterval)
	r.scheduleDeadlineLocked("completion_alert", nextAt, func() {
		r.runCompletionAlertTick(token)
	})
}

func (r *Runtime) runCompletionAlertTick(token uint64) {
	r.mu.Lock()
	if token != r.completionAlertToken || !r.idleActive {
		if token == r.completionAlertToken {
			r.cancelDeadlineLocked("completion_alert")
		}
		r.mu.Unlock()
		return
	}
	repeatInterval := storage.GetRuntimeConfig().CompletionAlertRepeatInterval
	r.cancelDeadlineLocked("completion_alert")
	r.mu.Unlock()

	r.playSound("assets/task-ending.mp3")

	r.mu.Lock()
	if token != r.completionAlertToken || !r.idleActive {
		r.mu.Unlock()
		return
	}
	r.scheduleCompletionAlertTickLocked(token, repeatInterval)
	r.mu.Unlock()
}

func (r *Runtime) armNoTaskTimersLocked(now time.Time) {
	cfg := storage.GetRuntimeConfig()
	r.stopNoTaskTimersLocked()
	r.noTaskSince = now
	r.noTaskWarned = false
	noTaskSince := r.noTaskSince
	if cfg.IdleWarnAfter > 0 {
		r.scheduleDeadlineLocked("no_task_warn", noTaskSince.Add(cfg.IdleWarnAfter), func() {
			r.notifyNoTaskStillActive(noTaskSince)
		})
	}
	if cfg.IdleLockAfter > 0 {
		r.scheduleDeadlineLocked("no_task_lock", noTaskSince.Add(cfg.IdleLockAfter), func() {
			r.lockNoTaskStillActive(noTaskSince)
		})
	}
}

func (r *Runtime) notifyNoTaskStillActive(noTaskSince time.Time) {
	r.mu.Lock()
	if !r.noTaskSince.Equal(noTaskSince) || r.current != nil || r.systemLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
		r.mu.Unlock()
		return
	}
	if r.noTaskWarned {
		r.mu.Unlock()
		return
	}
	r.noTaskWarned = true
	cfg := storage.GetRuntimeConfig()
	r.mu.Unlock()
	remaining := (cfg.IdleLockAfter - cfg.IdleWarnAfter).Round(time.Second)
	r.notify("No Task Active", "No task active. Locking in "+remaining.String()+".")
}

func (r *Runtime) lockNoTaskStillActive(noTaskSince time.Time) {
	r.mu.Lock()
	if !r.noTaskSince.Equal(noTaskSince) || r.current != nil || r.systemLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
		r.mu.Unlock()
		return
	}
	r.stopNoTaskTimersLocked()
	r.noTaskSince = time.Time{}
	r.noTaskWarned = false
	r.mu.Unlock()
	r.lockScreen()
}

func (r *Runtime) resumeNoTaskTrackingIfNeededLocked(now time.Time) {
	if r.current != nil || r.systemLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
		return
	}
	if r.noTaskSince.IsZero() || !r.hasDeadlineLocked("no_task_warn") || !r.hasDeadlineLocked("no_task_lock") {
		r.armNoTaskTimersLocked(now)
	}
}

func (r *Runtime) resumeNoTaskTrackingAfterCooldownLocked(now time.Time) {
	if r.current != nil || r.systemLocked {
		return
	}
	if r.noTaskSince.IsZero() || !r.hasDeadlineLocked("no_task_warn") || !r.hasDeadlineLocked("no_task_lock") {
		r.armNoTaskTimersLocked(now)
	}
}

func (r *Runtime) stopNoTaskTimersLocked() {
	r.cancelDeadlineLocked("no_task_warn")
	r.cancelDeadlineLocked("no_task_lock")
}

func (r *Runtime) stopTaskTimersLocked() {
	r.cancelDeadlineLocked("task_expire")
	r.cancelDeadlineLocked("break_warn")
	r.cancelDeadlineLocked("break_start")
	r.cancelDeadlineLocked("break_end")
	r.cancelDeadlineLocked("relock")
	r.cancelDeadlineLocked("cooldown_start")
	r.cancelDeadlineLocked("cooldown_end")
}

func (r *Runtime) stopAllTimersLocked() {
	r.stopTaskTimersLocked()
	r.stopNoTaskTimersLocked()
}

func (r *Runtime) stopCompletionAlertLocked() {
	r.cancelDeadlineLocked("completion_alert")
	r.completionAlertToken++
}

func (r *Runtime) scheduleDeadlineLocked(name string, at time.Time, fn func()) {
	if r.deadlines == nil {
		r.deadlines = make(map[string]*scheduler.CallbackHandle)
	}
	r.cancelDeadlineLocked(name)
	handle := r.timers.Schedule(at, fn)
	if handle == nil {
		return
	}
	r.deadlines[name] = handle
}

func (r *Runtime) cancelDeadlineLocked(name string) {
	if r.deadlines == nil {
		return
	}
	handle, ok := r.deadlines[name]
	if !ok || handle == nil {
		return
	}
	handle.Cancel()
	delete(r.deadlines, name)
}

func (r *Runtime) hasDeadlineLocked(name string) bool {
	if r.deadlines == nil {
		return false
	}
	_, ok := r.deadlines[name]
	return ok
}

func (r *Runtime) lockScreen() {
	r.tracef("action lock_screen")
	r.actions.LockScreen()
}

func (r *Runtime) unlockScreen() {
	r.tracef("action unlock_screen")
	r.actions.UnlockScreen()
}

func (r *Runtime) playSound(path string) {
	r.tracef("action play_sound path=%s", path)
	r.actions.PlaySound(path)
}

func (r *Runtime) notify(title, message string) {
	r.tracef("action notify title=%q", title)
	r.actions.Notify(title, message)
}

func (r *Runtime) tracef(format string, args ...any) {
	if !r.trace {
		return
	}
	log.Printf("flow "+format, args...)
}

func (r *Runtime) startDecisionLocked() error {
	switch r.state.Phase {
	case domain.PhaseIdle:
		return nil
	case domain.PhasePendingCooldown, domain.PhaseCooldown:
		return fmt.Errorf("cooldown active, wait before creating a new task")
	case domain.PhaseBreak:
		return fmt.Errorf("break active, wait before creating a new task")
	default:
		return fmt.Errorf("a task is already active")
	}
}

type breakPlan struct {
	startOffset time.Duration
	duration    time.Duration
}

func breakPlanForDuration(duration time.Duration, cfg storage.RuntimeConfig) (breakPlan, bool) {
	switch {
	case duration >= cfg.TaskDeep:
		return breakPlan{startOffset: cfg.BreakDeepStart, duration: cfg.BreakDeepDuration}, true
	case duration >= cfg.TaskLong:
		return breakPlan{startOffset: cfg.BreakLongStart, duration: cfg.BreakLongDuration}, true
	default:
		return breakPlan{}, false
	}
}

func cooldownDurationFor(duration time.Duration, cfg storage.RuntimeConfig) time.Duration {
	switch {
	case duration >= cfg.TaskDeep:
		return cfg.CooldownDeep
	case duration >= cfg.TaskLong:
		return cfg.CooldownLong
	default:
		return cfg.CooldownShort
	}
}

func (r *Runtime) pruneHistoryToTodayLocked(now time.Time) {
	if len(r.history) == 0 {
		return
	}
	todayStart := startOfToday(now)
	todayEnd := todayStart.Add(24 * time.Hour)
	filtered := make([]domain.Task, 0, len(r.history))
	for _, task := range r.history {
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
