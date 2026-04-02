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

func Start(ctx context.Context) (*Listener, error) {
	helperPath, err := resolveHelperPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, helperPath, "--format=binary")
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
	go scanBinaryStdout(stdout, eventCh, errCh)
	go logStderr(stderr)
	go waitForExit(cmd, errCh)
	return &Listener{cmd: cmd, Events: eventCh, Errors: errCh}, nil
}

func scanBinaryStdout(stdout io.ReadCloser, eventCh chan<- Event, errCh chan<- error) {
	defer close(eventCh)
	frame := make([]byte, wireSize)
	for {
		_, err := io.ReadFull(stdout, frame)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				return
			}
			errCh <- fmt.Errorf("read helper stdout: %w", err)
			return
		}
		event, err := ParseBinaryFrame(frame)
		if err != nil {
			errCh <- err
			continue
		}
		eventCh <- event
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
	if p := os.Getenv("FOCUS_LIBEXEC_DIR"); p != "" {
		candidate := filepath.Join(p, "focus-events")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		for _, candidate := range []string{
			filepath.Join(execDir, "focus-events"),
			filepath.Join(filepath.Dir(execDir), "libexec", "focus", "focus-events"),
		} {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, nil
			}
		}
	}
	for _, candidate := range []string{filepath.Join("dist", "focus-events"), filepath.Join("libexec", "focus", "focus-events"), "focus-events"} {
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("focus-events helper not found")
}
