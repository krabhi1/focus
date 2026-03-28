package main

import (
	"strings"
	"testing"
)

func TestColorTextDisabledWhenTTYUnavailable(t *testing.T) {
	oldTTY := stdoutIsTerminalFn
	oldNoColor := noColorDisabledFn
	t.Cleanup(func() {
		stdoutIsTerminalFn = oldTTY
		noColorDisabledFn = oldNoColor
	})

	stdoutIsTerminalFn = func() bool { return false }
	noColorDisabledFn = func() bool { return false }

	if got := colorText("focus", ansiGreen); got != "focus" {
		t.Fatalf("colorText() = %q, want plain text", got)
	}
}

func TestColorTextDisabledByNoColor(t *testing.T) {
	oldTTY := stdoutIsTerminalFn
	oldNoColor := noColorDisabledFn
	t.Cleanup(func() {
		stdoutIsTerminalFn = oldTTY
		noColorDisabledFn = oldNoColor
	})

	stdoutIsTerminalFn = func() bool { return true }
	noColorDisabledFn = func() bool { return true }

	if got := colorText("focus", ansiGreen); got != "focus" {
		t.Fatalf("colorText() = %q, want plain text with NO_COLOR", got)
	}
}

func TestColorTextEnabled(t *testing.T) {
	oldTTY := stdoutIsTerminalFn
	oldNoColor := noColorDisabledFn
	t.Cleanup(func() {
		stdoutIsTerminalFn = oldTTY
		noColorDisabledFn = oldNoColor
	})

	stdoutIsTerminalFn = func() bool { return true }
	noColorDisabledFn = func() bool { return false }

	got := colorText("focus", ansiGreen)
	want := ansiGreen + "focus" + ansiReset
	if got != want {
		t.Fatalf("colorText() = %q, want %q", got, want)
	}
}

func TestColorStatusMessageStylesKnownStates(t *testing.T) {
	oldTTY := stdoutIsTerminalFn
	oldNoColor := noColorDisabledFn
	t.Cleanup(func() {
		stdoutIsTerminalFn = oldTTY
		noColorDisabledFn = oldNoColor
	})

	stdoutIsTerminalFn = func() bool { return true }
	noColorDisabledFn = func() bool { return false }

	cases := []struct {
		name    string
		message string
		style   string
	}{
		{name: "idle", message: "Idle", style: ansiDim},
		{name: "cooldown", message: "Cooldown active | Remaining: 1m0s", style: ansiYellow},
		{name: "break", message: "Task: demo | Status: break | Break remaining: 1m0s | Task remaining: 4m0s", style: ansiYellow},
		{name: "task", message: "Task: demo | Remaining: 4m0s", style: ansiGreen},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := colorStatusMessage(tc.message)
			if !strings.HasPrefix(got, tc.style) {
				t.Fatalf("colorStatusMessage() = %q, want prefix %q", got, tc.style)
			}
		})
	}
}

func TestColorHistoryMessageStylesLines(t *testing.T) {
	oldTTY := stdoutIsTerminalFn
	oldNoColor := noColorDisabledFn
	t.Cleanup(func() {
		stdoutIsTerminalFn = oldTTY
		noColorDisabledFn = oldNoColor
	})

	stdoutIsTerminalFn = func() bool { return true }
	noColorDisabledFn = func() bool { return false }

	got := colorHistoryMessage("[1] demo | 30m0s | completed | started 2026-03-28T10:00:00Z")
	for _, want := range []string{ansiCyan, ansiGreen, ansiDim} {
		if !strings.Contains(got, want) {
			t.Fatalf("colorHistoryMessage() = %q, want ANSI %q", got, want)
		}
	}
}
