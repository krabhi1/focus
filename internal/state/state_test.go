package state

import (
	"focus/internal/sys"
	"strings"
	"testing"
	"time"
)

func newTestState() *DaemonState {
	return &DaemonState{
		taskHistory: []*Task{},
		actions:     sys.NoopActions{},
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

func TestBreakPlanForDuration(t *testing.T) {
	cases := []struct {
		name         string
		duration     time.Duration
		wantHasBreak bool
		wantStart    time.Duration
		wantBreakDur time.Duration
	}{
		{name: "short", duration: 15 * time.Minute, wantHasBreak: false},
		{name: "medium", duration: 30 * time.Minute, wantHasBreak: false},
		{name: "long", duration: 60 * time.Minute, wantHasBreak: true, wantStart: LongTaskBreakStartOffset, wantBreakDur: 5 * time.Minute},
		{name: "deep", duration: 90 * time.Minute, wantHasBreak: true, wantStart: DeepTaskBreakStartOffset, wantBreakDur: 10 * time.Minute},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, ok := breakPlanForDuration(tc.duration)
			if ok != tc.wantHasBreak {
				t.Fatalf("breakPlanForDuration(%s) hasBreak = %v, want %v", tc.duration, ok, tc.wantHasBreak)
			}
			if !tc.wantHasBreak {
				return
			}
			if plan.startOffset != tc.wantStart {
				t.Fatalf("startOffset = %s, want %s", plan.startOffset, tc.wantStart)
			}
			if plan.duration != tc.wantBreakDur {
				t.Fatalf("duration = %s, want %s", plan.duration, tc.wantBreakDur)
			}
		})
	}
}

func TestOnScreenUnlockedSchedulesRelockDuringBreak(t *testing.T) {
	s := newTestState()
	actions := &recordingActions{}
	s.actions = actions
	s.currentTask = &Task{
		ID:        1,
		Title:     "deep work",
		Duration:  90 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusBreak,
	}
	s.breakUntil = time.Now().Add(time.Minute)

	s.OnScreenUnlocked()

	if s.breakRelockTimer == nil {
		t.Fatal("expected relock timer to be scheduled")
	}
	if actions.notifyCount != 1 {
		t.Fatalf("notifyCount = %d, want 1", actions.notifyCount)
	}

	s.relockIfBreak(1)
	if actions.lockCount != 1 {
		t.Fatalf("lockCount = %d, want 1", actions.lockCount)
	}

	s.OnScreenLocked()
	if s.breakRelockTimer != nil {
		t.Fatal("expected relock timer to be cleared after lock event")
	}
}

type recordingActions struct {
	lockCount   int
	notifyCount int
}

func (r *recordingActions) LockScreen() {
	r.lockCount++
}

func (r *recordingActions) UnlockScreen() {}

func (r *recordingActions) PlaySound(string) {}

func (r *recordingActions) Notify(string, string) {
	r.notifyCount++
}
