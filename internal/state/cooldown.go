package state

import "time"

func (s *DaemonState) beginCooldownLocked(task *Task, now time.Time) {
	s.cooldownUntil = now.Add(s.cooldownDuration(task.Duration))
}

func (s *DaemonState) cooldownRemainingLocked(now time.Time) time.Duration {
	if s.cooldownUntil.IsZero() {
		return 0
	}
	if !now.Before(s.cooldownUntil) {
		s.cooldownUntil = time.Time{}
		return 0
	}
	return s.cooldownUntil.Sub(now)
}

func cooldownDurationFor(duration time.Duration) time.Duration {
	switch {
	case duration >= 90*time.Minute:
		return DeepBreakDuration
	case duration >= 60*time.Minute:
		return LongBreakDuration
	default:
		return BreakDuration
	}
}

func (s *DaemonState) cooldownDuration(duration time.Duration) time.Duration {
	if s.cooldownPolicy != nil {
		return s.cooldownPolicy(duration)
	}
	return cooldownDurationFor(duration)
}
