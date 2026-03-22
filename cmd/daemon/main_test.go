package main

import (
	"encoding/gob"
	"focus/internal/protocol"
	"focus/internal/state"
	"focus/internal/sys"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestConnectionStartStatusCooldownFlow(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 10 * time.Millisecond
	cfg.TaskMedium = 20 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})
	restoreCooldownDelay := state.SetCooldownStartDelayForTest(10 * time.Millisecond)
	t.Cleanup(restoreCooldownDelay)

	st := newTestState(t)

	socketPath := filepath.Join(t.TempDir(), "focus.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})

	acceptDone := make(chan struct{})
	srv := NewServer(st, sys.NoopActions{}, nil)
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go srv.HandleConnection(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
	})

	res := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "integration task",
			Preset: "short",
		},
	})
	assertSuccessMessageContains(t, res, "Started task: integration task")

	time.Sleep(40 * time.Millisecond)

	statusRes := roundTripRequest(t, socketPath, protocol.Request{Command: "status"})
	if statusRes.Success == nil {
		t.Fatalf("response = %#v, want success response", statusRes)
	}
	if !strings.Contains(statusRes.Success.Message, "Cooldown active") {
		t.Fatalf("status = %q, want cooldown", statusRes.Success.Message)
	}

	cooldownRes := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "blocked task",
			Preset: "short",
		},
	})
	if cooldownRes.Error == nil {
		t.Fatalf("response = %#v, want error response", cooldownRes)
	}
	if !strings.Contains(cooldownRes.Error.Message, "cooldown active") {
		t.Fatalf("error = %q, want cooldown rejection", cooldownRes.Error.Message)
	}

	time.Sleep(250 * time.Millisecond)

	retryRes := roundTripRequest(t, socketPath, protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "second task",
			Preset: "short",
		},
	})
	assertSuccessMessageContains(t, retryRes, "Started task: second task")

	historyRes := roundTripRequest(t, socketPath, protocol.Request{Command: "history"})
	if historyRes.Success == nil {
		t.Fatalf("response = %#v, want success response", historyRes)
	}
	if !strings.Contains(historyRes.Success.Message, "integration task") {
		t.Fatalf("history = %q, want first task", historyRes.Success.Message)
	}
	if !strings.Contains(historyRes.Success.Message, "second task") {
		t.Fatalf("history = %q, want second task", historyRes.Success.Message)
	}
}

func TestConnectionReloadFlow(t *testing.T) {
	st := newTestState(t)

	socketPath := filepath.Join(t.TempDir(), "focus.sock")
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})

	var reloadCalls atomic.Int32
	srv := NewServer(st, sys.NoopActions{}, func() error {
		reloadCalls.Add(1)
		return nil
	})

	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go srv.HandleConnection(conn)
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
	})

	res := roundTripRequest(t, socketPath, protocol.Request{Command: "reload"})
	assertSuccessMessageContains(t, res, "Config reloaded.")
	if got := reloadCalls.Load(); got != 1 {
		t.Fatalf("reload calls = %d, want 1", got)
	}
}

func TestLoadDaemonConfigKeepsHardcodedEventsDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	body := `{
		"task":{"short":"5s","medium":"10s","long":"20s","deep":"30s"},
		"cooldown":{"short":"5s","long":"6s","deep":"7s"},
		"break":{"long_start":"5s","deep_start":"10s","warning":"2s","long_duration":"3s","deep_duration":"4s","relock_delay":"2s"},
		"idle":{"warn_after":"2s","lock_after":"4s"},
		"alert":{"repeat_interval":"1s"}
	}`
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	opts := daemonOptions{configPath: configPath}
	if err := loadDaemonConfig(opts); err != nil {
		t.Fatalf("loadDaemonConfig failed: %v", err)
	}

	cfg := state.GetRuntimeConfig()
	if cfg.EventsIdleThreshold != 10*time.Second {
		t.Fatalf("EventsIdleThreshold = %s, want 10s default", cfg.EventsIdleThreshold)
	}
	if cfg.EventsIdlePoll != 5*time.Second {
		t.Fatalf("EventsIdlePoll = %s, want 5s default", cfg.EventsIdlePoll)
	}
}

func TestParseDaemonOptionsIncludesEventsOverrides(t *testing.T) {
	oldArgs := os.Args
	os.Args = []string{
		"focusd",
		"--events-idle-threshold", "7s",
		"--events-idle-poll", "2s",
	}
	t.Cleanup(func() {
		os.Args = oldArgs
	})

	opts := parseDaemonOptions()
	if opts.overrides.EventsIdleThreshold == nil || *opts.overrides.EventsIdleThreshold != 7*time.Second {
		t.Fatalf("EventsIdleThreshold override not parsed: %#v", opts.overrides.EventsIdleThreshold)
	}
	if opts.overrides.EventsIdlePoll == nil || *opts.overrides.EventsIdlePoll != 2*time.Second {
		t.Fatalf("EventsIdlePoll override not parsed: %#v", opts.overrides.EventsIdlePoll)
	}
}

func newTestState(t *testing.T) *state.DaemonState {
	t.Helper()

	st := &state.DaemonState{}
	st.SetActions(sys.NoopActions{})
	st.SetCooldownPolicyForTest(func(time.Duration) time.Duration {
		return 200 * time.Millisecond
	})

	t.Cleanup(func() {
		st.SetActions(sys.RealActions{})
		st.SetCooldownPolicyForTest(nil)
	})

	return st
}

func roundTripRequest(t *testing.T, socketPath string, req protocol.Request) protocol.Response {
	t.Helper()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("net.Dial failed: %v", err)
	}
	defer conn.Close()

	if err := gob.NewEncoder(conn).Encode(req); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	var res protocol.Response
	if err := gob.NewDecoder(conn).Decode(&res); err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	return res
}

func assertSuccessMessageContains(t *testing.T, res protocol.Response, want string) {
	t.Helper()

	if res.Success == nil {
		t.Fatalf("response = %#v, want success response", res)
	}
	if !strings.Contains(res.Success.Message, want) {
		t.Fatalf("message = %q, want substring %q", res.Success.Message, want)
	}
}
