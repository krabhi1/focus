package main

import (
	"focus/internal/protocol"
	"focus/internal/state"
	"focus/internal/sys"
	"strings"
	"testing"
)

func TestHandleReloadSuccess(t *testing.T) {
	srv := NewServer(&state.DaemonState{}, sys.NoopActions{}, func() error {
		return nil
	})

	res := srv.handleRequest(protocol.Request{Command: "reload"})
	if res.Success == nil {
		t.Fatalf("response = %#v, want success", res)
	}
	if res.Success.Message != "Config reloaded." {
		t.Fatalf("message = %q, want %q", res.Success.Message, "Config reloaded.")
	}
}

func TestHandleReloadFailure(t *testing.T) {
	srv := NewServer(&state.DaemonState{}, sys.NoopActions{}, func() error {
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
	srv := NewServer(&state.DaemonState{}, sys.NoopActions{}, nil)

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
	srv := NewServer(&state.DaemonState{}, sys.NoopActions{}, nil)

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

var errReloadTest = &reloadTestError{"boom"}

type reloadTestError struct {
	msg string
}

func (e *reloadTestError) Error() string {
	return e.msg
}
