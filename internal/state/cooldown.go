package state

import "time"

func (s *DaemonState) beginCooldownLocked(task *Task, now time.Time) {
	duration := s.cooldownDuration(task.Duration)
	s.cooldownUntil = now.Add(duration)
	if s.cooldownTimer != nil {
		s.cooldownTimer.Stop()
		s.cooldownTimer = nil
	}
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
	currentActions := s.actionsLocked()
	s.mu.Unlock()

	currentActions.PlaySound("assets/task-ending.mp3")
}

func cooldownDurationFor(duration time.Duration, cfg RuntimeConfig) time.Duration {
	switch {
	case duration >= 90*time.Minute:
		return cfg.CooldownDeep
	case duration >= 60*time.Minute:
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
