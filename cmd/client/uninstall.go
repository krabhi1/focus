package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Uninstall(prefix string) error {
	bindir, err := resolveBinDir(prefix)
	if err != nil {
		return err
	}

	if err := removeUserService(); err != nil {
		return err
	}

	if err := removeInstalledBinaries(bindir); err != nil {
		return err
	}

	return nil
}

func resolveBinDir(prefix string) (string, error) {
	if prefix != "" {
		return filepath.Join(prefix, "bin"), nil
	}

	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable: %w", err)
	}
	if strings.HasPrefix(exe, os.TempDir()) || strings.Contains(exe, "go-build") {
		return "", fmt.Errorf("current executable looks temporary; use --prefix or scripts/uninstall.sh instead")
	}
	return filepath.Dir(exe), nil
}

func removeUserService() error {
	servicePath, err := userServicePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(servicePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect service file: %w", err)
	}

	if runtime.GOOS == "linux" {
		cmd := exec.Command("systemctl", "--user", "disable", "--now", "focusd.service")
		_ = cmd.Run()
	}

	if err := os.Remove(servicePath); err != nil {
		return fmt.Errorf("remove service file: %w", err)
	}

	if runtime.GOOS == "linux" {
		_ = exec.Command("systemctl", "--user", "daemon-reload").Run()
		_ = exec.Command("systemctl", "--user", "reset-failed").Run()
	}

	return nil
}

func removeInstalledBinaries(bindir string) error {
	files := []string{
		filepath.Join(bindir, "focus"),
		filepath.Join(bindir, "focusd"),
		filepath.Join(bindir, "focus-events"),
	}

	for _, file := range files {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file, err)
		}
	}

	prefix := filepath.Dir(bindir)
	assetsDir := filepath.Join(prefix, "share", "focus", "assets")
	if err := os.RemoveAll(assetsDir); err != nil {
		return fmt.Errorf("remove assets directory: %w", err)
	}

	_ = os.Remove(filepath.Join(prefix, "share", "focus"))

	return nil
}
