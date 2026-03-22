package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendCompletedTaskToLogWritesRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")
	t.Setenv(historyFileEnv, path)

	task := &Task{
		ID:        1,
		Title:     "write test",
		Duration:  15 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusCompleted,
	}

	if err := appendCompletedTaskToLog(task); err != nil {
		t.Fatalf("appendCompletedTaskToLog failed: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read history file: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var record historyRecord
	if err := json.Unmarshal([]byte(lines[0]), &record); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}

	if record.Title != task.Title {
		t.Fatalf("expected title %q, got %q", task.Title, record.Title)
	}
}

func TestLoadTodayHistoryFromLogFiltersByDate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(historyFileEnv, filepath.Join(dir, "history.jsonl"))
	now := time.Now()

	todayTask := &Task{
		ID:        1,
		Title:     "today task",
		Duration:  30 * time.Minute,
		StartTime: now,
		Status:    StatusCompleted,
	}
	yesterdayTask := &Task{
		ID:        2,
		Title:     "yesterday task",
		Duration:  30 * time.Minute,
		StartTime: now.Add(-24 * time.Hour),
		Status:    StatusCompleted,
	}

	if err := appendCompletedTaskToLog(todayTask); err != nil {
		t.Fatalf("append today task: %v", err)
	}
	if err := appendCompletedTaskToLog(yesterdayTask); err != nil {
		t.Fatalf("append yesterday task: %v", err)
	}

	tasks, err := loadTodayHistoryFromLog()
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

func TestAppendCompletedTaskToLogUsesDefaultPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(historyFileEnv, "")

	task := &Task{
		ID:        1,
		Title:     "disabled",
		Duration:  15 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusCompleted,
	}

	if err := appendCompletedTaskToLog(task); err != nil {
		t.Fatalf("appendCompletedTaskToLog failed: %v", err)
	}

	path := filepath.Join(home, ".config", "focus", "history.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected default history file to be created, stat err=%v", err)
	}
}
