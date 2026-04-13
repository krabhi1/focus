package app

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
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

	noTaskSince              time.Time
	noTaskWarned             bool
	relockUntil              time.Time
	taskPaused               bool
	taskPauseTaskID          int
	taskPauseExpireLeft      time.Duration
	taskPauseBreakWarn       time.Duration
	taskPauseBreakStart      time.Duration
	taskPauseHasBreakWarn    bool
	taskPauseHasBreakStart   bool
	completionAlertActive    bool
	completionAlertToken     uint64
	completionAlertRemaining int

	deadlines  map[string]*scheduler.CallbackHandle
	deadlineAt map[string]time.Time
}

const completionAlertRepeatDelay = 3 * time.Second

func NewRuntime(actions effects.Actions) *Runtime {
	if actions == nil {
		actions = effects.RealActions{}
	}
	rt := &Runtime{
		actions:    actions,
		clock:      realRuntimeClock{},
		timers:     scheduler.NewCallbackLoop(schedulerClockAdapter{clock: realRuntimeClock{}}),
		trace:      os.Getenv("FOCUS_TRACE_FLOW") == "1",
		state:      domain.InitialState(),
		nextID:     1,
		deadlines:  map[string]*scheduler.CallbackHandle{},
		deadlineAt: map[string]time.Time{},
	}
	rt.mu.Lock()
	rt.resumeNoTaskTrackingIfNeededLocked(rt.clock.Now())
	rt.mu.Unlock()
	return rt
}

func (r *Runtime) SetTraceForTest(enabled bool) {
	r.trace = enabled
}

func (r *Runtime) SetClockForTest(clock runtimeClock) {
	if clock == nil {
		return
	}
	r.mu.Lock()
	oldTimers := r.timers
	r.clock = clock
	r.timers = scheduler.NewCallbackLoop(schedulerClockAdapter{clock: clock})
	r.mu.Unlock()
	if oldTimers != nil {
		oldTimers.Stop()
	}
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

func (r *Runtime) StartTask(title string, duration time.Duration, noBreak bool) (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.traceStateSnapshotLocked()

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
	r.clearPausedTaskLocked()

	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTaskStarted, At: now, Task: task}).State
	r.stopTaskTimersLocked()
	r.stopNoTaskTimersLocked()
	r.stopCompletionAlertLocked()
	r.armTaskTimersLocked(task, noBreak)
	r.traceStateChangeLocked("task_started", before)
	r.tracef("task_started id=%d title=%q duration=%s", task.ID, task.Title, task.Duration)
	return task, nil
}

func (r *Runtime) CancelCurrentTask() (*domain.Task, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.traceStateSnapshotLocked()

	if r.current == nil {
		return nil, fmt.Errorf("no active task to cancel")
	}
	if r.clock.Since(r.current.StartTime) > storage.TaskLockedWaitDuration {
		return nil, fmt.Errorf("task is locked (grace period of %v expired)", storage.TaskLockedWaitDuration)
	}

	task := r.current
	r.current = nil
	r.clearPausedTaskLocked()
	r.removeTaskFromHistoryLocked(task.ID)
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTaskCancelled, At: r.clock.Now()}).State
	r.stopTaskTimersLocked()
	r.stopNoTaskTimersLocked()
	r.relockUntil = time.Time{}
	r.stopCompletionAlertLocked()
	r.traceStateChangeLocked("task_cancelled", before)
	r.tracef("task_cancelled id=%d title=%q", task.ID, task.Title)
	r.resumeNoTaskTrackingIfNeededLocked(r.clock.Now())
	return task, nil
}

func (r *Runtime) Status() string {
	r.mu.Lock()
	now := r.clock.Now()
	snapshot := r.state
	noTaskSince := r.noTaskSince
	r.mu.Unlock()

	if snapshot.Phase == domain.PhaseIdle && snapshot.CurrentTask == nil && !noTaskSince.IsZero() && !snapshot.ScreenLocked {
		remaining := storage.GetRuntimeConfig().IdleLockAfter - r.clock.Since(noTaskSince)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("No task active | Lock in: %s", remaining.Round(time.Second))
	}
	return status.Render(snapshot, now)
}

type traceStateSnapshot struct {
	phase                    domain.Phase
	currentTask              string
	screenLocked             bool
	breakUntil               time.Time
	cooldownStartUntil       time.Time
	cooldownUntil            time.Time
	relockUntil              time.Time
	noTaskSince              time.Time
	taskPaused               bool
	completionAlertActive    bool
	completionAlertRemaining int
}

func (r *Runtime) traceStateSnapshotLocked() traceStateSnapshot {
	return traceStateSnapshot{
		phase:                    r.state.Phase,
		currentTask:              taskTraceLabel(r.current),
		screenLocked:             r.state.ScreenLocked,
		breakUntil:               r.state.BreakUntil,
		cooldownStartUntil:       r.state.CooldownStartUntil,
		cooldownUntil:            r.state.CooldownUntil,
		relockUntil:              r.relockUntil,
		noTaskSince:              r.noTaskSince,
		taskPaused:               r.taskPaused,
		completionAlertActive:    r.completionAlertActive,
		completionAlertRemaining: r.completionAlertRemaining,
	}
}

func (s traceStateSnapshot) String() string {
	return fmt.Sprintf(
		"phase=%s current_task=%s screen_locked=%t break_until=%s cooldown_start_until=%s cooldown_until=%s relock_until=%s no_task_since=%s task_paused=%t completion_alert_active=%t completion_alert_remaining=%d",
		s.phase,
		s.currentTask,
		s.screenLocked,
		formatTraceTime(s.breakUntil),
		formatTraceTime(s.cooldownStartUntil),
		formatTraceTime(s.cooldownUntil),
		formatTraceTime(s.relockUntil),
		formatTraceTime(s.noTaskSince),
		s.taskPaused,
		s.completionAlertActive,
		s.completionAlertRemaining,
	)
}

func (r *Runtime) traceStateChangeLocked(event string, before traceStateSnapshot) {
	if !r.trace {
		return
	}
	after := r.traceStateSnapshotLocked()
	if before == after {
		return
	}
	r.tracef("state change event=%s before={%s} after={%s}", event, before.String(), after.String())
}

func formatTraceTime(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.Format(time.RFC3339Nano)
}

func taskTraceLabel(task *domain.Task) string {
	if task == nil {
		return "none"
	}
	return fmt.Sprintf("[%d] %s", task.ID, task.Title)
}

func taskEndActionForDuration(duration time.Duration) string {
	cfg := storage.GetRuntimeConfig()
	if duration >= cfg.TaskDeep {
		return cfg.TaskDeepEndAction
	}
	if duration >= cfg.TaskLong {
		return cfg.TaskLongEndAction
	}
	return storage.TaskEndActionLock
}

func (r *Runtime) ensureNoTaskTrackingLocked(now time.Time) {
	cfg := storage.GetRuntimeConfig()
	if cfg.IdleLockAfter <= 0 {
		return
	}
	if r.current != nil || r.state.ScreenLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
		return
	}
	if r.noTaskSince.IsZero() || !r.hasDeadlineLocked("no_task_warn") || !r.hasDeadlineLocked("no_task_lock") {
		r.armNoTaskTimersLocked(now)
	}
}

func (r *Runtime) DebugString() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.clock.Now()
	cfg := storage.GetRuntimeConfig()
	var b strings.Builder

	fmt.Fprintf(&b, "now: %s\n", now.Format(time.RFC3339Nano))
	fmt.Fprintf(&b, "phase: %s\n", r.state.Phase)
	fmt.Fprintf(&b, "screen_locked: %t\n", r.state.ScreenLocked)
	fmt.Fprintf(&b, "no_task_since: %s\n", formatDebugTime(r.noTaskSince))
	fmt.Fprintf(&b, "no_task_warned: %t\n", r.noTaskWarned)
	fmt.Fprintf(&b, "relock_until: %s\n", formatDebugTime(r.relockUntil))
	fmt.Fprintf(&b, "task_paused: %t\n", r.taskPaused)
	fmt.Fprintf(&b, "task_pause_task_id: %d\n", r.taskPauseTaskID)
	fmt.Fprintf(&b, "task_pause_expire_left: %s\n", r.taskPauseExpireLeft)
	fmt.Fprintf(&b, "task_pause_break_warn: %s\n", r.taskPauseBreakWarn)
	fmt.Fprintf(&b, "task_pause_break_start: %s\n", r.taskPauseBreakStart)
	fmt.Fprintf(&b, "task_pause_has_break_warn: %t\n", r.taskPauseHasBreakWarn)
	fmt.Fprintf(&b, "task_pause_has_break_start: %t\n", r.taskPauseHasBreakStart)
	fmt.Fprintf(&b, "completion_alert_active: %t\n", r.completionAlertActive)
	fmt.Fprintf(&b, "completion_alert_remaining: %d\n", r.completionAlertRemaining)

	if r.current == nil {
		b.WriteString("current_task: none\n")
	} else {
		fmt.Fprintf(&b, "current_task: [%d] %s | %s | started %s\n", r.current.ID, r.current.Title, r.current.Duration.Round(time.Second), r.current.StartTime.Format(time.RFC3339Nano))
		fmt.Fprintf(&b, "current_task_remaining: %s\n", remainingUntil(r.current.StartTime.Add(r.current.Duration), now))
	}

	keys := sortedDeadlineKeys(r.deadlineAt)
	if len(keys) == 0 {
		b.WriteString("deadlines: none\n")
	} else {
		b.WriteString("deadlines:\n")
		for _, key := range keys {
			fmt.Fprintf(&b, "  - %s at %s\n", key, r.deadlineAt[key].Format(time.RFC3339Nano))
		}
	}

	fmt.Fprintf(&b, "config.task.short: %s\n", cfg.TaskShort)
	fmt.Fprintf(&b, "config.task.medium: %s\n", cfg.TaskMedium)
	fmt.Fprintf(&b, "config.task.long: %s\n", cfg.TaskLong)
	fmt.Fprintf(&b, "config.task.deep: %s\n", cfg.TaskDeep)
	fmt.Fprintf(&b, "config.cooldown.short: %s\n", cfg.CooldownShort)
	fmt.Fprintf(&b, "config.cooldown.long: %s\n", cfg.CooldownLong)
	fmt.Fprintf(&b, "config.cooldown.deep: %s\n", cfg.CooldownDeep)
	fmt.Fprintf(&b, "config.break.long_start: %s\n", cfg.BreakLongStart)
	fmt.Fprintf(&b, "config.break.deep_start: %s\n", cfg.BreakDeepStart)
	fmt.Fprintf(&b, "config.break.warning: %s\n", cfg.BreakWarning)
	fmt.Fprintf(&b, "config.break.long_duration: %s\n", cfg.BreakLongDuration)
	fmt.Fprintf(&b, "config.break.deep_duration: %s\n", cfg.BreakDeepDuration)
	fmt.Fprintf(&b, "config.relock_delay: %s\n", cfg.RelockDelay)
	fmt.Fprintf(&b, "config.cooldown_start_delay: %s\n", cfg.CooldownStartDelay)
	fmt.Fprintf(&b, "config.idle.warn_after: %s\n", cfg.IdleWarnAfter)
	fmt.Fprintf(&b, "config.idle.lock_after: %s\n", cfg.IdleLockAfter)
	fmt.Fprintf(&b, "config.alert.repeat_count: %d\n", cfg.CompletionAlertRepeatCount)
	return b.String()
}

func (r *Runtime) OnSleepPrepared() {
	r.mu.Lock()
	r.pauseCurrentTaskTimersLocked()
	r.mu.Unlock()
}

func (r *Runtime) OnSleepResumed() {
	r.mu.Lock()
	r.resumePausedTaskTimersLocked()
	r.mu.Unlock()
}

func (r *Runtime) OnScreenLocked() {
	r.mu.Lock()
	before := r.traceStateSnapshotLocked()
	r.cancelDeadlineLocked("relock")
	r.relockUntil = time.Time{}
	r.stopNoTaskTimersLocked()
	r.noTaskSince = time.Time{}
	r.noTaskWarned = false
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenLocked, At: r.clock.Now()}).State
	r.resumeCompletionAlertIfNeededLocked()
	r.traceStateChangeLocked("screen_locked", before)
	r.mu.Unlock()
}

func (r *Runtime) OnScreenUnlocked() {
	r.mu.Lock()
	before := r.traceStateSnapshotLocked()
	r.stopCompletionAlertLocked()
	now := r.clock.Now()
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventScreenUnlock, At: now}).State
	snap := r.state
	cfg := storage.GetRuntimeConfig()

	if r.current == nil {
		if snap.Phase == domain.PhasePendingCooldown {
			r.traceStateChangeLocked("screen_unlocked", before)
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
			r.traceStateChangeLocked("screen_unlocked", before)
			r.mu.Unlock()
			r.notify("Cooldown Active", fmt.Sprintf("Cooldown active. Locking again in %s.", relockDelay.Round(time.Second)))
			return
		}
		r.resumeNoTaskTrackingIfNeededLocked(now)
		r.traceStateChangeLocked("screen_unlocked", before)
		r.mu.Unlock()
		return
	}

	if snap.Phase != domain.PhaseBreak {
		r.traceStateChangeLocked("screen_unlocked", before)
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
	r.traceStateChangeLocked("screen_unlocked", before)
	r.mu.Unlock()

	if notifyUser {
		r.notify("Break Active", fmt.Sprintf("Break is active. Locking again in %s.", relockDelay.Round(time.Second)))
	}
}

func (r *Runtime) pauseCurrentTaskTimersLocked() {
	before := r.traceStateSnapshotLocked()
	if r.current == nil || r.taskPaused || r.state.Phase != domain.PhaseActive {
		return
	}
	now := r.clock.Now()
	task := r.current
	cfg := storage.GetRuntimeConfig()

	r.taskPaused = true
	r.taskPauseTaskID = task.ID
	r.taskPauseExpireLeft = remainingUntil(task.StartTime.Add(task.Duration), now)
	r.taskPauseHasBreakWarn = false
	r.taskPauseHasBreakStart = false
	r.taskPauseBreakWarn = 0
	r.taskPauseBreakStart = 0

	if plan, ok := breakPlanForDuration(task.Duration, cfg); ok {
		if r.hasDeadlineLocked("break_warn") {
			r.taskPauseHasBreakWarn = true
			r.taskPauseBreakWarn = remainingUntil(task.StartTime.Add(plan.startOffset-cfg.BreakWarning), now)
		}
		if r.hasDeadlineLocked("break_start") {
			r.taskPauseHasBreakStart = true
			r.taskPauseBreakStart = remainingUntil(task.StartTime.Add(plan.startOffset), now)
		}
	}

	r.cancelDeadlineLocked("task_expire")
	r.cancelDeadlineLocked("break_warn")
	r.cancelDeadlineLocked("break_start")
	r.tracef("task_paused id=%d title=%q", task.ID, task.Title)
	r.traceStateChangeLocked("task_paused", before)
}

func (r *Runtime) resumePausedTaskTimersLocked() {
	before := r.traceStateSnapshotLocked()
	if r.current == nil || !r.taskPaused || r.state.Phase != domain.PhaseActive || r.current.ID != r.taskPauseTaskID {
		return
	}
	now := r.clock.Now()
	task := r.current

	if r.taskPauseExpireLeft > 0 {
		r.scheduleDeadlineLocked("task_expire", now.Add(r.taskPauseExpireLeft), func() {
			r.completeCurrentTask(task.ID)
		})
	}
	if r.taskPauseHasBreakWarn && r.taskPauseBreakWarn > 0 {
		r.scheduleDeadlineLocked("break_warn", now.Add(r.taskPauseBreakWarn), func() {
			r.notifyBreakComing(task.ID)
		})
	}
	if r.taskPauseHasBreakStart && r.taskPauseBreakStart > 0 {
		cfg := storage.GetRuntimeConfig()
		breakDuration := breakDurationForTask(task.Duration, cfg)
		r.scheduleDeadlineLocked("break_start", now.Add(r.taskPauseBreakStart), func() {
			r.startBreak(task.ID, breakDuration)
		})
	}

	r.clearPausedTaskLocked()
	r.traceStateChangeLocked("task_resumed", before)
	r.tracef("task_resumed id=%d title=%q", task.ID, task.Title)
}

func (r *Runtime) armTaskTimersLocked(task *domain.Task, noBreak bool) {
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

	if noBreak {
		return
	}

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
	before := r.traceStateSnapshotLocked()
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
	endAction := taskEndActionForDuration(task.Duration)
	r.scheduleDeadlineLocked("cooldown_start", cooldownStartAt, func() {
		r.beginCooldown(endAction)
	})
	r.traceStateChangeLocked("task_completed", before)
	r.mu.Unlock()

	if err := storage.AppendCompletedTask(*task); err != nil {
		fmt.Printf("failed to persist completed task: %v\n", err)
	}
	r.notify("Task Complete", fmt.Sprintf("'%s' has finished. Cooldown starts in %s", task.Title, cfg.CooldownStartDelay.Round(time.Second)))
}

func (r *Runtime) beginCooldown(endAction string) {
	r.mu.Lock()
	before := r.traceStateSnapshotLocked()
	if r.state.Phase != domain.PhasePendingCooldown {
		r.mu.Unlock()
		return
	}
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTick, At: r.clock.Now()}).State
	cooldownEndAt := r.state.CooldownUntil
	r.scheduleDeadlineLocked("cooldown_end", cooldownEndAt, func() {
		r.finishCooldown()
	})
	r.traceStateChangeLocked("cooldown_start", before)
	r.mu.Unlock()
	if endAction == storage.TaskEndActionSleep {
		r.sleep()
		return
	}
	r.lockScreen()
}

func (r *Runtime) finishCooldown() {
	r.mu.Lock()
	before := r.traceStateSnapshotLocked()
	if r.state.Phase != domain.PhaseCooldown {
		r.mu.Unlock()
		return
	}
	now := r.clock.Now()
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventTick, At: now}).State
	r.cancelDeadlineLocked("relock")
	r.relockUntil = time.Time{}
	r.ensureNoTaskTrackingLocked(now)
	r.traceStateChangeLocked("cooldown_complete", before)
	r.mu.Unlock()

	r.startCompletionAlert()
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
	before := r.traceStateSnapshotLocked()
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
	r.traceStateChangeLocked("break_start", before)
	r.mu.Unlock()
	r.notify("Break Started", fmt.Sprintf("Break for %s has started", breakDuration.Round(time.Second)))
	r.lockScreen()
}

func (r *Runtime) endBreak(taskID int) {
	r.mu.Lock()
	before := r.traceStateSnapshotLocked()
	if r.current == nil || r.current.ID != taskID || r.state.Phase != domain.PhaseBreak {
		r.mu.Unlock()
		return
	}
	r.cancelDeadlineLocked("break_end")
	r.cancelDeadlineLocked("relock")
	r.relockUntil = time.Time{}
	r.state = domain.Reduce(r.state, domain.Event{Type: domain.EventBreakEnded, At: r.clock.Now()}).State
	r.traceStateChangeLocked("break_end", before)
	r.mu.Unlock()
	r.playSound("assets/task-ending.mp3")
	r.unlockScreen()
	r.notify("Break Complete", "Break period ended. Continue your task.")
}

func (r *Runtime) relockIfBreak() {
	r.mu.Lock()
	if r.state.Phase != domain.PhaseBreak {
		r.mu.Unlock()
		return
	}
	if r.state.ScreenLocked {
		r.mu.Unlock()
		return
	}
	r.relockUntil = time.Time{}
	r.mu.Unlock()
	r.lockScreen()
}

func (r *Runtime) startCompletionAlert() {
	r.mu.Lock()
	repeatCount := storage.GetRuntimeConfig().CompletionAlertRepeatCount
	if repeatCount <= 0 {
		r.stopCompletionAlertLocked()
		r.mu.Unlock()
		return
	}
	r.completionAlertActive = true
	r.completionAlertToken++
	r.completionAlertRemaining = repeatCount - 1
	token := r.completionAlertToken
	locked := r.state.ScreenLocked
	r.mu.Unlock()

	r.playSound("assets/task-ending.mp3")

	r.mu.Lock()
	if token != r.completionAlertToken || !r.completionAlertActive {
		r.mu.Unlock()
		return
	}
	if r.completionAlertRemaining <= 0 {
		r.completionAlertActive = false
		r.mu.Unlock()
		return
	}
	if locked {
		r.scheduleCompletionAlertTickLocked(token)
	}
	r.mu.Unlock()
}

func (r *Runtime) scheduleCompletionAlertTickLocked(token uint64) {
	r.cancelDeadlineLocked("completion_alert")
	nextAt := r.clock.Now().Add(completionAlertRepeatDelay)
	r.scheduleDeadlineLocked("completion_alert", nextAt, func() {
		r.runCompletionAlertTick(token)
	})
}

func (r *Runtime) runCompletionAlertTick(token uint64) {
	r.mu.Lock()
	if token != r.completionAlertToken || !r.completionAlertActive || !r.state.ScreenLocked || r.completionAlertRemaining <= 0 {
		if token == r.completionAlertToken {
			r.cancelDeadlineLocked("completion_alert")
		}
		r.mu.Unlock()
		return
	}
	r.cancelDeadlineLocked("completion_alert")
	r.completionAlertRemaining--
	shouldRepeat := r.completionAlertRemaining > 0
	r.mu.Unlock()

	r.playSound("assets/task-ending.mp3")

	r.mu.Lock()
	if token != r.completionAlertToken || !r.completionAlertActive {
		r.mu.Unlock()
		return
	}
	if r.state.ScreenLocked && shouldRepeat {
		r.scheduleCompletionAlertTickLocked(token)
	} else if !shouldRepeat {
		r.completionAlertActive = false
	}
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
	if !r.noTaskSince.Equal(noTaskSince) || r.current != nil || r.state.ScreenLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
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
	if !r.noTaskSince.Equal(noTaskSince) || r.current != nil || r.state.ScreenLocked || r.state.Phase == domain.PhaseCooldown || r.state.Phase == domain.PhasePendingCooldown || r.state.Phase == domain.PhaseBreak {
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
	before := r.traceStateSnapshotLocked()
	r.ensureNoTaskTrackingLocked(now)
	r.traceStateChangeLocked("resume_no_task", before)
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
	r.completionAlertActive = false
	r.completionAlertToken++
}

func (r *Runtime) resumeCompletionAlertIfNeededLocked() {
	if !r.completionAlertActive || !r.state.ScreenLocked || r.hasDeadlineLocked("completion_alert") || r.completionAlertRemaining <= 0 {
		return
	}
	r.scheduleCompletionAlertTickLocked(r.completionAlertToken)
}

func (r *Runtime) scheduleDeadlineLocked(name string, at time.Time, fn func()) {
	if r.deadlines == nil {
		r.deadlines = make(map[string]*scheduler.CallbackHandle)
	}
	if r.deadlineAt == nil {
		r.deadlineAt = make(map[string]time.Time)
	}
	r.cancelDeadlineLocked(name)
	handle := r.timers.Schedule(at, fn)
	if handle == nil {
		return
	}
	r.deadlines[name] = handle
	r.deadlineAt[name] = at
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
	delete(r.deadlineAt, name)
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

func (r *Runtime) sleep() {
	r.tracef("action sleep")
	r.actions.Sleep()
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

func formatDebugTime(t time.Time) string {
	if t.IsZero() {
		return "none"
	}
	return t.Format(time.RFC3339Nano)
}

func sortedDeadlineKeys(m map[string]time.Time) []string {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
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

func breakDurationForTask(duration time.Duration, cfg storage.RuntimeConfig) time.Duration {
	switch {
	case duration >= cfg.TaskDeep:
		return cfg.BreakDeepDuration
	case duration >= cfg.TaskLong:
		return cfg.BreakLongDuration
	default:
		return 0
	}
}

func remainingUntil(at, now time.Time) time.Duration {
	remaining := at.Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
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

func (r *Runtime) removeTaskFromHistoryLocked(taskID int) {
	if len(r.history) == 0 {
		return
	}
	filtered := r.history[:0]
	for _, task := range r.history {
		if task.ID == taskID {
			continue
		}
		filtered = append(filtered, task)
	}
	r.history = filtered
}

func (r *Runtime) clearPausedTaskLocked() {
	r.taskPaused = false
	r.taskPauseTaskID = 0
	r.taskPauseExpireLeft = 0
	r.taskPauseBreakWarn = 0
	r.taskPauseBreakStart = 0
	r.taskPauseHasBreakWarn = false
	r.taskPauseHasBreakStart = false
}

func startOfToday(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
