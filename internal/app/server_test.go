package app

import (
	"focus/internal/domain"
	"focus/internal/effects"
	"focus/internal/protocol"
	"focus/internal/storage"
	"strings"
	"testing"
	"time"
)

func TestHandleReloadSuccess(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, func() error { return nil })

	res := srv.handleRequest(protocol.Request{Command: "reload"})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if res.Success.Message != "Config reloaded." {
		t.Fatalf("message = %q, want %q", res.Success.Message, "Config reloaded.")
	}
}

func TestHandleReloadFailure(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, func() error {
		return errReloadTest
	})

	res := srv.handleRequest(protocol.Request{Command: "reload"})
	if res.Error == nil {
		t.Fatalf("response = %#v, want error", res)
	}
	if !strings.Contains(res.Error.Message, "reload failed") {
		t.Fatalf("error = %q, want reload failed", res.Error.Message)
	}
}

func TestHandleStartRejectsMissingDurationAndPreset(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)

	res := srv.handleRequest(protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title: "no duration",
		},
	})
	if res.Error == nil {
		t.Fatalf("response = %#v, want error", res)
	}
	if !strings.Contains(res.Error.Message, "missing task duration") {
		t.Fatalf("error = %q, want missing duration error", res.Error.Message)
	}
}

func TestHandleStartRejectsUnknownPreset(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)

	res := srv.handleRequest(protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "bad preset",
			Preset: "invalid",
		},
	})
	if res.Error == nil {
		t.Fatalf("response = %#v, want error", res)
	}
	if !strings.Contains(res.Error.Message, "unknown duration preset") {
		t.Fatalf("error = %q, want preset error", res.Error.Message)
	}
}

func TestHandleStatusUsesProviderWhenConfigured(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)
	srv.SetStatusProvider(func() string {
		return "core-status"
	})

	res := srv.handleRequest(protocol.Request{Command: "status"})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if res.Success.Message != "core-status" {
		t.Fatalf("status = %q, want %q", res.Success.Message, "core-status")
	}
}

func TestHandleDebugReturnsRuntimeDump(t *testing.T) {
	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)

	res := srv.handleRequest(protocol.Request{Command: "debug"})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if !strings.Contains(res.Success.Message, "phase: idle") {
		t.Fatalf("debug = %q, want phase field", res.Success.Message)
	}
	if !strings.Contains(res.Success.Message, "config.alert.repeat_count:") {
		t.Fatalf("debug = %q, want config snapshot", res.Success.Message)
	}
}

func TestHistoryResponseShowsTodayTasks(t *testing.T) {
	cfg := storage.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * 1e6
	if err := storage.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.SetRuntimeConfig(storage.DefaultRuntimeConfig())
	})

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)

	if _, err := rt.StartTask("demo", 20*1e6, false); err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if _, err := rt.CancelCurrentTask(); err != nil {
		t.Fatalf("CancelCurrentTask returned error: %v", err)
	}

	res := srv.handleRequest(protocol.Request{Command: "history"})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if !strings.Contains(res.Success.Message, "No task history") {
		t.Fatalf("history = %q, want empty history", res.Success.Message)
	}
}

func TestHistoryResponseShowsAllPersistedTasks(t *testing.T) {
	dir := t.TempDir()
	historyPath := dir + "/history.jsonl"
	t.Setenv("FOCUS_HISTORY_FILE", historyPath)

	rt := NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, effects.NoopActions{}, nil)

	if err := storage.AppendCompletedTask(domain.Task{
		ID:        1,
		Title:     "first",
		Duration:  30 * time.Minute,
		StartTime: time.Now().Add(-24 * time.Hour),
	}); err != nil {
		t.Fatalf("AppendCompletedTask(first) failed: %v", err)
	}
	if err := storage.AppendCompletedTask(domain.Task{
		ID:        2,
		Title:     "second",
		Duration:  45 * time.Minute,
		StartTime: time.Now(),
	}); err != nil {
		t.Fatalf("AppendCompletedTask(second) failed: %v", err)
	}

	res := srv.handleRequest(protocol.Request{Command: "history", HistoryAll: true})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if strings.Contains(res.Success.Message, "No task history") {
		t.Fatalf("history = %q, want all persisted tasks", res.Success.Message)
	}
	if !strings.Contains(res.Success.Message, "first") || !strings.Contains(res.Success.Message, "second") {
		t.Fatalf("history = %q, want both persisted tasks", res.Success.Message)
	}
}

var errReloadTest = &reloadTestError{"boom"}

type reloadTestError struct {
	msg string
}

func (e *reloadTestError) Error() string {
	return e.msg
}
