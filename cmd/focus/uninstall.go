package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var readUninstallConfirmationFn = readUninstallConfirmation

func Uninstall(prefix string) error {
	bindir, err := resolveBinDir(prefix)
	if err != nil {
		return err
	}

	for step := 1; step <= 3; step++ {
		fmt.Printf("%s (%d/3) [y/N]: ", colorPrompt("Are you sure you want to uninstall focus"), step)
		answer, err := readUninstallConfirmationFn()
		if err != nil {
			return err
		}
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("uninstall cancelled")
		}
	}

	if err := removeUserService(); err != nil {
		return err
	}

	if err := removeInstalledBinaries(bindir); err != nil {
		return err
	}

	return nil
}

func readUninstallConfirmation() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read confirmation: %w", err)
	}
	return strings.TrimSpace(strings.ToLower(answer)), nil
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
	prefix := filepath.Dir(bindir)
	for _, file := range []string{
		filepath.Join(bindir, "focus"),
		filepath.Join(bindir, "focusd"),
		filepath.Join(bindir, "focus-events"),
	} {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file, err)
		}
	}
	libexecDir := filepath.Join(prefix, "libexec", "focus")
	for _, file := range []string{
		filepath.Join(libexecDir, "focusd"),
		filepath.Join(libexecDir, "focus-events"),
	} {
		if err := os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", file, err)
		}
	}
	_ = os.Remove(libexecDir)
	_ = os.Remove(filepath.Join(prefix, "libexec"))
	assetsDir := filepath.Join(prefix, "share", "focus", "assets")
	if err := os.RemoveAll(assetsDir); err != nil {
		return fmt.Errorf("remove assets directory: %w", err)
	}

	_ = os.Remove(filepath.Join(prefix, "share", "focus"))

	return nil
}
