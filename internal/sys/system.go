package sys

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Actions interface {
	LockScreen()
	UnlockScreen()
	PlaySound(path string)
	Notify(title, message string)
}

type RealActions struct{}

type NoopActions struct{}

func realLockScreen() {
	cmd := exec.Command("xdg-screensaver", "lock")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error locking screen: %v\n", err)
	}
}

func realUnlockScreen() {
	cmd := exec.Command("cinnamon-screensaver-command", "-d")
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error unlocking screen: %v\n", err)
	}
}

func realPlaySound(path string) {
	resolvedPath := resolveAssetPath(path)
	cmd := exec.Command("paplay", resolvedPath)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error playing sound: %v\n", err)
	}
}

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

	candidates := []string{
		filepath.Join(base, path),
	}

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

func realNotify(title, message string) {
	cmd := exec.Command("notify-send", "-i", "dialog-information", title, message)
	err := cmd.Run()
	if err != nil {
		fmt.Printf("Error sending notification: %v\n", err)
	}
}

func (RealActions) LockScreen() {
	realLockScreen()
}

func (RealActions) UnlockScreen() {
	realUnlockScreen()
}

func (RealActions) PlaySound(path string) {
	realPlaySound(path)
}

func (RealActions) Notify(title, message string) {
	realNotify(title, message)
}

func (NoopActions) LockScreen()           {}
func (NoopActions) UnlockScreen()         {}
func (NoopActions) PlaySound(string)      {}
func (NoopActions) Notify(string, string) {}
