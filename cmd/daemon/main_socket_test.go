package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSocketPathAvailableMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "focus.sock")
	if err := ensureSocketPathAvailable(path); err != nil {
		t.Fatalf("ensureSocketPathAvailable returned error: %v", err)
	}
}

func TestEnsureSocketPathAvailableRemovesStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "focus.sock")

	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}

	if err := ensureSocketPathAvailable(path); err != nil {
		t.Fatalf("ensureSocketPathAvailable returned error: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("socket path still exists after cleanup: %v", err)
	}
}

func TestEnsureSocketPathAvailableRejectsRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "focus.sock")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if err := ensureSocketPathAvailable(path); err == nil {
		t.Fatal("expected error for non-socket path")
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("regular file should remain in place: %v", err)
	}
}
