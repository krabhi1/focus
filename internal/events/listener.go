package events

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

type Listener struct {
	cmd    *exec.Cmd
	Events <-chan Event
	Errors <-chan error
}

func Start(ctx context.Context, idleThresholdSeconds, idlePollSeconds int) (*Listener, error) {
	helperPath, err := resolveHelperPath()
	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(
		ctx,
		helperPath,
		fmt.Sprintf("%d", idleThresholdSeconds),
		fmt.Sprintf("%d", idlePollSeconds),
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open helper stdout: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("open helper stderr: %w", err)
	}

	eventCh := make(chan Event)
	errCh := make(chan error, 2)

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start helper: %w", err)
	}

	go scanStdout(stdout, eventCh, errCh)
	go logStderr(stderr)
	go waitForExit(cmd, errCh)

	return &Listener{
		cmd:    cmd,
		Events: eventCh,
		Errors: errCh,
	}, nil
}

func scanStdout(stdout io.ReadCloser, eventCh chan<- Event, errCh chan<- error) {
	defer close(eventCh)

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		event, err := Parse(line)
		if err != nil {
			errCh <- err
			continue
		}
		eventCh <- event
	}

	if err := scanner.Err(); err != nil {
		errCh <- fmt.Errorf("read helper stdout: %w", err)
	}
}

func logStderr(stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		log.Printf("focus-events stderr: %s", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		log.Printf("focus-events stderr read error: %v", err)
	}
}

func waitForExit(cmd *exec.Cmd, errCh chan<- error) {
	if err := cmd.Wait(); err != nil {
		errCh <- fmt.Errorf("focus-events exited: %w", err)
	}
}

func resolveHelperPath() (string, error) {
	execPath, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(execPath), "focus-events")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	candidates := []string{
		filepath.Join("dist", "focus-events"),
		"focus-events",
	}

	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("focus-events helper not found")
}
