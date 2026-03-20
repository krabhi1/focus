package state

import (
	"context"
	"time"
)

func (s *DaemonState) StartIdleMonitor(ctx context.Context) {
	ticker := time.NewTicker(IdleMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
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

			if elapsed >= IdleLockAfter {
				actions.LockScreen()
				s.idleSince = time.Time{}
			} else if elapsed >= IdleWarningAfter && !s.notified {
				actions.Notify("Idle Warning", "No task active. Locking in 2 minute.")
				s.notified = true
			}

			s.mu.Unlock()
		}
	}
}
