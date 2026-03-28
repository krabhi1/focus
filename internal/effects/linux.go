package effects

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RealActions struct{}
type NoopActions struct{}

type commandSpec struct {
	name string
	args []string
}

var (
	commandLookPath = exec.LookPath
	commandRun      = func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	}
	sessionIDValue = func() string {
		return strings.TrimSpace(os.Getenv("XDG_SESSION_ID"))
	}
)

func resolveAssetPath(path string) string {
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	if _, err := os.Stat(path); err == nil {
		return path
	}
	exePath, err := os.Executable()
	if err != nil {
		return path
	}
	base := filepath.Dir(exePath)
	candidates := []string{filepath.Join(base, path)}
	clean := filepath.ToSlash(path)
	if strings.HasPrefix(clean, "assets/") {
		assetName := strings.TrimPrefix(clean, "assets/")
		prefix := filepath.Dir(base)
		candidates = append(candidates, filepath.Join(prefix, "share", "focus", "assets", filepath.FromSlash(assetName)))
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}
	return path
}

func (RealActions) LockScreen() {
	tryCommands(lockBackends())
}

func (RealActions) UnlockScreen() {
	tryCommands(unlockBackends())
}

func lockBackends() []commandSpec {
	var specs []commandSpec
	if sessionID := sessionIDValue(); sessionID != "" {
		specs = append(specs, commandSpec{
			name: "loginctl",
			args: []string{"lock-session", sessionID},
		})
	}
	specs = append(specs,
		commandSpec{name: "xdg-screensaver", args: []string{"lock"}},
		commandSpec{name: "cinnamon-screensaver-command", args: []string{"-l"}},
		commandSpec{name: "gnome-screensaver-command", args: []string{"-l"}},
	)
	return specs
}

func unlockBackends() []commandSpec {
	var specs []commandSpec
	if sessionID := sessionIDValue(); sessionID != "" {
		specs = append(specs, commandSpec{
			name: "loginctl",
			args: []string{"unlock-session", sessionID},
		})
	}
	specs = append(specs,
		commandSpec{name: "cinnamon-screensaver-command", args: []string{"-d"}},
		commandSpec{name: "gnome-screensaver-command", args: []string{"-d"}},
	)
	return specs
}

func tryCommands(specs []commandSpec) {
	for _, spec := range specs {
		if spec.name == "" {
			continue
		}
		if _, err := commandLookPath(spec.name); err != nil {
			continue
		}
		if err := commandRun(spec.name, spec.args...); err == nil {
			return
		}
	}
}

func (RealActions) PlaySound(path string) {
	resolvedPath := resolveAssetPath(path)
	tryCommands(soundBackends(resolvedPath))
}

func (RealActions) Notify(title, message string) {
	_ = exec.Command("notify-send", "-e", "-t", "2000", "-i", "dialog-information", title, message).Run()
}

func soundBackends(path string) []commandSpec {
	return []commandSpec{
		{name: "paplay", args: []string{path}},
		{name: "pw-play", args: []string{path}},
		{name: "aplay", args: []string{path}},
		{name: "mpv", args: []string{"--no-video", "--really-quiet", path}},
		{name: "ffplay", args: []string{"-nodisp", "-autoexit", "-loglevel", "quiet", path}},
		{name: "cvlc", args: []string{"--play-and-exit", "--intf", "dummy", path}},
		{name: "mpg123", args: []string{path}},
	}
}

func (NoopActions) LockScreen()           {}
func (NoopActions) UnlockScreen()         {}
func (NoopActions) PlaySound(string)      {}
func (NoopActions) Notify(string, string) {}
