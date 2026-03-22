package state

import "time"

func (s *DaemonState) OnIdleEntered() {
	s.mu.Lock()
	now := time.Now()
	s.idleActive = true

	if s.currentTask != nil || s.isSystemLocked {
		s.stopIdleTimersLocked()
		s.idleSince = time.Time{}
		s.notified = false
		s.mu.Unlock()
		return
	}

	if remaining := s.cooldownRemainingLocked(now); remaining > 0 {
		s.stopIdleTimersLocked()
		s.idleSince = time.Time{}
		s.notified = false
		s.mu.Unlock()
		actions := s.actionsLocked()
		actions.LockScreen()
		return
	}

	s.armIdleTimersLocked(now)
	s.mu.Unlock()
}

func (s *DaemonState) OnIdleExited() {
	s.mu.Lock()
	s.idleActive = false
	s.stopIdleTimersLocked()
	s.stopCompletionAlertLocked()
	s.idleSince = time.Time{}
	s.notified = false
	s.mu.Unlock()
}

func (s *DaemonState) armIdleTimersLocked(now time.Time) {
	cfg := GetRuntimeConfig()

	s.stopIdleTimersLocked()
	s.idleSince = now
	s.notified = false

	idleSince := s.idleSince
	if cfg.IdleWarnAfter > 0 {
		s.idleWarnTimer = time.AfterFunc(cfg.IdleWarnAfter, func() {
			s.notifyIfStillIdle(idleSince)
		})
	}
	if cfg.IdleLockAfter > 0 {
		s.idleLockTimer = time.AfterFunc(cfg.IdleLockAfter, func() {
			s.lockIfStillIdle(idleSince)
		})
	}
}

func (s *DaemonState) notifyIfStillIdle(idleSince time.Time) {
	s.mu.Lock()
	if !s.idleActive || !s.idleSince.Equal(idleSince) || s.currentTask != nil || s.isSystemLocked {
		s.mu.Unlock()
		return
	}
	if remaining := s.cooldownRemainingLocked(time.Now()); remaining > 0 {
		s.mu.Unlock()
		return
	}
	if s.notified {
		s.mu.Unlock()
		return
	}
	s.notified = true
	actions := s.actionsLocked()
	cfg := GetRuntimeConfig()
	s.mu.Unlock()

	remaining := (cfg.IdleLockAfter - cfg.IdleWarnAfter).Round(time.Second)
	actions.Notify("Idle Warning", "No task active. Locking in "+remaining.String()+".")
}

func (s *DaemonState) lockIfStillIdle(idleSince time.Time) {
	s.mu.Lock()
	if !s.idleActive || !s.idleSince.Equal(idleSince) || s.currentTask != nil || s.isSystemLocked {
		s.mu.Unlock()
		return
	}
	if remaining := s.cooldownRemainingLocked(time.Now()); remaining > 0 {
		s.mu.Unlock()
		return
	}
	s.stopIdleTimersLocked()
	s.idleSince = time.Time{}
	s.notified = false
	actions := s.actionsLocked()
	s.mu.Unlock()

	actions.LockScreen()
}

func (s *DaemonState) resumeIdleTrackingIfNeededLocked(now time.Time) {
	if !s.idleActive || s.currentTask != nil || s.isSystemLocked {
		return
	}
	if remaining := s.cooldownRemainingLocked(now); remaining > 0 {
		return
	}
	if s.idleSince.IsZero() || s.idleWarnTimer == nil || s.idleLockTimer == nil {
		s.armIdleTimersLocked(now)
	}
}

func (s *DaemonState) stopIdleTimersLocked() {
	if s.idleWarnTimer != nil {
		s.idleWarnTimer.Stop()
		s.idleWarnTimer = nil
	}
	if s.idleLockTimer != nil {
		s.idleLockTimer.Stop()
		s.idleLockTimer = nil
	}
}
