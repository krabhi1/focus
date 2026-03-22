package main

import (
	"focus/internal/core"
	"focus/internal/protocol"
	"focus/internal/sys"
	"strings"
	"testing"
)

func TestHandleReloadSuccess(t *testing.T) {
	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, sys.NoopActions{}, func() error {
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
	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, sys.NoopActions{}, func() error {
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
	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, sys.NoopActions{}, nil)

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
	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, sys.NoopActions{}, nil)

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
	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	srv := NewServer(rt, sys.NoopActions{}, nil)
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

func TestStartDecisionFromCore(t *testing.T) {
	cases := []struct {
		name    string
		phase   core.Phase
		wantErr bool
		wantMsg string
	}{
		{name: "idle allows", phase: core.PhaseIdle, wantErr: false},
		{name: "pending cooldown blocks", phase: core.PhasePendingCooldown, wantErr: true, wantMsg: "cooldown active"},
		{name: "cooldown blocks", phase: core.PhaseCooldown, wantErr: true, wantMsg: "cooldown active"},
		{name: "break blocks", phase: core.PhaseBreak, wantErr: true, wantMsg: "break active"},
		{name: "active blocks", phase: core.PhaseActive, wantErr: true, wantMsg: "already active"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := startDecisionFromCore(core.State{Phase: tc.phase})
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

func TestCancelDecisionFromCore(t *testing.T) {
	cases := []struct {
		name    string
		phase   core.Phase
		wantErr bool
		wantMsg string
	}{
		{name: "idle blocks", phase: core.PhaseIdle, wantErr: true, wantMsg: "no active task"},
		{name: "pending cooldown blocks", phase: core.PhasePendingCooldown, wantErr: true, wantMsg: "no active task"},
		{name: "cooldown blocks", phase: core.PhaseCooldown, wantErr: true, wantMsg: "no active task"},
		{name: "active allows", phase: core.PhaseActive, wantErr: false},
		{name: "break allows", phase: core.PhaseBreak, wantErr: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := cancelDecisionFromCore(core.State{Phase: tc.phase})
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error = %q, want contains %q", err.Error(), tc.wantMsg)
			}
		})
	}
}

var errReloadTest = &reloadTestError{"boom"}

type reloadTestError struct {
	msg string
}

func (e *reloadTestError) Error() string {
	return e.msg
}
