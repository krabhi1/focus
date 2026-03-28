package app

import (
	"time"

	"focus/internal/scheduler"
)

type runtimeClock interface {
	Now() time.Time
	Since(t time.Time) time.Duration
	Until(t time.Time) time.Duration
	AfterFunc(d time.Duration, f func()) runtimeTimer
	NewTimer(d time.Duration) runtimeTimer
}

type runtimeTimer interface {
	Stop() bool
	C() <-chan time.Time
}

type realRuntimeClock struct{}

type realRuntimeTimer struct {
	timer *time.Timer
}

func (realRuntimeClock) Now() time.Time {
	return time.Now()
}

func (realRuntimeClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

func (realRuntimeClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

func (realRuntimeClock) AfterFunc(d time.Duration, f func()) runtimeTimer {
	return &realRuntimeTimer{timer: time.AfterFunc(d, f)}
}

func (realRuntimeClock) NewTimer(d time.Duration) runtimeTimer {
	return &realRuntimeTimer{timer: time.NewTimer(d)}
}

func (t *realRuntimeTimer) Stop() bool {
	if t == nil || t.timer == nil {
		return false
	}
	return t.timer.Stop()
}

func (t *realRuntimeTimer) C() <-chan time.Time {
	if t == nil || t.timer == nil {
		return nil
	}
	return t.timer.C
}

type schedulerClockAdapter struct {
	clock runtimeClock
}

func (a schedulerClockAdapter) Now() time.Time {
	return a.clock.Now()
}

func (a schedulerClockAdapter) AfterFunc(d time.Duration, f func()) scheduler.Timer {
	timer := a.clock.AfterFunc(d, f)
	return schedulerTimerAdapter{timer: timer}
}

type schedulerTimerAdapter struct {
	timer runtimeTimer
}

func (t schedulerTimerAdapter) Stop() bool {
	if t.timer == nil {
		return false
	}
	return t.timer.Stop()
}
