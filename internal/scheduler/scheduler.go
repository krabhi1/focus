package scheduler

import (
	"context"
	"time"

	"focus/internal/domain"
)

type Runner interface {
	Apply(domain.Event) domain.Result
}

type Scheduler struct {
	clock Clock
	queue *Queue
}

func New(clock Clock) *Scheduler {
	if clock == nil {
		clock = realClock{}
	}
	return &Scheduler{
		clock: clock,
		queue: NewQueue(),
	}
}

func (s *Scheduler) Add(d domain.Deadline) {
	s.queue.Push(d)
}

func (s *Scheduler) Run(ctx context.Context, r Runner) error {
	for {
		next, ok := s.queue.Next()
		if !ok {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				time.Sleep(10 * time.Millisecond)
				continue
			}
		}

		wait := next.At.Sub(s.clock.Now())
		if wait > 0 {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}

		now := s.clock.Now()
		for _, d := range s.queue.PopDue(now) {
			res := r.Apply(domain.Event{Type: d.Type, At: now})
			for _, nd := range res.Deadlines {
				s.Add(nd)
			}
		}
	}
}
