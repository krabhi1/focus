package state

import "time"

func (s *DaemonState) startCompletionAlert() {
	s.mu.Lock()
	if !s.idleActive {
		actions := s.actionsLocked()
		s.mu.Unlock()
		actions.PlaySound("assets/task-ending.mp3")
		return
	}

	if s.completionAlertStop != nil {
		s.mu.Unlock()
		return
	}

	stopCh := make(chan struct{})
	s.completionAlertStop = stopCh
	repeatInterval := GetRuntimeConfig().CompletionAlertRepeatInterval
	actions := s.actionsLocked()
	s.mu.Unlock()

	go s.runCompletionAlertLoop(stopCh, actions, repeatInterval)
}

func (s *DaemonState) stopCompletionAlertLocked() {
	if s.completionAlertStop != nil {
		close(s.completionAlertStop)
		s.completionAlertStop = nil
	}
}

func (s *DaemonState) runCompletionAlertLoop(stopCh <-chan struct{}, actions interface{ PlaySound(string) }, repeatInterval time.Duration) {
	defer func() {
		s.mu.Lock()
		if s.completionAlertStop == stopCh {
			s.completionAlertStop = nil
		}
		s.mu.Unlock()
	}()

	for {
		select {
		case <-stopCh:
			return
		default:
		}

		actions.PlaySound("assets/task-ending.mp3")

		timer := time.NewTimer(repeatInterval)
		select {
		case <-stopCh:
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}

		s.mu.Lock()
		running := s.completionAlertStop == stopCh && s.idleActive
		s.mu.Unlock()
		if !running {
			return
		}
	}
}
