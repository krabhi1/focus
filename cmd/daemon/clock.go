package main

import "time"

type runtimeTimer interface {
	Stop() bool
}

type runtimeSleepTimer interface {
	Stop() bool
	C() <-chan time.Time
}

type runtimeClock interface {
	Now() time.Time
	Since(time.Time) time.Duration
	Until(time.Time) time.Duration
	AfterFunc(time.Duration, func()) runtimeTimer
	NewTimer(time.Duration) runtimeSleepTimer
}

type realRuntimeClock struct{}

func (realRuntimeClock) Now() time.Time {
	return time.Now()
}

func (realRuntimeClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

func (realRuntimeClock) Until(t time.Time) time.Duration {
	return time.Until(t)
}

func (realRuntimeClock) AfterFunc(d time.Duration, fn func()) runtimeTimer {
	return time.AfterFunc(d, fn)
}

func (realRuntimeClock) NewTimer(d time.Duration) runtimeSleepTimer {
	return &realSleepTimer{timer: time.NewTimer(d)}
}

type realSleepTimer struct {
	timer *time.Timer
}

func (t *realSleepTimer) Stop() bool {
	return t.timer.Stop()
}

func (t *realSleepTimer) C() <-chan time.Time {
	return t.timer.C
}
