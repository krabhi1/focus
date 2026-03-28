package scheduler

import (
	"sync"
	"testing"
	"time"
)

func TestCallbackLoopRunsScheduledJobs(t *testing.T) {
	loop := NewCallbackLoop(realClock{})
	defer loop.Stop()

	var mu sync.Mutex
	var calls []string
	done := make(chan struct{}, 1)

	loop.Schedule(time.Now().Add(10*time.Millisecond), func() {
		mu.Lock()
		calls = append(calls, "first")
		mu.Unlock()
	})
	loop.Schedule(time.Now().Add(20*time.Millisecond), func() {
		mu.Lock()
		calls = append(calls, "second")
		mu.Unlock()
		done <- struct{}{}
	})

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for scheduled jobs")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want 2 callbacks", calls)
	}
	if calls[0] != "first" || calls[1] != "second" {
		t.Fatalf("calls = %v, want first then second", calls)
	}
}

func TestCallbackLoopCancelStopsJob(t *testing.T) {
	loop := NewCallbackLoop(realClock{})
	defer loop.Stop()

	fired := make(chan struct{}, 1)
	handle := loop.Schedule(time.Now().Add(15*time.Millisecond), func() {
		fired <- struct{}{}
	})
	handle.Cancel()

	select {
	case <-fired:
		t.Fatal("canceled callback still fired")
	case <-time.After(100 * time.Millisecond):
	}
}
