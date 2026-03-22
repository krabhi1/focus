package main

import (
	"encoding/gob"
	"focus/internal/core"
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
	cfg.CooldownShort = 200 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})
	restoreCooldownDelay := setCooldownStartDelayForTest(10 * time.Millisecond)
	t.Cleanup(restoreCooldownDelay)

	rt := newTestRuntime(t)

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
	srv := NewServer(rt, sys.NoopActions{}, nil)
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
	rt := newTestRuntime(t)

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
	srv := NewServer(rt, sys.NoopActions{}, func() error {
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

func TestCoreBackedStatusTaskToCooldownToIdle(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 25 * time.Millisecond
	cfg.TaskMedium = 50 * time.Millisecond
	cfg.CooldownShort = 60 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})
	restoreCooldownDelay := setCooldownStartDelayForTest(10 * time.Millisecond)
	t.Cleanup(restoreCooldownDelay)

	rt := newTestRuntime(t)
	srv := newCoreBackedServerForTest(t, rt)

	start := srv.handleRequest(protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "flow-task",
			Preset: "short",
		},
	})
	assertSuccessMessageContains(t, start, "Started task: flow-task")

	waitForStatusContains(t, srv, "Cooldown starting", 1*time.Second)
	waitForStatusContains(t, srv, "Cooldown active", 1*time.Second)
	waitForStatusContains(t, srv, "Idle", 2*time.Second)
}

func TestCoreBackedStatusShowsBreakBeforeCooldown(t *testing.T) {
	cfg := state.DefaultRuntimeConfig()
	cfg.TaskShort = 20 * time.Millisecond
	cfg.TaskMedium = 40 * time.Millisecond
	cfg.TaskLong = 80 * time.Millisecond
	cfg.TaskDeep = 150 * time.Millisecond
	cfg.BreakLongStart = 10 * time.Millisecond
	cfg.BreakLongDuration = 20 * time.Millisecond
	cfg.BreakDeepStart = 30 * time.Millisecond
	cfg.BreakDeepDuration = 20 * time.Millisecond
	cfg.BreakWarning = 1 * time.Millisecond
	cfg.BreakRelockDelay = 5 * time.Millisecond
	cfg.CooldownLong = 60 * time.Millisecond
	if err := state.SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}
	t.Cleanup(func() {
		_ = state.SetRuntimeConfig(state.DefaultRuntimeConfig())
	})
	restoreCooldownDelay := setCooldownStartDelayForTest(10 * time.Millisecond)
	t.Cleanup(restoreCooldownDelay)

	rt := newTestRuntime(t)
	srv := newCoreBackedServerForTest(t, rt)

	start := srv.handleRequest(protocol.Request{
		Command: "start",
		Start: &protocol.StartRequest{
			Title:  "break-flow-task",
			Preset: "long",
		},
	})
	assertSuccessMessageContains(t, start, "Started task: break-flow-task")

	waitForStatusContains(t, srv, "Task active", 500*time.Millisecond)
	waitForStatusContains(t, srv, "Status: break", 1*time.Second)
	waitForStatusContains(t, srv, "Cooldown starting", 2*time.Second)
	waitForStatusContains(t, srv, "Cooldown active", 2*time.Second)
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

func TestFormatCoreStatus(t *testing.T) {
	now := time.Now()
	if got := formatCoreStatus(core.State{Phase: core.PhaseIdle}); got != "Idle" {
		t.Fatalf("idle status = %q, want Idle", got)
	}

	pending := formatCoreStatus(core.State{
		Phase:              core.PhasePendingCooldown,
		CooldownStartUntil: now.Add(2 * time.Second),
	})
	if !strings.Contains(pending, "Cooldown starting") {
		t.Fatalf("pending status = %q, want cooldown starting", pending)
	}

	cooldown := formatCoreStatus(core.State{
		Phase:         core.PhaseCooldown,
		CooldownUntil: now.Add(2 * time.Second),
	})
	if !strings.Contains(cooldown, "Cooldown active") {
		t.Fatalf("cooldown status = %q, want cooldown active", cooldown)
	}

	brk := formatCoreStatus(core.State{
		Phase:      core.PhaseBreak,
		BreakUntil: now.Add(2 * time.Second),
	})
	if !strings.Contains(brk, "Status: break") {
		t.Fatalf("break status = %q, want break status", brk)
	}
}

func newTestRuntime(t *testing.T) *DaemonRuntime {
	t.Helper()

	rt := NewDaemonRuntime(sys.NoopActions{})
	t.Cleanup(rt.Close)
	return rt
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

func newCoreBackedServerForTest(t *testing.T, rt *DaemonRuntime) *Server {
	t.Helper()

	srv := NewServer(rt, sys.NoopActions{}, nil)
	srv.SetStatusProvider(func() string {
		return formatCoreStatus(rt.CoreSnapshot())
	})
	return srv
}

func waitForStatusContains(t *testing.T, srv *Server, want string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	last := ""
	for time.Now().Before(deadline) {
		res := srv.handleRequest(protocol.Request{Command: "status"})
		if res.Success == nil {
			t.Fatalf("status response = %#v, want success", res)
		}
		last = res.Success.Message
		if strings.Contains(last, want) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Fatalf("status did not contain %q within %s; last=%q", want, timeout, last)
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
