package effects

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestRealActionsLockScreenPrefersLoginctl(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	t.Setenv("XDG_SESSION_ID", "c1")
	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "loginctl" {
			return "", errors.New("missing")
		}
		return "/usr/bin/loginctl", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.LockScreen()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 loginctl call", calls)
	}
	if got := calls[0]; got != "loginctl lock-session c1" {
		t.Fatalf("call = %q, want loginctl lock-session c1", got)
	}
}

func TestRealActionsLockScreenPrefersCinnamonDesktop(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	t.Setenv("XDG_CURRENT_DESKTOP", "Cinnamon")
	t.Setenv("DESKTOP_SESSION", "cinnamon")
	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "cinnamon-screensaver-command" {
			return "", errors.New("missing")
		}
		return "/usr/bin/cinnamon-screensaver-command", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.LockScreen()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 cinnamon call", calls)
	}
	if got := calls[0]; got != "cinnamon-screensaver-command -l" {
		t.Fatalf("call = %q, want cinnamon-screensaver-command -l", got)
	}
}

func TestRealActionsLockScreenFallsBackToXdgScreensaver(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	commandLookPath = func(name string) (string, error) {
		if name == "xdg-screensaver" {
			return "/usr/bin/xdg-screensaver", nil
		}
		return "", errors.New("missing")
	}
	commandRun = func(name string, args ...string) error {
		if name != "xdg-screensaver" {
			t.Fatalf("unexpected command %q", name)
		}
		if len(args) != 1 || args[0] != "lock" {
			t.Fatalf("args = %v, want lock", args)
		}
		return nil
	}

	RealActions{}.LockScreen()
}

func TestRealActionsUnlockScreenDoesNothingWhenUnavailable(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	commandLookPath = func(name string) (string, error) {
		return "", errors.New("missing")
	}
	runs := 0
	commandRun = func(name string, args ...string) error {
		runs++
		return nil
	}

	RealActions{}.UnlockScreen()

	if runs != 0 {
		t.Fatalf("command runs = %d, want 0", runs)
	}
}

func TestRealActionsUnlockScreenUsesLoginctlWhenAvailable(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	t.Setenv("XDG_SESSION_ID", "c1")
	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "loginctl" {
			return "", errors.New("missing")
		}
		return "/usr/bin/loginctl", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.UnlockScreen()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 loginctl call", calls)
	}
	if got := calls[0]; got != "loginctl unlock-session c1" {
		t.Fatalf("call = %q, want loginctl unlock-session c1", got)
	}
}

func TestRealActionsUnlockScreenPrefersCinnamonDesktop(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	t.Setenv("XDG_CURRENT_DESKTOP", "Cinnamon")
	t.Setenv("DESKTOP_SESSION", "cinnamon")
	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "cinnamon-screensaver-command" {
			return "", errors.New("missing")
		}
		return "/usr/bin/cinnamon-screensaver-command", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.UnlockScreen()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 cinnamon call", calls)
	}
	if got := calls[0]; got != "cinnamon-screensaver-command -d" {
		t.Fatalf("call = %q, want cinnamon-screensaver-command -d", got)
	}
}

func TestRealActionsPlaySoundPrefersPaplay(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "paplay" {
			return "", errors.New("missing")
		}
		return "/usr/bin/paplay", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.PlaySound("assets/task-ending.mp3")

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 paplay call", calls)
	}
	if got := calls[0]; !strings.HasPrefix(got, "paplay ") {
		t.Fatalf("call = %q, want paplay", got)
	}
}

func TestRealActionsPlaySoundFallsBackToMpv(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name == "mpv" {
			return "/usr/bin/mpv", nil
		}
		return "", errors.New("missing")
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		if name == "mpv" {
			return nil
		}
		return errors.New("missing")
	}

	RealActions{}.PlaySound("assets/task-ending.mp3")

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 mpv call", calls)
	}
	if got := calls[0]; !strings.HasPrefix(got, "mpv --no-video --really-quiet ") {
		t.Fatalf("call = %q, want mpv fallback", got)
	}
}

func TestRealActionsPlaySoundNoopWhenUnavailable(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	commandLookPath = func(name string) (string, error) {
		return "", errors.New("missing")
	}
	runs := 0
	commandRun = func(name string, args ...string) error {
		runs++
		return nil
	}

	RealActions{}.PlaySound("assets/task-ending.mp3")

	if runs != 0 {
		t.Fatalf("command runs = %d, want 0", runs)
	}
}

func TestRealActionsSleepPrefersLoginctlWhenAvailable(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	t.Setenv("XDG_SESSION_ID", "session-123")
	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "loginctl" {
			return "", errors.New("missing")
		}
		return "/usr/bin/loginctl", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.Sleep()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 loginctl call", calls)
	}
	if got := calls[0]; got != "loginctl suspend-session session-123" {
		t.Fatalf("call = %q, want loginctl suspend-session session-123", got)
	}
}

func TestRealActionsSleepFallsBackToSystemctl(t *testing.T) {
	restore := stubCommandEnv()
	defer restore()

	calls := []string{}
	commandLookPath = func(name string) (string, error) {
		if name != "systemctl" {
			return "", errors.New("missing")
		}
		return "/usr/bin/systemctl", nil
	}
	commandRun = func(name string, args ...string) error {
		calls = append(calls, fmt.Sprintf("%s %s", name, strings.Join(args, " ")))
		return nil
	}

	RealActions{}.Sleep()

	if len(calls) != 1 {
		t.Fatalf("calls = %v, want 1 systemctl call", calls)
	}
	if got := calls[0]; got != "systemctl suspend" {
		t.Fatalf("call = %q, want systemctl suspend", got)
	}
}

func stubCommandEnv() func() {
	oldLookPath := commandLookPath
	oldRun := commandRun
	oldSessionID := sessionIDValue
	return func() {
		commandLookPath = oldLookPath
		commandRun = oldRun
		sessionIDValue = oldSessionID
	}
}
