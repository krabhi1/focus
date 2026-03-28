package main

import (
	"bytes"
	"errors"
	"focus/internal/app"
	"focus/internal/effects"
	"focus/internal/events"
	"focus/internal/storage"
	"log"
	"strings"
	"testing"
	"time"
)

func TestWarnMissingRuntimeDependencies(t *testing.T) {
	var calls []string
	lookPath := func(name string) (string, error) {
		calls = append(calls, name)
		if name == "notify-send" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + name, nil
	}

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	warnMissingRuntimeDependencies(lookPath)

	if len(calls) != 3 {
		t.Fatalf("lookPath calls = %d, want 3", len(calls))
	}
	got := buf.String()
	if !strings.Contains(got, "missing dependency 'notify-send'") {
		t.Fatalf("log output = %q, want missing notify-send warning", got)
	}
	if strings.Contains(got, "screen lock/unlock backend") {
		t.Fatalf("log output = %q, did not expect warning for screen backend", got)
	}
	if strings.Contains(got, "sound backend") {
		t.Fatalf("log output = %q, did not expect warning for sound backend", got)
	}
}

func TestWarnMissingRuntimeDependenciesWarnsOncePerBackendGroup(t *testing.T) {
	var calls []string
	lookPath := func(name string) (string, error) {
		calls = append(calls, name)
		switch name {
		case "notify-send", "loginctl", "xdg-screensaver", "cinnamon-screensaver-command", "gnome-screensaver-command", "paplay", "pw-play", "aplay", "mpv", "ffplay", "cvlc", "mpg123":
			return "", errors.New("not found")
		default:
			return "/usr/bin/" + name, nil
		}
	}

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	warnMissingRuntimeDependencies(lookPath)

	if len(calls) != 12 {
		t.Fatalf("lookPath calls = %d, want 12", len(calls))
	}
	got := buf.String()
	if !strings.Contains(got, "missing dependency 'notify-send'") {
		t.Fatalf("log output = %q, want missing notify-send warning", got)
	}
	if !strings.Contains(got, "missing dependency 'screen lock/unlock backend'") {
		t.Fatalf("log output = %q, want missing screen backend warning", got)
	}
	if !strings.Contains(got, "missing dependency 'sound backend'") {
		t.Fatalf("log output = %q, want missing sound backend warning", got)
	}
}

func TestIsHelperFatalError(t *testing.T) {
	if !isHelperFatalError(errors.New("focus-events exited: exit status 1")) {
		t.Fatal("expected helper exit error to be fatal")
	}
	if isHelperFatalError(errors.New("read helper stdout: frame parse failed")) {
		t.Fatal("expected non-exit error to be non-fatal")
	}
	if isHelperFatalError(nil) {
		t.Fatal("expected nil error to be non-fatal")
	}
}

func TestMonitorHelperErrorsCancelsOnFatal(t *testing.T) {
	errCh := make(chan error, 2)
	helperFatal := make(chan error, 1)
	cancelled := false
	cancel := func() {
		cancelled = true
	}

	go monitorHelperErrors(errCh, cancel, helperFatal)

	errCh <- errors.New("frame decode failed")
	errCh <- errors.New("focus-events exited: exit status 1")
	close(errCh)

	select {
	case err := <-helperFatal:
		if err == nil || !strings.Contains(err.Error(), "focus-events exited") {
			t.Fatalf("unexpected helper fatal error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for helper fatal error")
	}

	if !cancelled {
		t.Fatal("expected cancel to be called for fatal helper error")
	}
}

func TestConsumeHelperEventsTraceLoggingIsGated(t *testing.T) {
	t.Setenv("FOCUS_TRACE_FLOW", "")

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	rt := appForTest(t)
	eventCh := make(chan events.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeHelperEvents(eventCh, rt)
	}()

	eventCh <- events.Event{Kind: events.KindScreen, State: "locked"}
	close(eventCh)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for helper event consumer")
	}

	if got := buf.String(); got != "" {
		t.Fatalf("log output = %q, want empty output when trace is disabled", got)
	}
}

func TestConsumeHelperEventsTraceLoggingEnabled(t *testing.T) {
	t.Setenv("FOCUS_TRACE_FLOW", "1")

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()

	rt := appForTest(t)
	eventCh := make(chan events.Event, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeHelperEvents(eventCh, rt)
	}()

	eventCh <- events.Event{Kind: events.KindScreen, State: "locked"}
	close(eventCh)

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for helper event consumer")
	}

	if got := buf.String(); !strings.Contains(got, "focus-events event=screen state=locked") {
		t.Fatalf("log output = %q, want focus-events trace log", got)
	}
}

func TestConsumeHelperEventsSleepsPauseAndResumeTaskTimers(t *testing.T) {
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

	rt := appForTest(t)
	task, err := rt.StartTask("demo", cfg.TaskLong, false)
	if err != nil {
		t.Fatalf("StartTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("task = nil, want task")
	}

	eventCh := make(chan events.Event, 4)
	done := make(chan struct{})
	go func() {
		defer close(done)
		consumeHelperEvents(eventCh, rt)
	}()

	eventCh <- events.Event{Kind: events.KindSleep, State: "prepare"}
	time.Sleep(80 * time.Millisecond)
	if got := rt.Snapshot().Phase; got != "active" {
		t.Fatalf("phase while sleep-paused = %s, want active", got)
	}

	eventCh <- events.Event{Kind: events.KindSleep, State: "resume"}
	close(eventCh)

	deadline := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(deadline) {
		if got := rt.Snapshot().Phase; got == "break" {
			<-done
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	<-done
	t.Fatalf("phase after sleep resume = %s, want break", rt.Snapshot().Phase)
}

func appForTest(t *testing.T) *app.Runtime {
	t.Helper()
	rt := app.NewRuntime(effects.NoopActions{})
	t.Cleanup(rt.Close)
	return rt
}
