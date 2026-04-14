package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDetectLockBackendPrefersLoginctlWithSession(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "loginctl")
	fakeExecutable(t, dir, "xdg-screensaver")
	fakeExecutable(t, dir, "cinnamon-screensaver-command")
	fakeExecutable(t, dir, "gnome-screensaver-command")

	t.Setenv("PATH", dir)
	t.Setenv("XDG_SESSION_ID", "session-123")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	if got := detectLockBackend(); got != "loginctl" {
		t.Fatalf("detectLockBackend() = %q, want loginctl", got)
	}
	if got := detectUnlockBackend(); got != "loginctl" {
		t.Fatalf("detectUnlockBackend() = %q, want loginctl", got)
	}
}

func TestDetectLockBackendFallsBackWithoutSession(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "xdg-screensaver")
	fakeExecutable(t, dir, "cinnamon-screensaver-command")
	fakeExecutable(t, dir, "gnome-screensaver-command")

	t.Setenv("PATH", dir)
	t.Setenv("XDG_SESSION_ID", "")
	t.Setenv("XDG_CURRENT_DESKTOP", "")
	t.Setenv("DESKTOP_SESSION", "")

	if got := detectLockBackend(); got != "xdg-screensaver" {
		t.Fatalf("detectLockBackend() = %q, want xdg-screensaver", got)
	}
	if got := detectUnlockBackend(); got != "cinnamon-screensaver-command" {
		t.Fatalf("detectUnlockBackend() = %q, want cinnamon-screensaver-command", got)
	}
}

func TestDetectLockBackendPrefersCinnamonDesktop(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "cinnamon-screensaver-command")
	fakeExecutable(t, dir, "loginctl")
	fakeExecutable(t, dir, "xdg-screensaver")

	t.Setenv("PATH", dir)
	t.Setenv("XDG_CURRENT_DESKTOP", "Cinnamon")
	t.Setenv("DESKTOP_SESSION", "cinnamon")
	t.Setenv("XDG_SESSION_ID", "session-123")

	if got := detectLockBackend(); got != "cinnamon-screensaver-command" {
		t.Fatalf("detectLockBackend() = %q, want cinnamon-screensaver-command", got)
	}
	if got := detectUnlockBackend(); got != "cinnamon-screensaver-command" {
		t.Fatalf("detectUnlockBackend() = %q, want cinnamon-screensaver-command", got)
	}
}

func TestDetectSoundBackendPrefersPaplay(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "paplay")
	fakeExecutable(t, dir, "mpv")
	fakeExecutable(t, dir, "ffplay")

	t.Setenv("PATH", dir)

	if got := detectSoundBackend(); got != "paplay" {
		t.Fatalf("detectSoundBackend() = %q, want paplay", got)
	}
}

func TestDetectSoundBackendFallsBackToMpv(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "mpv")
	fakeExecutable(t, dir, "ffplay")

	t.Setenv("PATH", dir)

	if got := detectSoundBackend(); got != "mpv" {
		t.Fatalf("detectSoundBackend() = %q, want mpv", got)
	}
}

func TestDetectSleepBackendPrefersLoginctlWithSession(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "loginctl")
	fakeExecutable(t, dir, "systemctl")

	t.Setenv("PATH", dir)
	t.Setenv("XDG_SESSION_ID", "session-123")

	if got := detectSleepBackend(); got != "loginctl" {
		t.Fatalf("detectSleepBackend() = %q, want loginctl", got)
	}
}

func TestDetectSleepBackendFallsBackToSystemctl(t *testing.T) {
	dir := t.TempDir()
	fakeExecutable(t, dir, "systemctl")

	t.Setenv("PATH", dir)
	t.Setenv("XDG_SESSION_ID", "")

	if got := detectSleepBackend(); got != "systemctl" {
		t.Fatalf("detectSleepBackend() = %q, want systemctl", got)
	}
}

func TestPrintInstalledCommandCheckUsesLibexecPath(t *testing.T) {
	prefix := t.TempDir()
	libexecDir := filepath.Join(prefix, "libexec", "focus")
	if err := os.MkdirAll(libexecDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeExecutable(t, libexecDir, "focusd")

	t.Setenv("FOCUS_LIBEXEC_DIR", libexecDir)

	var stdout bytes.Buffer
	withStdout(&stdout, func() {
		printInstalledCommandCheck("focusd", true)
	})

	got := stdout.String()
	if !strings.Contains(got, "dep.focusd:") {
		t.Fatalf("output = %q, want focusd check", got)
	}
	if !strings.Contains(got, libexecDir) {
		t.Fatalf("output = %q, want libexec path", got)
	}
}

func fakeExecutable(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	script := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		script = "@echo off\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile(%q) failed: %v", path, err)
	}
	return path
}
