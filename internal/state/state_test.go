package state

import (
	"strings"
	"testing"
	"time"
)

func newTestState() *DaemonState {
	return &DaemonState{
		taskHistory: []*Task{},
	}
}

func TestCooldownDurationFor(t *testing.T) {
	cases := []struct {
		name     string
		duration time.Duration
		want     time.Duration
	}{
		{name: "short task", duration: 15 * time.Minute, want: BreakDuration},
		{name: "medium task", duration: 30 * time.Minute, want: BreakDuration},
		{name: "long task", duration: 60 * time.Minute, want: LongBreakDuration},
		{name: "deep task", duration: 90 * time.Minute, want: DeepBreakDuration},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cooldownDurationFor(tc.duration); got != tc.want {
				t.Fatalf("cooldownDurationFor(%s) = %s, want %s", tc.duration, got, tc.want)
			}
		})
	}
}

func TestBeginCooldownLockedUsesTaskDuration(t *testing.T) {
	s := newTestState()
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	task := &Task{Duration: 90 * time.Minute}

	s.beginCooldownLocked(task, now)

	want := now.Add(DeepBreakDuration)
	if !s.cooldownUntil.Equal(want) {
		t.Fatalf("cooldownUntil = %v, want %v", s.cooldownUntil, want)
	}
}

func TestNewTaskRejectsActiveCooldown(t *testing.T) {
	s := newTestState()
	s.cooldownUntil = time.Now().Add(10 * time.Minute)

	_, err := s.NewTask("next task", 30*time.Minute)
	if err == nil {
		t.Fatal("expected error when cooldown is active")
	}
	if !strings.Contains(err.Error(), "cooldown active") {
		t.Fatalf("error = %q, want cooldown message", err.Error())
	}
}

func TestNewTaskAllowsExpiredCooldown(t *testing.T) {
	s := newTestState()
	s.cooldownUntil = time.Now().Add(-time.Minute)

	task, err := s.NewTask("next task", 30*time.Minute)
	if err != nil {
		t.Fatalf("NewTask returned error: %v", err)
	}
	if task == nil {
		t.Fatal("expected task to be created")
	}
	if s.currentTask != task {
		t.Fatal("expected task to become current task")
	}
}

func TestCancelCurrentTaskDoesNotStartCooldown(t *testing.T) {
	s := newTestState()
	task := &Task{
		Title:     "cancel me",
		Duration:  30 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusActive,
	}
	s.currentTask = task

	cancelled, err := s.CancelCurrentTask()
	if err != nil {
		t.Fatalf("CancelCurrentTask returned error: %v", err)
	}
	if cancelled.Status != StatusCancelled {
		t.Fatalf("Status = %q, want %q", cancelled.Status, StatusCancelled)
	}
	if !s.cooldownUntil.IsZero() {
		t.Fatalf("cooldownUntil = %v, want zero value", s.cooldownUntil)
	}
}

func TestGetStatusShowsCooldown(t *testing.T) {
	s := newTestState()
	s.cooldownUntil = time.Now().Add(90 * time.Second)

	status := s.GetStatus()
	if !strings.Contains(status, "Cooldown active") {
		t.Fatalf("status = %q, want cooldown state", status)
	}
	if !strings.Contains(status, "Remaining:") {
		t.Fatalf("status = %q, want remaining time", status)
	}
}
