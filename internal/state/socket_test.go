package state

import (
	"path/filepath"
	"testing"
)

func TestResolveSocketPathUsesXDGRuntimeDir(t *testing.T) {
	got := resolveSocketPath("/tmp/runtime", 1000)
	want := filepath.Join("/tmp/runtime", "focus", "focus.sock")
	if got != want {
		t.Fatalf("resolveSocketPath() = %q, want %q", got, want)
	}
}

func TestResolveSocketPathUsesTmpFallback(t *testing.T) {
	got := resolveSocketPath("", 1001)
	want := filepath.Join("/tmp", "focus-1001.sock")
	if got != want {
		t.Fatalf("resolveSocketPath() = %q, want %q", got, want)
	}
}

func TestResolveSocketPathNegativeUIDFallback(t *testing.T) {
	got := resolveSocketPath("", -1)
	want := filepath.Join("/tmp", "focus.sock")
	if got != want {
		t.Fatalf("resolveSocketPath() = %q, want %q", got, want)
	}
}
