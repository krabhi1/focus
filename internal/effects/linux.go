package effects

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type RealActions struct{}
type NoopActions struct{}

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
	if err := exec.Command("xdg-screensaver", "lock").Run(); err != nil {
		fmt.Printf("Error locking screen: %v\n", err)
	}
}

func (RealActions) UnlockScreen() {
	if err := exec.Command("cinnamon-screensaver-command", "-d").Run(); err != nil {
		fmt.Printf("Error unlocking screen: %v\n", err)
	}
}

func (RealActions) PlaySound(path string) {
	resolvedPath := resolveAssetPath(path)
	if err := exec.Command("paplay", resolvedPath).Run(); err != nil {
		fmt.Printf("Error playing sound: %v\n", err)
	}
}

func (RealActions) Notify(title, message string) {
	if err := exec.Command("notify-send", "-e", "-t", "2000", "-i", "dialog-information", title, message).Run(); err != nil {
		fmt.Printf("Error sending notification: %v\n", err)
	}
}

func (NoopActions) LockScreen()           {}
func (NoopActions) UnlockScreen()         {}
func (NoopActions) PlaySound(string)      {}
func (NoopActions) Notify(string, string) {}
