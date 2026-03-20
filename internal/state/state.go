package state

var global = &DaemonState{
	taskHistory:    []*Task{},
	isSystemLocked: false,
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
