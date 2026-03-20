package state

import "time"

var cooldownDurationPolicy = cooldownDurationFor

func (s *DaemonState) beginCooldownLocked(task *Task, now time.Time) {
	s.cooldownUntil = now.Add(cooldownDurationPolicy(task.Duration))
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

func SetCooldownDurationPolicyForTest(policy func(time.Duration) time.Duration) func() {
	oldPolicy := cooldownDurationPolicy
	if policy == nil {
		cooldownDurationPolicy = cooldownDurationFor
	} else {
		cooldownDurationPolicy = policy
	}

	return func() {
		cooldownDurationPolicy = oldPolicy
	}
}
