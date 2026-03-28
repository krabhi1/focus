package scheduler

import (
	"testing"
	"time"

	"focus/internal/domain"
)

func TestQueueOrdersDeadlines(t *testing.T) {
	q := NewQueue()
	base := time.Date(2026, 3, 22, 12, 0, 0, 0, time.UTC)

	q.Push(domain.Deadline{At: base.Add(30 * time.Second), Type: domain.EventTick})
	q.Push(domain.Deadline{At: base.Add(10 * time.Second), Type: domain.EventTick})
	q.Push(domain.Deadline{At: base.Add(20 * time.Second), Type: domain.EventTick})

	got := q.PopDue(base.Add(15 * time.Second))
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	if !got[0].At.Equal(base.Add(10 * time.Second)) {
		t.Fatalf("first deadline = %s, want %s", got[0].At, base.Add(10*time.Second))
	}
}
