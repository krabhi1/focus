package app

import (
	"sync"
	"testing"
	"time"

	"focus/internal/effects"
	"focus/internal/storage"
)

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

	task, err := rt.StartTask("demo", cfg.TaskShort)
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

func TestRuntimeCompletionAlertRepeatsAndStopsOnIdleExit(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatInterval = 10 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.OnIdleEntered()
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

	waitForPlays(2, 200*time.Millisecond)
	beforeExit := actions.Count()

	rt.OnIdleExited()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != beforeExit {
		t.Fatalf("plays after idle exit = %d, want %d", got, beforeExit)
	}
}

func TestRuntimeCompletionAlertStopsOnScreenUnlock(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.CompletionAlertRepeatInterval = 10 * time.Millisecond
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	actions := &soundRecorder{}
	rt := NewRuntime(actions)
	t.Cleanup(rt.Close)

	rt.OnIdleEntered()
	rt.startCompletionAlert()

	deadline := time.Now().Add(200 * time.Millisecond)
	for actions.Count() < 2 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	if actions.Count() < 2 {
		t.Fatalf("plays = %d, want at least 2", actions.Count())
	}

	beforeUnlock := actions.Count()
	rt.OnScreenUnlocked()
	time.Sleep(40 * time.Millisecond)
	if got := actions.Count(); got != beforeUnlock {
		t.Fatalf("plays after screen unlock = %d, want %d", got, beforeUnlock)
	}
}
