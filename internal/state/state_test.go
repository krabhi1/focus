package state

import (
	"context"
	"focus/internal/sys"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestState(t *testing.T) *DaemonState {
	t.Helper()
	if err := SetRuntimeConfig(DefaultRuntimeConfig()); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}

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
		{name: "short task", duration: 15 * time.Minute, want: ShortCooldownDuration},
		{name: "medium task", duration: 30 * time.Minute, want: ShortCooldownDuration},
		{name: "long task", duration: 60 * time.Minute, want: LongCooldownDuration},
		{name: "deep task", duration: 90 * time.Minute, want: DeepCooldownDuration},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cooldownDurationFor(tc.duration, GetRuntimeConfig()); got != tc.want {
				t.Fatalf("cooldownDurationFor(%s) = %s, want %s", tc.duration, got, tc.want)
			}
		})
	}
}

func TestBeginCooldownLockedUsesTaskDuration(t *testing.T) {
	s := newTestState(t)
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	task := &Task{Duration: 90 * time.Minute}

	s.beginCooldownLocked(task, now)

	want := now.Add(DeepCooldownDuration)
	if !s.cooldownUntil.Equal(want) {
		t.Fatalf("cooldownUntil = %v, want %v", s.cooldownUntil, want)
	}
}

func TestNewTaskRejectsActiveCooldown(t *testing.T) {
	s := newTestState(t)
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
	s := newTestState(t)
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
	s := newTestState(t)
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
	s := newTestState(t)
	s.cooldownUntil = time.Now().Add(90 * time.Second)

	status := s.GetStatus()
	if !strings.Contains(status, "Cooldown active") {
		t.Fatalf("status = %q, want cooldown state", status)
	}
	if !strings.Contains(status, "Remaining:") {
		t.Fatalf("status = %q, want remaining time", status)
	}
}

func TestOnScreenUnlockedLocksDuringCooldown(t *testing.T) {
	s := newTestState(t)
	actions := &recordingActions{}
	s.actions = actions
	s.cooldownUntil = time.Now().Add(2 * time.Minute)

	s.OnScreenUnlocked()

	if actions.notifyCount != 1 {
		t.Fatalf("notifyCount = %d, want 1", actions.notifyCount)
	}
	if actions.lastNotifyTitle != "Cooldown Active" {
		t.Fatalf("last notify title = %q, want %q", actions.lastNotifyTitle, "Cooldown Active")
	}
	if actions.lockCount != 1 {
		t.Fatalf("lockCount = %d, want 1", actions.lockCount)
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
	s := newTestState(t)
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
	if actions.lastNotifyTitle != "Break Active" {
		t.Fatalf("last notify title = %q, want %q", actions.lastNotifyTitle, "Break Active")
	}
	if s.breakRelockUntil.IsZero() {
		t.Fatal("expected relock deadline to be set")
	}

	s.relockIfBreak(1)
	if actions.lockCount != 1 {
		t.Fatalf("lockCount = %d, want 1", actions.lockCount)
	}
	if !s.breakRelockUntil.IsZero() {
		t.Fatal("expected relock deadline to be cleared after relock")
	}

	s.OnScreenLocked()
	if s.breakRelockTimer != nil {
		t.Fatal("expected relock timer to be cleared after lock event")
	}
	if !s.breakRelockUntil.IsZero() {
		t.Fatal("expected relock deadline to be cleared after lock event")
	}
}

func TestNotifyBreakComingUsesWarningOffset(t *testing.T) {
	s := newTestState(t)
	actions := &recordingActions{}
	s.actions = actions
	s.currentTask = &Task{
		ID:        2,
		Title:     "long work",
		Duration:  60 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusActive,
	}

	s.notifyBreakComing(2)

	if actions.notifyCount != 1 {
		t.Fatalf("notifyCount = %d, want 1", actions.notifyCount)
	}
	if actions.lastNotifyTitle != "Break Reminder" {
		t.Fatalf("last notify title = %q, want %q", actions.lastNotifyTitle, "Break Reminder")
	}
	if !strings.Contains(actions.lastNotifyBody, "2m0s") {
		t.Fatalf("notify body = %q, want warning offset", actions.lastNotifyBody)
	}
}

func TestEndBreakClearsBreakState(t *testing.T) {
	s := newTestState(t)
	actions := &recordingActions{}
	s.actions = actions
	task := &Task{
		ID:        3,
		Title:     "deep work",
		Duration:  90 * time.Minute,
		StartTime: time.Now(),
		Status:    StatusBreak,
	}
	s.currentTask = task
	s.breakUntil = time.Now().Add(5 * time.Minute)
	s.breakRelockUntil = time.Now().Add(30 * time.Second)
	s.breakRelockTimer = time.AfterFunc(time.Hour, func() {})

	s.endBreak(3)

	if task.Status != StatusActive {
		t.Fatalf("task status = %q, want %q", task.Status, StatusActive)
	}
	if !s.breakUntil.IsZero() {
		t.Fatalf("breakUntil = %v, want zero", s.breakUntil)
	}
	if s.breakRelockTimer != nil {
		t.Fatal("breakRelockTimer should be nil")
	}
	if !s.breakRelockUntil.IsZero() {
		t.Fatalf("breakRelockUntil = %v, want zero", s.breakRelockUntil)
	}
	if actions.lastNotifyTitle != "Break Complete" {
		t.Fatalf("last notify title = %q, want %q", actions.lastNotifyTitle, "Break Complete")
	}
	if actions.unlockCount != 1 {
		t.Fatalf("unlockCount = %d, want 1", actions.unlockCount)
	}
	if actions.playSoundCount != 1 {
		t.Fatalf("playSoundCount = %d, want 1", actions.playSoundCount)
	}
}

func TestCooldownTimerPlaysSoundWhenCooldownEnds(t *testing.T) {
	s := newTestState(t)
	actions := &atomicRecordingActions{}
	s.actions = actions
	s.SetCooldownPolicyForTest(func(time.Duration) time.Duration {
		return 20 * time.Millisecond
	})

	task := &Task{Duration: 30 * time.Minute}
	s.beginCooldownLocked(task, time.Now())

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if actions.playSoundCount.Load() > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if actions.playSoundCount.Load() != 1 {
		t.Fatalf("playSoundCount = %d, want 1", actions.playSoundCount.Load())
	}
	if actions.unlockCount.Load() != 0 {
		t.Fatalf("unlockCount = %d, want 0", actions.unlockCount.Load())
	}
	if !s.cooldownUntil.IsZero() {
		t.Fatalf("cooldownUntil = %v, want zero", s.cooldownUntil)
	}
	if s.cooldownTimer != nil {
		t.Fatal("cooldownTimer should be nil after expiry")
	}
}

func TestGetStatusShowsBreakAndRelockCountdown(t *testing.T) {
	s := newTestState(t)
	s.currentTask = &Task{
		ID:        4,
		Title:     "deep work",
		Duration:  90 * time.Minute,
		StartTime: time.Now().Add(-10 * time.Minute),
		Status:    StatusBreak,
	}
	s.breakUntil = time.Now().Add(3 * time.Minute)
	s.breakRelockUntil = time.Now().Add(20 * time.Second)

	status := s.GetStatus()

	if !strings.Contains(status, "Status: break") {
		t.Fatalf("status = %q, want break status", status)
	}
	if !strings.Contains(status, "Break remaining:") {
		t.Fatalf("status = %q, want break remaining", status)
	}
	if !strings.Contains(status, "Re-lock in:") {
		t.Fatalf("status = %q, want relock countdown", status)
	}
}

func TestIdleMonitorLocksWhenCooldownActive(t *testing.T) {
	s := newTestState(t)
	actions := &atomicRecordingActions{}
	s.actions = actions
	s.cooldownUntil = time.Now().Add(150 * time.Millisecond)

	cfg := DefaultRuntimeConfig()
	cfg.IdlePollInterval = 10 * time.Millisecond
	cfg.IdleWarnAfter = 5 * time.Minute
	cfg.IdleLockAfter = 10 * time.Minute
	if err := SetRuntimeConfig(cfg); err != nil {
		t.Fatalf("SetRuntimeConfig failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.StartIdleMonitor(ctx)

	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if actions.lockCount.Load() > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("expected cooldown lock via idle monitor; lockCount=%d", actions.lockCount.Load())
}

type recordingActions struct {
	lockCount       int
	unlockCount     int
	playSoundCount  int
	notifyCount     int
	lastNotifyTitle string
	lastNotifyBody  string
}

func (r *recordingActions) LockScreen() {
	r.lockCount++
}

func (r *recordingActions) UnlockScreen() {
	r.unlockCount++
}

func (r *recordingActions) PlaySound(string) {
	r.playSoundCount++
}

func (r *recordingActions) Notify(title string, body string) {
	r.notifyCount++
	r.lastNotifyTitle = title
	r.lastNotifyBody = body
}

type atomicRecordingActions struct {
	lockCount      atomic.Int32
	unlockCount    atomic.Int32
	playSoundCount atomic.Int32
}

func (a *atomicRecordingActions) LockScreen() {
	a.lockCount.Add(1)
}

func (a *atomicRecordingActions) UnlockScreen() {
	a.unlockCount.Add(1)
}

func (a *atomicRecordingActions) PlaySound(string) {
	a.playSoundCount.Add(1)
}

func (a *atomicRecordingActions) Notify(string, string) {}
