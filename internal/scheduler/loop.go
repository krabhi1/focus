package scheduler

import (
	"container/heap"
	"sync"
	"time"
)

type CallbackHandle struct {
	loop *CallbackLoop
	id   int64
	once sync.Once
}

func (h *CallbackHandle) Cancel() {
	if h == nil || h.loop == nil {
		return
	}
	h.once.Do(func() {
		h.loop.cancel(h.id)
	})
}

type callbackEntry struct {
	id       int64
	at       time.Time
	fn       func()
	index    int
	canceled bool
}

type callbackQueue []*callbackEntry

func (q callbackQueue) Len() int { return len(q) }
func (q callbackQueue) Less(i, j int) bool {
	return q[i].at.Before(q[j].at)
}
func (q callbackQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *callbackQueue) Push(x any) {
	it := x.(*callbackEntry)
	it.index = len(*q)
	*q = append(*q, it)
}
func (q *callbackQueue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*q = old[:n-1]
	return it
}

type CallbackLoop struct {
	clock Clock

	mu      sync.Mutex
	seq     int64
	stopped bool
	timer   Timer
	queue   callbackQueue
	index   map[int64]*callbackEntry
	wake    chan struct{}
	stop    chan struct{}
	done    chan struct{}
}

func NewCallbackLoop(clock Clock) *CallbackLoop {
	if clock == nil {
		clock = realClock{}
	}
	l := &CallbackLoop{
		clock: clock,
		queue: make(callbackQueue, 0),
		index: make(map[int64]*callbackEntry),
		wake:  make(chan struct{}, 1),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	heap.Init(&l.queue)
	go l.run()
	return l
}

func (l *CallbackLoop) Stop() {
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		<-l.done
		return
	}
	l.stopped = true
	close(l.stop)
	l.stopTimerLocked()
	l.mu.Unlock()
	l.signalWake()
	<-l.done
}

func (l *CallbackLoop) Schedule(at time.Time, fn func()) *CallbackHandle {
	if fn == nil {
		return nil
	}
	l.mu.Lock()
	if l.stopped {
		l.mu.Unlock()
		return nil
	}
	l.seq++
	entry := &callbackEntry{id: l.seq, at: at, fn: fn}
	heap.Push(&l.queue, entry)
	l.index[entry.id] = entry
	l.mu.Unlock()
	l.signalWake()
	return &CallbackHandle{loop: l, id: entry.id}
}

func (l *CallbackLoop) cancel(id int64) {
	l.mu.Lock()
	entry, ok := l.index[id]
	if !ok || l.stopped {
		l.mu.Unlock()
		return
	}
	entry.canceled = true
	delete(l.index, id)
	l.mu.Unlock()
	l.signalWake()
}

func (l *CallbackLoop) run() {
	defer close(l.done)
	for {
		next, ok := l.nextDue()
		if !ok {
			select {
			case <-l.stop:
				return
			case <-l.wake:
				continue
			}
		}

		wait := next.at.Sub(l.clock.Now())
		if wait > 0 {
			l.mu.Lock()
			l.stopTimerLocked()
			timer := l.clock.AfterFunc(wait, func() {
				l.signalWake()
			})
			l.timer = timer
			l.mu.Unlock()

			select {
			case <-l.stop:
				l.mu.Lock()
				l.stopTimerLocked()
				l.mu.Unlock()
				return
			case <-l.wake:
			}

			l.mu.Lock()
			l.stopTimerLocked()
			l.mu.Unlock()
			continue
		}

		now := l.clock.Now()
		due := l.popDue(now)
		for _, entry := range due {
			if entry.canceled || entry.fn == nil {
				continue
			}
			entry.fn()
		}
	}
}

func (l *CallbackLoop) nextDue() (*callbackEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for l.queue.Len() > 0 {
		next := l.queue[0]
		if next.canceled {
			heap.Pop(&l.queue)
			continue
		}
		return next, true
	}
	return nil, false
}

func (l *CallbackLoop) popDue(now time.Time) []*callbackEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]*callbackEntry, 0)
	for l.queue.Len() > 0 {
		next := l.queue[0]
		if next.canceled {
			heap.Pop(&l.queue)
			continue
		}
		if next.at.After(now) {
			break
		}
		entry := heap.Pop(&l.queue).(*callbackEntry)
		delete(l.index, entry.id)
		out = append(out, entry)
	}
	return out
}

func (l *CallbackLoop) stopTimerLocked() {
	if l.timer != nil {
		l.timer.Stop()
		l.timer = nil
	}
}

func (l *CallbackLoop) signalWake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}
