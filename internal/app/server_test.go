package app

import (
	"focus/internal/effects"
	"focus/internal/protocol"
	"focus/internal/storage"
	"strings"
	"testing"
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
	if !strings.Contains(res.Success.Message, "demo") {
		t.Fatalf("history = %q, want demo", res.Success.Message)
	}
}

var errReloadTest = &reloadTestError{"boom"}

type reloadTestError struct {
	msg string
}

func (e *reloadTestError) Error() string {
	return e.msg
}
