package main

import (
	"fmt"
	"focus/internal/state"
	"sync"
	"testing"
	"time"
)

type recordedAction struct {
	kind  string
	title string
	body  string
	at    time.Time
}

type recordingActions struct {
	mu      sync.Mutex
	actions []recordedAction
}

func (r *recordingActions) LockScreen() {
	r.record(recordedAction{kind: "lock"})
}

func (r *recordingActions) UnlockScreen() {
	r.record(recordedAction{kind: "unlock"})
}

func (r *recordingActions) PlaySound(path string) {
	r.record(recordedAction{kind: "sound", body: path})
}

func (r *recordingActions) Notify(title, message string) {
	r.record(recordedAction{kind: "notify", title: title, body: message})
}

func (r *recordingActions) record(action recordedAction) {
	r.mu.Lock()
	defer r.mu.Unlock()
	action.at = time.Now()
	r.actions = append(r.actions, action)
}

func (r *recordingActions) snapshot() []recordedAction {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedAction, len(r.actions))
	copy(out, r.actions)
	return out
}

func (r *recordingActions) count(kind string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, a := range r.actions {
		if a.kind == kind {
			count++
		}
	}
	return count
}

func TestRuntimeTaskCompleteActionOrder(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 30 * time.Millisecond
	cfg.TaskMedium = 60 * time.Millisecond
	cfg.CooldownShort = 40 * time.Millisecond
	cfg.CooldownStartDelay = 10 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})

	actions := &recordingActions{}
	rt := NewDaemonRuntime(actions)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("order-task", cfg.TaskShort); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return actions.count("sound") >= 1
	}, "expected completion sound after cooldown end")

	all := actions.snapshot()
	notifyIdx := findNotifyIndex(all, "Task Complete")
	lockIdx := findActionIndex(all, "lock")
	soundIdx := findActionIndex(all, "sound")

	if notifyIdx == -1 {
		t.Fatalf("missing Task Complete notification, actions=%v", summarizeActions(all))
	}
	if lockIdx == -1 {
		t.Fatalf("missing lock action, actions=%v", summarizeActions(all))
	}
	if soundIdx == -1 {
		t.Fatalf("missing sound action, actions=%v", summarizeActions(all))
	}
	if !(notifyIdx < lockIdx && lockIdx < soundIdx) {
		t.Fatalf("unexpected action order notify<lock<sound, actions=%v", summarizeActions(all))
	}
}

func TestRuntimeBreakUnlockRelockAndEndActions(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 120 * time.Millisecond
	cfg.TaskDeep = 200 * time.Millisecond
	cfg.BreakWarning = 5 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakLongDuration = 80 * time.Millisecond
	cfg.BreakDeepStart = 20 * time.Millisecond
	cfg.BreakDeepDuration = 80 * time.Millisecond
	cfg.RelockDelay = 15 * time.Millisecond
	cfg.CooldownLong = 60 * time.Millisecond
	cfg.CooldownStartDelay = 500 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})

	actions := &recordingActions{}
	rt := NewDaemonRuntime(actions)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("break-task", cfg.TaskLong); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return findNotifyIndex(actions.snapshot(), "Break Started") >= 0
	}, "expected Break Started notification")

	rt.SetSystemLocked(false)
	rt.OnScreenUnlocked()

	waitForCondition(t, 2*time.Second, func() bool {
		all := actions.snapshot()
		return findNotifyIndex(all, "Break Active") >= 0 && countActionKind(all, "lock") >= 2
	}, "expected break relock after unlock")

	waitForCondition(t, 2*time.Second, func() bool {
		all := actions.snapshot()
		return findNotifyIndex(all, "Break Complete") >= 0 && findActionIndex(all, "unlock") >= 0
	}, "expected break end unlock + notification")

	all := actions.snapshot()
	breakStartedNotifyIdx := findNotifyIndex(all, "Break Started")
	firstLockIdx := findActionIndex(all, "lock")
	if breakStartedNotifyIdx == -1 || firstLockIdx == -1 || breakStartedNotifyIdx > firstLockIdx {
		t.Fatalf("expected Break Started notify before first lock, actions=%v", summarizeActions(all))
	}
}

func TestRuntimeCooldownUnlockRelocksAfterDelay(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 25 * time.Millisecond
	cfg.TaskMedium = 50 * time.Millisecond
	cfg.CooldownShort = 80 * time.Millisecond
	cfg.CooldownStartDelay = 10 * time.Millisecond
	cfg.RelockDelay = 15 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})

	actions := &recordingActions{}
	rt := NewDaemonRuntime(actions)
	t.Cleanup(rt.Close)

	if _, err := rt.StartTask("cooldown-task", cfg.TaskShort); err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}

	waitForCondition(t, 2*time.Second, func() bool {
		return findNotifyIndex(actions.snapshot(), "Task Complete") >= 0
	}, "expected task complete notification")

	waitForCondition(t, 2*time.Second, func() bool {
		return findActionIndex(actions.snapshot(), "lock") >= 1
	}, "expected cooldown lock to occur")

	rt.OnScreenUnlocked()

	waitForCondition(t, 2*time.Second, func() bool {
		all := actions.snapshot()
		return findNotifyIndex(all, "Cooldown Active") >= 0 && countActionKind(all, "lock") >= 2
	}, "expected cooldown relock after unlock")
}

func TestRuntimeCompletionAlertLoopStopsOnIdleExitAndUnlock(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatInterval = 10 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})

	actions := &recordingActions{}
	rt := NewDaemonRuntime(actions)
	t.Cleanup(rt.Close)

	rt.mu.Lock()
	rt.idleActive = true
	rt.mu.Unlock()
	rt.startCompletionAlert()

	waitForCondition(t, time.Second, func() bool {
		return actions.count("sound") >= 2
	}, "expected repeated sound loop while idle")

	rt.OnIdleExited()
	before := actions.count("sound")
	time.Sleep(50 * time.Millisecond)
	after := actions.count("sound")
	if after != before {
		t.Fatalf("sound loop did not stop on idle exit: before=%d after=%d", before, after)
	}

	rt.mu.Lock()
	rt.idleActive = true
	rt.mu.Unlock()
	rt.startCompletionAlert()
	waitForCondition(t, time.Second, func() bool {
		return actions.count("sound") >= before+2
	}, "expected sound loop to restart")

	rt.OnScreenUnlocked()
	beforeUnlock := actions.count("sound")
	time.Sleep(50 * time.Millisecond)
	afterUnlock := actions.count("sound")
	if afterUnlock != beforeUnlock {
		t.Fatalf("sound loop did not stop on screen unlock: before=%d after=%d", beforeUnlock, afterUnlock)
	}
}

func TestRuntimeUsesInjectedClockForTaskStartTime(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = time.Minute
	cfg.TaskMedium = 2 * time.Minute
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})

	fixedNow := time.Date(2026, 3, 22, 10, 0, 0, 0, time.UTC)
	clock := &fixedClock{now: fixedNow}
	rt := NewDaemonRuntimeWithClock(&recordingActions{}, clock)
	t.Cleanup(rt.Close)

	task, err := rt.StartTask("clocked", cfg.TaskShort)
	if err != nil {
		t.Fatalf("StartTask failed: %v", err)
	}
	if !task.StartTime.Equal(fixedNow) {
		t.Fatalf("task start time = %s, want %s", task.StartTime, fixedNow)
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timeout: %s", message)
}

func findActionIndex(actions []recordedAction, kind string) int {
	for i, a := range actions {
		if a.kind == kind {
			return i
		}
	}
	return -1
}

func findNotifyIndex(actions []recordedAction, title string) int {
	for i, a := range actions {
		if a.kind == "notify" && a.title == title {
			return i
		}
	}
	return -1
}

func countActionKind(actions []recordedAction, kind string) int {
	total := 0
	for _, a := range actions {
		if a.kind == kind {
			total++
		}
	}
	return total
}

func summarizeActions(actions []recordedAction) []string {
	out := make([]string, 0, len(actions))
	for _, a := range actions {
		if a.kind == "notify" {
			out = append(out, fmt.Sprintf("notify:%s", a.title))
			continue
		}
		out = append(out, a.kind)
	}
	return out
}

type fixedClock struct {
	now time.Time
}

func (c *fixedClock) Now() time.Time {
	return c.now
}

func (c *fixedClock) Since(t time.Time) time.Duration {
	return c.now.Sub(t)
}

func (c *fixedClock) Until(t time.Time) time.Duration {
	return t.Sub(c.now)
}

func (c *fixedClock) AfterFunc(d time.Duration, fn func()) runtimeTimer {
	return time.AfterFunc(d, fn)
}

func (c *fixedClock) NewTimer(d time.Duration) runtimeSleepTimer {
	return &realSleepTimer{timer: time.NewTimer(d)}
}
