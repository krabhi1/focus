package state

import (
	"focus/internal/sys"
	"time"
)

func (s *DaemonState) StartIdleMonitor() {
	ticker := time.NewTicker(30 * time.Second)
	for range ticker.C {
		s.mu.Lock()

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
			sys.LockScreen()
			s.idleSince = time.Time{}
		} else if elapsed >= 3*time.Minute && !s.notified {
			sys.Notify("Idle Warning", "No task active. Locking in 2 minute.")
			s.notified = true
		}

		s.mu.Unlock()
	}
}
