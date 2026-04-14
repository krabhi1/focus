package main

import (
	"encoding/gob"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"focus/internal/protocol"
	"focus/internal/storage"
)

func TestRunConfigUpdatesFileAndReloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"idle":{"warn_after":"1m","lock_after":"2m"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Setenv("FOCUS_CONFIG", path)

	reloaded := false
	if err := runConfig([]string{"idle.lock_after", "3m"}, func() error {
		reloaded = true
		return nil
	}); err != nil {
		t.Fatalf("runConfig returned error: %v", err)
	}
	if !reloaded {
		t.Fatal("reload hook was not called")
	}

	cfg, exists, err := storage.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !exists {
		t.Fatal("exists = false, want true")
	}
	if cfg.Idle.WarnAfter != "1m" {
		t.Fatalf("idle.warn_after = %q, want 1m", cfg.Idle.WarnAfter)
	}
	if cfg.Idle.LockAfter != "3m" {
		t.Fatalf("idle.lock_after = %q, want 3m", cfg.Idle.LockAfter)
	}
}

func TestRunConfigReloadFailureStillSucceeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Setenv("FOCUS_CONFIG", path)

	if err := runConfig([]string{"relock_delay", "5s"}, func() error {
		return os.ErrNotExist
	}); err != nil {
		t.Fatalf("runConfig returned error: %v", err)
	}

	cfg, exists, err := storage.Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if !exists {
		t.Fatal("exists = false, want true")
	}
	if cfg.RelockDelay != "5s" {
		t.Fatalf("relock_delay = %q, want 5s", cfg.RelockDelay)
	}
}

func TestRunConfigReadsValueAndDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"idle":{"warn_after":"1m","lock_after":"3m"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Setenv("FOCUS_CONFIG", path)

	var stdout strings.Builder
	withStdout(&stdout, func() {
		if err := runConfig([]string{"idle.lock_after"}, nil); err != nil {
			t.Fatalf("runConfig returned error: %v", err)
		}
	})

	got := stdout.String()
	if !strings.Contains(got, "idle.lock_after = 3m0s (default: 2m0s)") {
		t.Fatalf("read output = %q, want current and default values", got)
	}
}

func TestRunConfigReadsTaskEndActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	body := `{"task":{"long_end_action":"sleep","deep_end_action":"lock"}}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	t.Setenv("FOCUS_CONFIG", path)

	var stdout strings.Builder
	withStdout(&stdout, func() {
		if err := runConfig([]string{"task.long_end_action"}, nil); err != nil {
			t.Fatalf("runConfig returned error: %v", err)
		}
		if err := runConfig([]string{"task.deep_end_action"}, nil); err != nil {
			t.Fatalf("runConfig returned error: %v", err)
		}
	})

	got := stdout.String()
	if !strings.Contains(got, "task.long_end_action = sleep (default: lock)") {
		t.Fatalf("read output = %q, want task.long_end_action current and default values", got)
	}
	if !strings.Contains(got, "task.deep_end_action = lock (default: sleep)") {
		t.Fatalf("read output = %q, want task.deep_end_action current and default values", got)
	}
}

func TestRunConfigHelpPrintsUsage(t *testing.T) {
	var stdout strings.Builder
	withStdout(&stdout, func() {
		if err := runConfig([]string{"--help"}, nil); err != nil {
			t.Fatalf("runConfig returned error: %v", err)
		}
	})

	got := stdout.String()
	for _, want := range []string{
		"focus config <key> [<value>]",
		"task.short (default: 15m0s)",
		"task.long_end_action (default: lock)",
		"task.deep_end_action (default: sleep)",
		"relock_delay (default: 10s)",
		"cooldown_start_delay (default: 2m0s)",
		"focus config idle.lock_after 3m",
		"Use one argument to read a value.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output = %q, want %q", got, want)
		}
	}
}

func TestReloadDaemonSendsReloadRequest(t *testing.T) {
	sockPath := filepath.Join(t.TempDir(), "focus.sock")
	t.Setenv("FOCUS_SOCKET_PATH", sockPath)

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer listener.Close()

	got := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		var req protocol.Request
		if err := gob.NewDecoder(conn).Decode(&req); err != nil {
			return
		}
		got <- req.Command
		if err := gob.NewEncoder(conn).Encode(protocol.Response{
			Type: "response",
			Success: &protocol.SuccessResponse{
				Message: "Config reloaded.",
			},
		}); err != nil {
			return
		}
	}()

	if err := reloadDaemon(); err != nil {
		t.Fatalf("reloadDaemon returned error: %v", err)
	}
	<-done

	select {
	case cmd := <-got:
		if cmd != "reload" {
			t.Fatalf("command = %q, want reload", cmd)
		}
	default:
		t.Fatal("did not receive reload request")
	}
}

func withStdout(dst io.Writer, fn func()) {
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.Copy(dst, r)
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = old
}
