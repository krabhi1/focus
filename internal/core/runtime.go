package core

import (
	"sync"
	"time"
)

type Runtime struct {
	mu     sync.RWMutex
	state  State
	events chan Event
	stop   chan struct{}
	done   chan struct{}
}

func NewRuntime(initial State) *Runtime {
	rt := &Runtime{
		state:  initial,
		events: make(chan Event, 64),
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
	}
	go rt.loop()
	return rt
}

func (r *Runtime) Publish(ev Event) {
	select {
	case r.events <- ev:
	default:
		// Drop when overloaded in shadow mode to avoid backpressure.
	}
}

func (r *Runtime) Snapshot() State {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

func (r *Runtime) Close() {
	close(r.stop)
	<-r.done
}

func (r *Runtime) loop() {
	defer close(r.done)

	var timer *time.Timer
	var timerCh <-chan time.Time

	resetTimer := func() {
		next := r.Snapshot().NextWakeAt()
		if timer != nil {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
		timerCh = nil
		if next.IsZero() {
			return
		}
		d := time.Until(next)
		if d < 0 {
			d = 0
		}
		timer = time.NewTimer(d)
		timerCh = timer.C
	}

	apply := func(ev Event) {
		r.mu.Lock()
		r.state = Reduce(r.state, ev)
		r.mu.Unlock()
		resetTimer()
	}

	resetTimer()

	for {
		select {
		case <-r.stop:
			if timer != nil {
				timer.Stop()
			}
			return
		case ev := <-r.events:
			apply(ev)
		case <-timerCh:
			apply(Event{Type: EventTick, At: time.Now()})
		}
	}
}
