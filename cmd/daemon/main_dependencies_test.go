package main

import (
	"bytes"
	"errors"
	"focus/internal/events"
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
	if strings.Contains(got, "xdg-screensaver") {
		t.Fatalf("log output = %q, did not expect warning for xdg-screensaver", got)
	}
	if strings.Contains(got, "paplay") {
		t.Fatalf("log output = %q, did not expect warning for paplay", got)
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

	rt := NewDaemonRuntime(nil)
	t.Cleanup(rt.Close)

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

	rt := NewDaemonRuntime(nil)
	t.Cleanup(rt.Close)

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
