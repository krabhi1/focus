package app

import (
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

	rt.SetSystemLocked(true)
	rt.OnScreenLocked()
	waitForPlays(2, 4*time.Second)
	beforeUnlock := actions.Count()

	rt.SetSystemLocked(false)
	rt.OnScreenUnlocked()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != beforeUnlock {
		t.Fatalf("plays after screen unlock = %d, want %d", got, beforeUnlock)
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

func TestRuntimeBreakEndDoesNotTriggerCompletionAlert(t *testing.T) {
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
