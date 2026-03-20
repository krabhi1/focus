package state

import "focus/internal/sys"

var global = &DaemonState{
	taskHistory:    []*Task{},
	isSystemLocked: false,
	actions:        sys.RealActions{},
}

// Get returns the singleton instance.
func Get() *DaemonState {
	return global
}

func (s *DaemonState) SetSystemLocked(locked bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isSystemLocked = locked
}

func (s *DaemonState) IsSystemLocked() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isSystemLocked
}

func (s *DaemonState) SetActions(actions sys.Actions) {
	if actions == nil {
		actions = sys.RealActions{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.actions = actions
}
