package state

import (
	"context"
	"time"
)

func (s *DaemonState) StartIdleMonitor(ctx context.Context) {
	cfg := GetRuntimeConfig()
	ticker := time.NewTicker(cfg.IdlePollInterval)
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

			if remaining := s.cooldownRemainingLocked(time.Now()); remaining > 0 {
				actions.LockScreen()
				s.idleSince = time.Time{}
				s.notified = false
				s.mu.Unlock()
				continue
			}

			if s.idleSince.IsZero() {
				s.idleSince = time.Now()
			}

			elapsed := time.Since(s.idleSince)

			if elapsed >= cfg.IdleLockAfter {
				actions.LockScreen()
				s.idleSince = time.Time{}
			} else if elapsed >= cfg.IdleWarnAfter && !s.notified {
				remaining := (cfg.IdleLockAfter - cfg.IdleWarnAfter).Round(time.Second)
				actions.Notify("Idle Warning", "No task active. Locking in "+remaining.String()+".")
				s.notified = true
			}

			s.mu.Unlock()
		}
	}
}
