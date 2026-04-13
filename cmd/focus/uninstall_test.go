package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesBinariesAndService(t *testing.T) {
	fixture := setupUninstallFixture(t)

	oldPrompt := readUninstallConfirmationFn
	readUninstallConfirmationFn = func() (string, error) { return "yes", nil }
	t.Cleanup(func() {
		readUninstallConfirmationFn = oldPrompt
	})

	if err := Uninstall(fixture.prefix); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	for _, name := range []string{"focus", "focusd", "focus-events"} {
		if _, err := os.Stat(filepath.Join(fixture.bindir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall", name)
		}
	}
	for _, name := range []string{"focusd", "focus-events"} {
		if _, err := os.Stat(filepath.Join(fixture.libexecDir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall", name)
		}
	}
	if _, err := os.Stat(fixture.libexecDir); !os.IsNotExist(err) {
		t.Fatalf("libexec dir still exists after uninstall")
	}
	if _, err := os.Stat(fixture.servicePath); !os.IsNotExist(err) {
		t.Fatalf("service file still exists after uninstall")
	}
	if _, err := os.Stat(fixture.assetsDir); !os.IsNotExist(err) {
		t.Fatalf("assets dir still exists after uninstall")
	}

	logBytes, err := os.ReadFile(fixture.systemctlLog)
	if err != nil {
		t.Fatalf("read systemctl log: %v", err)
	}
	log := string(logBytes)
	if !strings.Contains(log, "--user disable --now focusd.service") {
		t.Fatalf("systemctl log = %q, want disable command", log)
	}
	if !strings.Contains(log, "--user daemon-reload") {
		t.Fatalf("systemctl log = %q, want daemon-reload", log)
	}
}

func TestUninstallCancelsWhenDeclined(t *testing.T) {
	fixture := setupUninstallFixture(t)

	oldPrompt := readUninstallConfirmationFn
	readUninstallConfirmationFn = func() (string, error) { return "n", nil }
	t.Cleanup(func() {
		readUninstallConfirmationFn = oldPrompt
	})

	err := Uninstall(fixture.prefix)
	if err == nil {
		t.Fatal("Uninstall returned nil, want cancellation error")
	}
	if !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("error = %v, want cancellation", err)
	}

	for _, path := range []string{
		filepath.Join(fixture.bindir, "focus"),
		filepath.Join(fixture.libexecDir, "focusd"),
		filepath.Join(fixture.libexecDir, "focus-events"),
		fixture.servicePath,
		filepath.Join(fixture.assetsDir, "task-ending.mp3"),
	} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("%s missing after cancelled uninstall: %v", path, statErr)
		}
	}

	if _, err := os.Stat(fixture.systemctlLog); !os.IsNotExist(err) {
		t.Fatalf("systemctl log should not exist after cancelled uninstall")
	}
}

func TestResolveBinDirUsesPrefix(t *testing.T) {
	dir, err := resolveBinDir("/opt/focus")
	if err != nil {
		t.Fatalf("resolveBinDir returned error: %v", err)
	}
	if want := filepath.Join("/opt/focus", "bin"); dir != want {
		t.Fatalf("resolveBinDir() = %q, want %q", dir, want)
	}
}

type uninstallFixture struct {
	prefix       string
	bindir       string
	libexecDir   string
	assetsDir    string
	servicePath  string
	systemctlLog string
}

func setupUninstallFixture(t *testing.T) uninstallFixture {
	t.Helper()

	tmp := t.TempDir()
	prefix := filepath.Join(tmp, "focus-prefix")
	bindir := filepath.Join(prefix, "bin")
	libexecDir := filepath.Join(prefix, "libexec", "focus")
	assetsDir := filepath.Join(prefix, "share", "focus", "assets")
	serviceDir := filepath.Join(tmp, "home", ".config", "systemd", "user")
	servicePath := filepath.Join(serviceDir, "focusd.service")
	fakeBin := filepath.Join(tmp, "fake-bin")
	systemctlLog := filepath.Join(tmp, "systemctl.log")

	for _, dir := range []string{bindir, serviceDir, assetsDir, libexecDir, fakeBin} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	files := map[string]struct {
		mode    os.FileMode
		content string
	}{
		filepath.Join(bindir, "focus"):            {mode: 0o755, content: "x"},
		filepath.Join(libexecDir, "focusd"):       {mode: 0o755, content: "x"},
		filepath.Join(libexecDir, "focus-events"): {mode: 0o755, content: "x"},
		servicePath: {mode: 0o644, content: "[Service]\n"},
		filepath.Join(assetsDir, "task-ending.mp3"): {mode: 0o644, content: "x"},
	}
	for path, file := range files {
		if err := os.WriteFile(path, []byte(file.content), file.mode); err != nil {
			t.Fatal(err)
		}
	}

	systemctlPath := filepath.Join(fakeBin, "systemctl")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> \"" + systemctlLog + "\"\n"
	if err := os.WriteFile(systemctlPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	return uninstallFixture{
		prefix:       prefix,
		bindir:       bindir,
		libexecDir:   libexecDir,
		assetsDir:    assetsDir,
		servicePath:  servicePath,
		systemctlLog: systemctlLog,
	}
}
