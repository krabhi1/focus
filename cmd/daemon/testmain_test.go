package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "focus-daemon-tests-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create test temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tempDir)

	testHistoryPath := filepath.Join(tempDir, "history.jsonl")
	_ = os.Setenv("FOCUS_HISTORY_FILE", testHistoryPath)

	code := m.Run()
	os.Exit(code)
}
