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

var errReloadTest = &reloadTestError{"boom"}

type reloadTestError struct {
	msg string
}

func (e *reloadTestError) Error() string {
	return e.msg
}
