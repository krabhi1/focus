package app

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"focus/internal/domain"
	"focus/internal/effects"
	"focus/internal/storage"
)

type fakeRuntimeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeRuntimeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeRuntimeClock) Since(t time.Time) time.Duration {
	return c.Now().Sub(t)
}

func (c *fakeRuntimeClock) Until(t time.Time) time.Duration {
	return t.Sub(c.Now())
}

func (c *fakeRuntimeClock) AfterFunc(d time.Duration, f func()) runtimeTimer {
	return &noopRuntimeTimer{}
}

func (c *fakeRuntimeClock) NewTimer(d time.Duration) runtimeTimer {
	return &noopRuntimeTimer{}
}

func (c *fakeRuntimeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type noopRuntimeTimer struct{}

func (noopRuntimeTimer) Stop() bool          { return false }
func (noopRuntimeTimer) C() <-chan time.Time { return nil }

type soundRecorder struct {
	mu    sync.Mutex
	plays []string
}

func (r *soundRecorder) LockScreen()           {}
func (r *soundRecorder) UnlockScreen()         {}
func (r *soundRecorder) Sleep()                {}
func (r *soundRecorder) Notify(string, string) {}
func (r *soundRecorder) PlaySound(path string) {
	r.mu.Lock()
	r.plays = append(r.plays, path)
	r.mu.Unlock()
}

func (r *soundRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.plays)
}

type lockRecorder struct {
	mu     sync.Mutex
	locks  int
	sleeps int
}

func (r *lockRecorder) LockScreen() {
	r.mu.Lock()
	r.locks++
	r.mu.Unlock()
}

func (r *lockRecorder) UnlockScreen()         {}
func (r *lockRecorder) Sleep()                { r.mu.Lock(); r.sleeps++; r.mu.Unlock() }
func (r *lockRecorder) Notify(string, string) {}
func (r *lockRecorder) PlaySound(string)      {}

func (r *lockRecorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.locks
}

func (r *lockRecorder) SleepCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sleeps
}

func captureRuntimeLogs(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	})
	return &buf
}

func TestRuntimeStartTaskAndCancel(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 50 * time.Millisecond
	cfg.TaskMedium = 100 * time.Millisecond
	cfg.CooldownShort = 60 * time.Millisecond
	cfg.CooldownStartDelay = 20 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskShort, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil || task.Title != "demo" {
		t.Fatalf("task = %#v, want demo", task)
	}

	snapshot := rt.Snapshot()
	if snapshot.Phase != "active" {
		t.Fatalf("phase = %s, want active", snapshot.Phase)
	}

	cancelled, err := rt.CancelCurrentTask()
	if err != nil {
		t.Fatalf("CancelCurrentTask returned error: %v", err)
	}
	if cancelled == nil || cancelled.Title != "demo" {
		t.Fatalf("cancelled = %#v, want demo", cancelled)
	}

	if got := rt.Snapshot().Phase; got != "idle" {
		t.Fatalf("phase after cancel = %s, want idle", got)
	}
	if got := rt.HistoryCount(); got != 0 {
		t.Fatalf("HistoryCount after cancel = %d, want 0", got)
	}
}

func TestRuntimeTraceLogsTaskStart(t *testing.T) {
	buf := captureRuntimeLogs(t)

	rt := NewRuntime(effects.NoopActions{})
	rt.SetTraceForTest(true)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("demo", 30*time.Minute, false); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}

	got := buf.String()
	if !strings.Contains(got, "state change event=task_started") {
		t.Fatalf("log output = %q, want task_started trace", got)
	}
	if !strings.Contains(got, "before={phase=idle") || !strings.Contains(got, "after={phase=active") {
		t.Fatalf("log output = %q, want phase transition snapshots", got)
	}
	if !strings.Contains(got, "current_task=none") || !strings.Contains(got, "current_task=[1] demo") {
		t.Fatalf("log output = %q, want current task transition snapshots", got)
	}
}

func TestRuntimeTraceLogsCooldownCompletion(t *testing.T) {
	buf := captureRuntimeLogs(t)

	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	clock := &fakeRuntimeClock{now: time.Date(2026, 4, 13, 14, 0, 0, 0, time.UTC)}
	rt := NewRuntime(effects.NoopActions{})
	rt.SetTraceForTest(true)
	rt.SetClockForTest(clock)
	t.Cleanup(rt.Close)

	rt.mu.Lock()
	rt.state = domain.State{
		Phase:              domain.PhaseCooldown,
		CooldownUntil:      clock.Now().Add(-time.Second),
		CooldownDuration:   2 * time.Minute,
		CooldownStartUntil: time.Time{},
	}
	rt.current = nil
	rt.noTaskSince = time.Time{}
	rt.mu.Unlock()

	rt.finishCooldown()

	got := buf.String()
	if !strings.Contains(got, "state change event=cooldown_complete") {
		t.Fatalf("log output = %q, want cooldown_complete trace", got)
	}
	if !strings.Contains(got, "before={phase=cooldown") || !strings.Contains(got, "after={phase=idle") {
		t.Fatalf("log output = %q, want cooldown phase transition snapshots", got)
	}
	if !strings.Contains(got, "no_task_since=none") || !strings.Contains(got, "no_task_since=2026-04-13T14:00:00Z") {
		t.Fatalf("log output = %q, want no_task_since repair trace", got)
	}
}

func TestRuntimeCooldownStartLocksForLongTasksByDefault(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 10 * time.Millisecond
	cfg.TaskMedium = 20 * time.Millisecond
	cfg.TaskLong = 30 * time.Millisecond
	cfg.TaskDeep = 40 * time.Millisecond
	cfg.BreakWarning = 1 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakDeepStart = 20 * time.Millisecond
	cfg.BreakLongDuration = 5 * time.Millisecond
	cfg.BreakDeepDuration = 5 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownShort = 10 * time.Millisecond
	cfg.CooldownLong = 10 * time.Millisecond
	cfg.CooldownDeep = 10 * time.Millisecond
	cfg.CooldownStartDelay = 5 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &lockRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("demo", cfg.TaskLong, true); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}

	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		if actions.Count() > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if got := actions.Count(); got != 1 {
		t.Fatalf("lock count = %d, want 1 for long task cooldown start", got)
	}
	if got := actions.SleepCount(); got != 0 {
		t.Fatalf("sleep count = %d, want 0 for long task cooldown start", got)
	}
}

func TestRuntimeCooldownStartSleepsForDeepTasksByDefault(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 10 * time.Millisecond
	cfg.TaskMedium = 20 * time.Millisecond
	cfg.TaskLong = 30 * time.Millisecond
	cfg.TaskDeep = 40 * time.Millisecond
	cfg.BreakWarning = 1 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakDeepStart = 20 * time.Millisecond
	cfg.BreakLongDuration = 5 * time.Millisecond
	cfg.BreakDeepDuration = 5 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownShort = 10 * time.Millisecond
	cfg.CooldownLong = 10 * time.Millisecond
	cfg.CooldownDeep = 10 * time.Millisecond
	cfg.CooldownStartDelay = 5 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &lockRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("demo", cfg.TaskDeep, true); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}

	deadline := time.Now().Add(120 * time.Millisecond)
	for time.Now().Before(deadline) {
		if actions.SleepCount() > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if got := actions.SleepCount(); got != 1 {
		t.Fatalf("sleep count = %d, want 1 for deep task cooldown start", got)
	}
	if got := actions.Count(); got != 0 {
		t.Fatalf("lock count = %d, want 0 for deep task cooldown start", got)
	}
}

func TestRuntimeCooldownCompletionCancelsRelock(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.RelockDelay = 20 * time.Millisecond
	cfg.CooldownStartDelay = 1 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &lockRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.mu.Lock()
	rt.state = domain.State{
		Phase:         domain.PhaseCooldown,
		ScreenLocked:  true,
		CooldownUntil: time.Now().Add(50 * time.Millisecond),
	}
	rt.mu.Unlock()

	rt.OnScreenUnlocked()
	rt.finishCooldown()

	time.Sleep(60 * time.Millisecond)

	if got := actions.Count(); got != 0 {
		t.Fatalf("lock count = %d, want 0 after cooldown completion canceled relock", got)
	}
}

func TestRuntimeStatusDoesNotMutateState(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	clock := &fakeRuntimeClock{now: time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC)}
	rt.SetClockForTest(clock)

	rt.mu.Lock()
	rt.noTaskSince = time.Time{}
	rt.stopNoTaskTimersLocked()
	before := rt.traceStateSnapshotLocked()
	rt.mu.Unlock()

	if got := rt.Status(); got != "Idle" {
		t.Fatalf("status = %q, want Idle", got)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	after := rt.traceStateSnapshotLocked()
	if before != after {
		t.Fatalf("state changed after status read: before=%+v after=%+v", before, after)
	}
}

func TestRuntimeCancelTaskGraceWindow(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	clock := &fakeRuntimeClock{now: time.Date(2026, 3, 28, 10, 0, 0, 0, time.UTC)}
	rt := NewRuntime(effects.NoopActions{})
	rt.SetClockForTest(clock)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("demo", cfg.TaskShort, false); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	clock.Advance(59 * time.Second)
	if _, err := rt.CancelCurrentTask(); err != nil {
		t.Fatalf("CancelCurrentTask before 1m returned error: %v", err)
	}

	rt2 := NewRuntime(effects.NoopActions{})
	rt2.SetClockForTest(&fakeRuntimeClock{now: time.Date(2026, 3, 28, 11, 0, 0, 0, time.UTC)})
	t.Cleanup(rt2.Close)
	clock2 := rt2.clock.(*fakeRuntimeClock)
	if _, err := rt2.StartTask("demo2", cfg.TaskShort, false); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	clock2.Advance(61 * time.Second)
	if _, err := rt2.CancelCurrentTask(); err == nil {
		t.Fatal("expected cancel to fail after 1m grace window")
	}
}

func TestRuntimeStatusReArmsAfterScreenUnlock(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	clock := &fakeRuntimeClock{now: time.Date(2026, 4, 13, 13, 0, 0, 0, time.UTC)}
	rt.SetClockForTest(clock)

	rt.OnScreenLocked()

	rt.mu.Lock()
	if !rt.noTaskSince.IsZero() {
		rt.mu.Unlock()
		t.Fatal("noTaskSince after lock = non-zero, want cleared")
	}
	rt.mu.Unlock()

	rt.OnScreenUnlocked()

	if got := rt.Status(); !strings.HasPrefix(got, "No task active | Lock in:") {
		t.Fatalf("status after unlock = %q, want idle countdown", got)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.noTaskSince.IsZero() {
		t.Fatal("noTaskSince after unlock = zero, want repaired timestamp")
	}
}

func TestRuntimeLoadHistoryAndCount(t *testing.T) {
	t.Setenv("FOCUS_HISTORY_FILE", "")
	home := t.TempDir()
	t.Setenv("HOME", home)

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	if err := rt.LoadHistoryFromDisk(); err != nil {
		t.Fatalf("LoadHistoryFromDisk returned error: %v", err)
	}

	if got := rt.HistoryCount(); got != 0 {
		t.Fatalf("HistoryCount = %d, want 0", got)
	}
}

func TestRuntimeLoadHistorySetsNextIDFromAllHistory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FOCUS_HISTORY_FILE", filepath.Join(dir, "history.jsonl"))

	clock := &fakeRuntimeClock{now: time.Date(2026, 3, 29, 12, 0, 0, 0, time.FixedZone("IST", 5*60*60+30*60))}

	yesterday := clock.now.Add(-24 * time.Hour)
	today := clock.now.Add(-time.Hour)

	if err := storage.AppendCompletedTask(domain.Task{
		ID:        90,
		Title:     "old task",
		Duration:  30 * time.Minute,
		StartTime: yesterday,
	}); err != nil {
		t.Fatalf("append old task: %v", err)
	}
	if err := storage.AppendCompletedTask(domain.Task{
		ID:        1,
		Title:     "today task",
		Duration:  30 * time.Minute,
		StartTime: today,
	}); err != nil {
		t.Fatalf("append today task: %v", err)
	}

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	rt.SetClockForTest(clock)

	if err := rt.LoadHistoryFromDisk(); err != nil {
		t.Fatalf("LoadHistoryFromDisk returned error: %v", err)
	}

	if got := rt.HistoryCount(); got != 1 {
		t.Fatalf("HistoryCount = %d, want 1 today task", got)
	}

	task, err := rt.StartTask("new task", 5*time.Minute, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if got := task.ID; got != 91 {
		t.Fatalf("new task ID = %d, want 91", got)
	}
}

func TestRuntimeDebugStringIncludesStateAndConfig(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatCount = 2
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("demo", cfg.TaskShort, false); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	debug := rt.DebugString()
	if !strings.Contains(debug, "phase: active") {
		t.Fatalf("debug = %q, want active phase", debug)
	}
	if !strings.Contains(debug, "current_task:") {
		t.Fatalf("debug = %q, want current task", debug)
	}
	if !strings.Contains(debug, "deadlines:") {
		t.Fatalf("debug = %q, want deadline section", debug)
	}
	if !strings.Contains(debug, "config.alert.repeat_count: 2") {
		t.Fatalf("debug = %q, want alert repeat count", debug)
	}
}

func TestRuntimeCompletionAlertPlaysOnceWhenCountIsOne(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatCount = 1
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.OnScreenLocked()
	rt.startCompletionAlert()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != 1 {
		t.Fatalf("plays with repeat count 1 = %d, want 1", got)
	}
}

func TestRuntimeCompletionAlertRepeatsWhileLockedAndStopsOnUnlock(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatCount = 2
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.OnScreenLocked()
	rt.startCompletionAlert()

	waitForPlays := func(min int, timeout time.Duration) {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if actions.Count() >= min {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("plays = %d, want at least %d", actions.Count(), min)
	}

	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != 1 {
		t.Fatalf("plays while unlocked = %d, want 1", got)
	}

	rt.OnScreenLocked()
	waitForPlays(2, 4*time.Second)
	beforeUnlock := actions.Count()

	rt.OnScreenUnlocked()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != beforeUnlock {
		t.Fatalf("plays after screen unlock = %d, want %d", got, beforeUnlock)
	}
}

func TestRuntimeCompletionAlertDoesNotStartWhileUnlocked(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatCount = 2
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.startCompletionAlert()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != 0 {
		t.Fatalf("plays while unlocked = %d, want 0", got)
	}
}

func TestRuntimeStartTaskNoBreakSkipsBreakTimers(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 120 * time.Millisecond
	cfg.CooldownShort = 10 * time.Millisecond
	cfg.CooldownLong = 20 * time.Millisecond
	cfg.CooldownDeep = 30 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 20 * time.Millisecond
	cfg.BreakDeepStart = 30 * time.Millisecond
	cfg.BreakLongDuration = 10 * time.Millisecond
	cfg.BreakDeepDuration = 10 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownStartDelay = 10 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskLong, true)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	time.Sleep(35 * time.Millisecond)
	if got := rt.Snapshot().Phase; got != "active" {
		t.Fatalf("phase after break offset = %s, want active", got)
	}
}

func TestRuntimeStartTaskSchedulesBreakTimersWhenEnabled(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 120 * time.Millisecond
	cfg.CooldownShort = 10 * time.Millisecond
	cfg.CooldownLong = 20 * time.Millisecond
	cfg.CooldownDeep = 30 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 20 * time.Millisecond
	cfg.BreakDeepStart = 30 * time.Millisecond
	cfg.BreakLongDuration = 10 * time.Millisecond
	cfg.BreakDeepDuration = 10 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownStartDelay = 10 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskLong, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := rt.Snapshot().Phase; got == "break" {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("phase = %s, want break", rt.Snapshot().Phase)
}

func TestRuntimeBreakEndDoesNotPlaySoundWhenUnlocked(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 120 * time.Millisecond
	cfg.CooldownShort = 100 * time.Millisecond
	cfg.CooldownLong = 120 * time.Millisecond
	cfg.CooldownDeep = 140 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakDeepStart = 15 * time.Millisecond
	cfg.BreakLongDuration = 5 * time.Millisecond
	cfg.BreakDeepDuration = 5 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownStartDelay = 50 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskLong, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	time.Sleep(40 * time.Millisecond)
	if got := rt.Snapshot().Phase; got != domain.PhaseActive {
		t.Fatalf("phase after break end = %s, want active", got)
	}
	if got := actions.Count(); got != 0 {
		t.Fatalf("sound plays after break end = %d, want 0", got)
	}
}

func TestRuntimeBreakEndPlaysSoundWhenLocked(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 120 * time.Millisecond
	cfg.CooldownShort = 100 * time.Millisecond
	cfg.CooldownLong = 120 * time.Millisecond
	cfg.CooldownDeep = 140 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakDeepStart = 15 * time.Millisecond
	cfg.BreakLongDuration = 5 * time.Millisecond
	cfg.BreakDeepDuration = 5 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownStartDelay = 50 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskLong, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	rt.OnScreenLocked()

	time.Sleep(40 * time.Millisecond)
	if got := rt.Snapshot().Phase; got != domain.PhaseActive {
		t.Fatalf("phase after break end = %s, want active", got)
	}
	if got := actions.Count(); got != 1 {
		t.Fatalf("sound plays after break end = %d, want 1", got)
	}
}

func TestRuntimeCooldownTransitionsFromPendingToActiveToIdle(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("FOCUS_HISTORY_FILE", filepath.Join(dataDir, "history.jsonl"))

	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 120 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakDeepStart = 15 * time.Millisecond
	cfg.BreakLongDuration = 5 * time.Millisecond
	cfg.BreakDeepDuration = 5 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownShort = 100 * time.Millisecond
	cfg.CooldownLong = 120 * time.Millisecond
	cfg.CooldownDeep = 140 * time.Millisecond
	cfg.CooldownStartDelay = 50 * time.Millisecond
	cfg.CompletionAlertRepeatCount = 0
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskShort, true)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	waitForPhase := func(want domain.Phase, timeout time.Duration) {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if got := rt.Snapshot().Phase; got == want {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
		t.Fatalf("phase = %s, want %s", rt.Snapshot().Phase, want)
	}

	waitForPhase(domain.PhaseCooldown, 500*time.Millisecond)
	if got := rt.Status(); !strings.HasPrefix(got, "Cooldown active") {
		t.Fatalf("status after cooldown start = %q, want cooldown active", got)
	}

	waitForPhase(domain.PhaseIdle, 1*time.Second)
	if got := rt.Status(); !strings.HasPrefix(got, "No task active | Lock in:") {
		t.Fatalf("status after cooldown = %q, want idle countdown", got)
	}
}

func TestRuntimePausesAndResumesTaskOnSleep(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 120 * time.Millisecond
	cfg.TaskDeep = 160 * time.Millisecond
	cfg.CooldownShort = 10 * time.Millisecond
	cfg.CooldownLong = 20 * time.Millisecond
	cfg.CooldownDeep = 30 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 40 * time.Millisecond
	cfg.BreakDeepStart = 60 * time.Millisecond
	cfg.BreakLongDuration = 20 * time.Millisecond
	cfg.BreakDeepDuration = 20 * time.Millisecond
	cfg.RelockDelay = 1 * time.Millisecond
	cfg.CooldownStartDelay = 10 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("demo", cfg.TaskLong, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	time.Sleep(15 * time.Millisecond)
	rt.OnSleepPrepared()
	time.Sleep(80 * time.Millisecond)
	if got := rt.Snapshot().Phase; got != "active" {
		t.Fatalf("phase while paused = %s, want active", got)
	}

	rt.OnSleepResumed()
	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := rt.Snapshot().Phase; got == "break" {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("phase after resume = %s, want break", rt.Snapshot().Phase)
}
