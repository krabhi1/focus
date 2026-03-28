package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultRepo = "krabhi1/focus"

type releaseInfo struct {
	TagName string `json:"tag_name"`
}

type checksumEntry struct {
	Hash string
	Name string
}

var resolveLatestReleaseFn = resolveLatestRelease

func Update(versionArg, prefix string, yes bool) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("release updates are currently available only for linux/amd64")
	}

	bindir, err := resolveBinDir(prefix)
	if err != nil {
		return err
	}

	tag := versionArg
	if tag == "" || tag == "latest" {
		tag, err = resolveLatestReleaseFn(releaseRepo())
		if err != nil {
			return err
		}
	}
	if currentVersion := strings.TrimSpace(version); currentVersion != "" && currentVersion == tag {
		fmt.Printf("%s (%s).\n", colorSuccess("Focus is already up to date"), colorInfo(tag))
		return nil
	}

	assetName, err := releaseAssetName(tag)
	if err != nil {
		return err
	}
	checksumName := fmt.Sprintf("checksums_%s.txt", tag)

	if !yes {
		fmt.Printf("%s %s? [y/N]: ", colorPrompt("Update focus to"), colorInfo(tag))
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read confirmation: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			return fmt.Errorf("update cancelled")
		}
	}

	tmpDir, err := os.MkdirTemp("", "focus-update-*")
	if err != nil {
		return fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, assetName)
	checksumsPath := filepath.Join(tmpDir, checksumName)

	if err := downloadReleaseFile(releaseRepo(), tag, assetName, archivePath); err != nil {
		return err
	}
	if err := downloadReleaseFile(releaseRepo(), tag, checksumName, checksumsPath); err != nil {
		return err
	}

	if err := verifyReleaseChecksum(checksumsPath, archivePath, assetName); err != nil {
		return err
	}

	extractDir := filepath.Join(tmpDir, "extract")
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		return fmt.Errorf("create extract dir: %w", err)
	}
	if err := extractReleaseArchive(archivePath, extractDir); err != nil {
		return err
	}

	stageDir := filepath.Join(extractDir, fmt.Sprintf("focus_%s_%s_%s", tag, runtime.GOOS, runtime.GOARCH))
	if err := installReleaseBinaries(stageDir, bindir); err != nil {
		return err
	}

	if err := updateServiceIfPresent(bindir); err != nil {
		return err
	}

	return nil
}

func resolveLatestRelease(repo string) (string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "focus-updater")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch latest release: unexpected status %s", resp.Status)
	}

	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decode latest release: %w", err)
	}
	if info.TagName == "" {
		return "", fmt.Errorf("latest release tag is empty")
	}
	return info.TagName, nil
}

func releaseAssetName(tag string) (string, error) {
	goos := runtime.GOOS
	goarch, err := releaseArch(runtime.GOARCH)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("focus_%s_%s_%s.tar.gz", tag, goos, goarch), nil
}

func releaseArch(arch string) (string, error) {
	switch arch {
	case "amd64":
		return "amd64", nil
	default:
		return "", fmt.Errorf("unsupported architecture: %s (linux/amd64 only)", arch)
	}
}

func releaseRepo() string {
	if repo := os.Getenv("FOCUS_REPO"); repo != "" {
		return repo
	}
	return defaultRepo
}

func downloadReleaseFile(repo, tag, name, dest string) error {
	client := &http.Client{Timeout: 60 * time.Second}
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, tag, name)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "focus-updater")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download %s: %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: unexpected status %s", name, resp.Status)
	}

	file, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create %s: %w", dest, err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("save %s: %w", name, err)
	}
	return nil
}

func verifyReleaseChecksum(checksumsPath, archivePath, archiveName string) error {
	checksumFile, err := os.Open(checksumsPath)
	if err != nil {
		return fmt.Errorf("open checksums: %w", err)
	}
	defer checksumFile.Close()

	sum, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(checksumFile)
	for scanner.Scan() {
		entry, err := parseChecksumEntry(scanner.Text())
		if err != nil {
			continue
		}
		if entry.Name == archiveName || entry.Name == "./"+archiveName {
			if entry.Hash != sum {
				return fmt.Errorf("checksum mismatch for %s", archiveName)
			}
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read checksums: %w", err)
	}
	return fmt.Errorf("checksum entry for %s not found", archiveName)
}

func parseChecksumEntry(line string) (checksumEntry, error) {
	fields := strings.Fields(line)
	if len(fields) != 2 {
		return checksumEntry{}, fmt.Errorf("invalid checksum line")
	}
	return checksumEntry{Hash: fields[0], Name: fields[1]}, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("hash archive: %w", err)
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func extractReleaseArchive(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("open gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}

		target, err := safeArchiveTarget(destDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("create dir %s: %w", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create parent dir: %w", err)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("create file %s: %w", target, err)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return fmt.Errorf("write file %s: %w", target, err)
			}
			if err := out.Close(); err != nil {
				return fmt.Errorf("close file %s: %w", target, err)
			}
		case tar.TypeSymlink, tar.TypeLink:
			return fmt.Errorf("unsupported archive entry type for %s", header.Name)
		}
	}
}

func installReleaseBinaries(stageDir, bindir string) error {
	files := []string{"focus", "focusd", "focus-events"}
	if err := os.MkdirAll(bindir, 0o755); err != nil {
		return fmt.Errorf("create binary dir: %w", err)
	}

	staged := make(map[string]string, len(files))
	for _, name := range files {
		src := filepath.Join(stageDir, name)
		if _, err := os.Stat(src); err != nil {
			return fmt.Errorf("missing release binary %s", name)
		}

		dest := filepath.Join(bindir, name)
		tmpDest, err := copyToTempInDir(src, filepath.Dir(dest), "."+filepath.Base(dest)+".new.", 0o755)
		if err != nil {
			return err
		}
		staged[name] = tmpDest
	}

	replaced := make([]string, 0, len(files))
	backups := map[string]string{}

	rollback := func() {
		for _, name := range replaced {
			dest := filepath.Join(bindir, name)
			if backup, ok := backups[name]; ok {
				_ = os.Remove(dest)
				_ = os.Rename(backup, dest)
			}
		}
		for _, path := range backups {
			_ = os.Remove(path)
		}
		for _, path := range staged {
			_ = os.Remove(path)
		}
	}

	for _, name := range files {
		dest := filepath.Join(bindir, name)
		if _, err := os.Stat(dest); err == nil {
			backup, err := os.CreateTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".bak.")
			if err != nil {
				rollback()
				return fmt.Errorf("create backup placeholder for %s: %w", dest, err)
			}
			backupPath := backup.Name()
			if err := backup.Close(); err != nil {
				rollback()
				return fmt.Errorf("close backup placeholder for %s: %w", dest, err)
			}
			_ = os.Remove(backupPath)
			if err := os.Rename(dest, backupPath); err != nil {
				rollback()
				return fmt.Errorf("backup %s: %w", dest, err)
			}
			backups[name] = backupPath
		}

		if err := os.Rename(staged[name], dest); err != nil {
			rollback()
			return fmt.Errorf("replace %s: %w", dest, err)
		}
		replaced = append(replaced, name)
		delete(staged, name)
	}

	for _, backup := range backups {
		_ = os.Remove(backup)
	}
	for _, path := range staged {
		_ = os.Remove(path)
	}

	return nil
}

func copyToTempInDir(src, dir, pattern string, mode os.FileMode) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("copy %s: %w", src, err)
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("chmod temp %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("close temp %s: %w", tmpPath, err)
	}

	return tmpPath, nil
}

func safeArchiveTarget(destDir, entryName string) (string, error) {
	clean := filepath.Clean(entryName)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("invalid archive entry name %q", entryName)
	}
	if filepath.IsAbs(clean) {
		return "", fmt.Errorf("absolute archive entry path not allowed: %q", entryName)
	}

	target := filepath.Join(destDir, clean)
	rel, err := filepath.Rel(destDir, target)
	if err != nil {
		return "", fmt.Errorf("resolve archive path for %q: %w", entryName, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("archive entry escapes destination: %q", entryName)
	}

	return target, nil
}

func runSystemctl(args ...string) error {
	cmd := exec.Command("systemctl", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(output))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("systemctl %s failed: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func updateServiceIfPresent(bindir string) error {
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

	if err := os.MkdirAll(filepath.Dir(servicePath), 0o755); err != nil {
		return fmt.Errorf("create service dir: %w", err)
	}
	if err := os.WriteFile(servicePath, []byte(renderUserService(bindir)), 0o644); err != nil {
		return fmt.Errorf("write service file: %w", err)
	}

	if runtime.GOOS != "linux" {
		return nil
	}

	active := exec.Command("systemctl", "--user", "is-active", "--quiet", "focusd.service").Run() == nil
	if active {
		if err := runSystemctl("--user", "stop", "focusd.service"); err != nil {
			return err
		}
	}
	if err := runSystemctl("--user", "daemon-reload"); err != nil {
		return err
	}
	if active {
		if err := runSystemctl("--user", "start", "focusd.service"); err != nil {
			return err
		}
	}
	return nil
}
