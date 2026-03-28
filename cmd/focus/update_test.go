package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateNoopsWhenVersionAlreadyInstalled(t *testing.T) {
	oldVersion := version
	version = "v0.1.2"
	t.Cleanup(func() {
		version = oldVersion
	})

	dir := t.TempDir()
	prefix := filepath.Join(dir, "prefix")

	if err := Update("v0.1.2", prefix, true); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

func TestUpdateNoopsWhenLatestMatchesInstalledVersion(t *testing.T) {
	oldVersion := version
	oldResolveLatest := resolveLatestReleaseFn
	version = "v0.1.2"
	resolveLatestReleaseFn = func(string) (string, error) {
		return "v0.1.2", nil
	}
	t.Cleanup(func() {
		version = oldVersion
		resolveLatestReleaseFn = oldResolveLatest
	})

	dir := t.TempDir()
	prefix := filepath.Join(dir, "prefix")

	if err := Update("", prefix, true); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
}

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

func TestCopyToTempInDir(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")

	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	tmp, err := copyToTempInDir(src, dir, ".dest.new.", 0o755)
	if err != nil {
		t.Fatalf("copyToTempInDir returned error: %v", err)
	}
	defer os.Remove(tmp)

	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "new" {
		t.Fatalf("temp contents = %q, want new", string(data))
	}
}

func TestSafeArchiveTargetRejectsEscape(t *testing.T) {
	dest := t.TempDir()

	_, err := safeArchiveTarget(dest, "../../etc/passwd")
	if err == nil {
		t.Fatal("expected escape path to be rejected")
	}
}

func TestExtractReleaseArchiveRejectsEscapeEntry(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "bad.tar.gz")

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name: "../../escape",
		Mode: 0o644,
		Size: int64(len("x")),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(tw, "x"); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	err := extractReleaseArchive(archivePath, filepath.Join(dir, "out"))
	if err == nil {
		t.Fatal("expected extraction to fail for escape entry")
	}
}
