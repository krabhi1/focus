package storage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"focus/internal/domain"
)

func TestAppendCompletedTaskWritesRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	t.Setenv(historyFileEnv, path)

	task := domain.Task{
		ID:        1,
		Title:     "write test",
		Duration:  15 * time.Minute,
		StartTime: time.Now(),
	}

	if err := AppendCompletedTask(task); err != nil {
		t.Fatalf("AppendCompletedTask failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var record struct {
		domain.Task
		CompletedAt time.Time `json:"completed_at"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	if record.Title != task.Title {
		t.Fatalf("expected title %q, got %q", task.Title, record.Title)
	}
}

func TestLoadTodayHistoryFiltersByDate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(historyFileEnv, filepath.Join(dir, "history.jsonl"))
	now := time.Now()

	todayTask := domain.Task{
		ID:        1,
		Title:     "today task",
		Duration:  30 * time.Minute,
		StartTime: now,
	}
	yesterdayTask := domain.Task{
		ID:        2,
		Title:     "yesterday task",
		Duration:  30 * time.Minute,
		StartTime: now.Add(-24 * time.Hour),
	}

	if err := AppendCompletedTask(todayTask); err != nil {
		t.Fatalf("append today task: %v", err)
	}
	if err := AppendCompletedTask(yesterdayTask); err != nil {
		t.Fatalf("append yesterday task: %v", err)
	}

	tasks, err := LoadTodayHistory()
	if err != nil {
		t.Fatalf("load today history: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 today task, got %d", len(tasks))
	}
	if tasks[0].Title != todayTask.Title {
		t.Fatalf("expected %q, got %q", todayTask.Title, tasks[0].Title)
	}
}

func TestDefaultHistoryPathUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(historyFileEnv, "")

	got, err := DefaultHistoryPath()
	if err != nil {
		t.Fatalf("DefaultHistoryPath returned error: %v", err)
	}
	want := filepath.Join(home, ".config", "focus", "history.jsonl")
	if got != want {
		t.Fatalf("DefaultHistoryPath() = %q, want %q", got, want)
	}
}
