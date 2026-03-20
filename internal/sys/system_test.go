package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveAssetPathUsesDirectRelativePathWhenPresent(t *testing.T) {
	originalWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	defer func() {
		_ = os.Chdir(originalWD)
	}()

	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}

	if err := os.MkdirAll("assets", 0o755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join("assets", "task-ending.mp3"), []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	got := resolveAssetPath("assets/task-ending.mp3")
	if got != "assets/task-ending.mp3" {
		t.Fatalf("resolveAssetPath() = %q, want direct relative path", got)
	}
}

func TestResolveAssetPathReturnsOriginalWhenMissing(t *testing.T) {
	input := "assets/missing.mp3"
	got := resolveAssetPath(input)
	if got != input {
		t.Fatalf("resolveAssetPath() = %q, want %q", got, input)
	}
}
