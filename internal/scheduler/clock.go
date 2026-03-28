package scheduler

import "time"

type Clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) Timer
}

type Timer interface {
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) AfterFunc(d time.Duration, f func()) Timer {
	return &realTimer{timer: time.AfterFunc(d, f)}
}

type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) Stop() bool {
	if t == nil || t.timer == nil {
		return false
	}
	return t.timer.Stop()
}
