package status

import (
	"fmt"
	"time"

	"focus/internal/domain"
)

func Render(s domain.State, now time.Time) string {
	switch s.Phase {
	case domain.PhasePendingCooldown:
		remaining := s.CooldownStartUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown starting | Remaining: %s", remaining.Round(time.Second))
	case domain.PhaseCooldown:
		remaining := s.CooldownUntil.Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Cooldown active | Remaining: %s", remaining.Round(time.Second))
	case domain.PhaseBreak:
		if s.CurrentTask == nil {
			return "Status: break"
		}
		taskRemaining := s.CurrentTask.StartTime.Add(s.CurrentTask.Duration).Sub(now)
		if taskRemaining < 0 {
			taskRemaining = 0
		}
		breakRemaining := s.BreakUntil.Sub(now)
		if breakRemaining < 0 {
			breakRemaining = 0
		}
		return fmt.Sprintf("Task: %s | Status: break | Break remaining: %s | Task remaining: %s", s.CurrentTask.Title, breakRemaining.Round(time.Second), taskRemaining.Round(time.Second))
	case domain.PhaseActive:
		if s.CurrentTask == nil {
			return "Task active"
		}
		remaining := s.CurrentTask.StartTime.Add(s.CurrentTask.Duration).Sub(now)
		if remaining < 0 {
			remaining = 0
		}
		return fmt.Sprintf("Task: %s | Remaining: %s", s.CurrentTask.Title, remaining.Round(time.Second))
	default:
		if s.CurrentTask == nil && !s.ScreenLocked {
			return "Idle"
		}
		return "Idle"
	}
}
