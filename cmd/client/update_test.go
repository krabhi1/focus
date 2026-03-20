package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseChecksumEntry(t *testing.T) {
	entry, err := parseChecksumEntry("abc123  ./focus_v0.1.2_linux_amd64.tar.gz")
	if err != nil {
		t.Fatalf("parseChecksumEntry returned error: %v", err)
	}
	if entry.Hash != "abc123" {
		t.Fatalf("hash = %q, want abc123", entry.Hash)
	}
	if entry.Name != "./focus_v0.1.2_linux_amd64.tar.gz" {
		t.Fatalf("name = %q, want ./focus_v0.1.2_linux_amd64.tar.gz", entry.Name)
	}
}

func TestVerifyReleaseChecksumAcceptsDotSlashEntry(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "focus_v0.1.2_linux_amd64.tar.gz")
	checksumsPath := filepath.Join(dir, "checksums_v0.1.2.txt")

	if err := os.WriteFile(archivePath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	sumBytes := sha256.Sum256([]byte("payload"))
	sum := hex.EncodeToString(sumBytes[:])
	checksumLine := sum + "  ./focus_v0.1.2_linux_amd64.tar.gz\n"
	if err := os.WriteFile(checksumsPath, []byte(checksumLine), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := verifyReleaseChecksum(checksumsPath, archivePath, "focus_v0.1.2_linux_amd64.tar.gz"); err != nil {
		t.Fatalf("verifyReleaseChecksum returned error: %v", err)
	}
}

func TestAtomicReplaceFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dest := filepath.Join(dir, "dest")

	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := atomicReplaceFile(src, dest, 0o755); err != nil {
		t.Fatalf("atomicReplaceFile returned error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "new" {
		t.Fatalf("dest contents = %q, want new", string(data))
	}
}
