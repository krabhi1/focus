package state

import "time"

var cooldownStartDelay = 10 * time.Second

func SetCooldownStartDelayForTest(d time.Duration) func() {
	old := cooldownStartDelay
	cooldownStartDelay = d
	return func() {
		cooldownStartDelay = old
	}
}

func (s *DaemonState) scheduleCooldownStartLocked(task *Task, now time.Time) {
	duration := s.cooldownDuration(task.Duration)
	if s.cooldownStartTimer != nil {
		s.cooldownStartTimer.Stop()
		s.cooldownStartTimer = nil
	}
	s.cooldownStartUntil = now.Add(cooldownStartDelay)
	cooldownStartUntil := s.cooldownStartUntil
	s.cooldownStartTimer = time.AfterFunc(cooldownStartDelay, func() {
		s.onCooldownStartDelayEnded(cooldownStartUntil, duration)
	})
}

func (s *DaemonState) beginCooldownLocked(task *Task, now time.Time) {
	duration := s.cooldownDuration(task.Duration)
	s.beginCooldownLockedWithDuration(duration, now)
}

func (s *DaemonState) beginCooldownLockedWithDuration(duration time.Duration, now time.Time) {
	s.cooldownUntil = now.Add(duration)
	if s.cooldownTimer != nil {
		s.cooldownTimer.Stop()
		s.cooldownTimer = nil
	}
	s.actionsLocked().LockScreen()
	cooldownUntil := s.cooldownUntil
	s.cooldownTimer = time.AfterFunc(duration, func() {
		s.onCooldownEnded(cooldownUntil)
	})
}

func (s *DaemonState) cooldownRemainingLocked(now time.Time) time.Duration {
	if s.cooldownUntil.IsZero() {
		return 0
	}
	if !now.Before(s.cooldownUntil) {
		return 0
	}
	return s.cooldownUntil.Sub(now)
}

func (s *DaemonState) onCooldownEnded(cooldownUntil time.Time) {
	s.mu.Lock()
	if !s.cooldownUntil.Equal(cooldownUntil) {
		s.mu.Unlock()
		return
	}
	if time.Now().Before(s.cooldownUntil) {
		s.mu.Unlock()
		return
	}
	s.cooldownUntil = time.Time{}
	if s.cooldownTimer != nil {
		s.cooldownTimer.Stop()
		s.cooldownTimer = nil
	}
	s.resumeIdleTrackingIfNeededLocked(time.Now())
	s.mu.Unlock()

	s.startCompletionAlert()
}

func (s *DaemonState) pendingCooldownRemainingLocked(now time.Time) time.Duration {
	if s.cooldownStartUntil.IsZero() {
		return 0
	}
	if !now.Before(s.cooldownStartUntil) {
		return 0
	}
	return s.cooldownStartUntil.Sub(now)
}

func (s *DaemonState) onCooldownStartDelayEnded(cooldownStartUntil time.Time, duration time.Duration) {
	s.mu.Lock()
	if !s.cooldownStartUntil.Equal(cooldownStartUntil) {
		s.mu.Unlock()
		return
	}
	if time.Now().Before(s.cooldownStartUntil) {
		s.mu.Unlock()
		return
	}
	s.cooldownStartUntil = time.Time{}
	if s.cooldownStartTimer != nil {
		s.cooldownStartTimer.Stop()
		s.cooldownStartTimer = nil
	}
	s.beginCooldownLockedWithDuration(duration, time.Now())
	s.mu.Unlock()
}

func cooldownDurationFor(duration time.Duration, cfg RuntimeConfig) time.Duration {
	switch {
	case duration >= cfg.TaskDeep:
		return cfg.CooldownDeep
	case duration >= cfg.TaskLong:
		return cfg.CooldownLong
	default:
		return cfg.CooldownShort
	}
}

func (s *DaemonState) cooldownDuration(duration time.Duration) time.Duration {
	if s.cooldownPolicy != nil {
		return s.cooldownPolicy(duration)
	}
	return cooldownDurationFor(duration, GetRuntimeConfig())
}
