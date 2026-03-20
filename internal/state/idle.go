package state

import (
	"time"
)

func (s *DaemonState) StartIdleMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()
		actions := s.actionsLocked()

		if s.currentTask != nil || s.isSystemLocked {
			s.idleSince = time.Time{}
			s.notified = false
			s.mu.Unlock()
			continue
		}

		if s.idleSince.IsZero() {
			s.idleSince = time.Now()
		}

		elapsed := time.Since(s.idleSince)

		if elapsed >= 5*time.Minute {
			actions.LockScreen()
			s.idleSince = time.Time{}
		} else if elapsed >= 3*time.Minute && !s.notified {
			actions.Notify("Idle Warning", "No task active. Locking in 2 minute.")
			s.notified = true
		}

		s.mu.Unlock()
	}
}
