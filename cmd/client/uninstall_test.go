package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUninstallRemovesBinariesAndService(t *testing.T) {
	tmp := t.TempDir()
	prefix := filepath.Join(tmp, "focus-prefix")
	bindir := filepath.Join(prefix, "bin")
	serviceDir := filepath.Join(tmp, "home", ".config", "systemd", "user")
	servicePath := filepath.Join(serviceDir, "focusd.service")
	fakeBin := filepath.Join(tmp, "fake-bin")
	systemctlLog := filepath.Join(tmp, "systemctl.log")

	if err := os.MkdirAll(bindir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindir, "focus"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindir, "focusd"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bindir, "focus-events"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(servicePath, []byte("[Service]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	systemctlPath := filepath.Join(fakeBin, "systemctl")
	script := "#!/usr/bin/env bash\nprintf '%s\\n' \"$*\" >> \"" + systemctlLog + "\"\n"
	if err := os.WriteFile(systemctlPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := Uninstall(prefix); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	for _, name := range []string{"focus", "focusd", "focus-events"} {
		if _, err := os.Stat(filepath.Join(bindir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s still exists after uninstall", name)
		}
	}
	if _, err := os.Stat(servicePath); !os.IsNotExist(err) {
		t.Fatalf("service file still exists after uninstall")
	}

	logBytes, err := os.ReadFile(systemctlLog)
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

func TestResolveBinDirUsesPrefix(t *testing.T) {
	dir, err := resolveBinDir("/opt/focus")
	if err != nil {
		t.Fatalf("resolveBinDir returned error: %v", err)
	}
	if want := filepath.Join("/opt/focus", "bin"); dir != want {
		t.Fatalf("resolveBinDir() = %q, want %q", dir, want)
	}
}
