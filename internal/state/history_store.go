package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const historyFileEnv = "FOCUS_HISTORY_FILE"

func DefaultHistoryPath() (string, error) {
	return historyFilePath()
}

func historyFilePath() (string, error) {
	if path := os.Getenv(historyFileEnv); path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".config", "focus", "history.jsonl"), nil
}

type historyRecord struct {
	Task
	CompletedAt time.Time `json:"completed_at"`
}

func appendCompletedTaskToLog(task *Task) error {
	if task == nil || task.Status != StatusCompleted {
		return nil
	}

	path, err := historyFilePath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create history dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open history file: %w", err)
	}
	defer f.Close()

	record := historyRecord{
		Task:        *task,
		CompletedAt: time.Now(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal history record: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write history record: %w", err)
	}
	return nil
}

func loadTodayHistoryFromLog() ([]Task, error) {
	path, err := historyFilePath()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("open history file: %w", err)
	}
	defer file.Close()

	todayStart := startOfToday(time.Now())
	todayEnd := todayStart.Add(24 * time.Hour)

	scanner := bufio.NewScanner(file)
	tasks := make([]Task, 0)

	for scanner.Scan() {
		var record historyRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			continue
		}
		if record.StartTime.Before(todayStart) || !record.StartTime.Before(todayEnd) {
			continue
		}
		record.Status = StatusCompleted
		tasks = append(tasks, record.Task)
	}
	if err := scanner.Err(); err != nil {
		return tasks, fmt.Errorf("scan history file: %w", err)
	}
	return tasks, nil
}

func startOfToday(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}
