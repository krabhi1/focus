package scheduler

import (
	"container/heap"
	"time"

	"focus/internal/domain"
)

type item struct {
	deadline domain.Deadline
	index    int
}

type deadlineQueue []*item

func (q deadlineQueue) Len() int { return len(q) }
func (q deadlineQueue) Less(i, j int) bool {
	return q[i].deadline.At.Before(q[j].deadline.At)
}
func (q deadlineQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *deadlineQueue) Push(x any) {
	it := x.(*item)
	it.index = len(*q)
	*q = append(*q, it)
}
func (q *deadlineQueue) Pop() any {
	old := *q
	n := len(old)
	it := old[n-1]
	old[n-1] = nil
	it.index = -1
	*q = old[:n-1]
	return it
}

type Queue struct {
	items deadlineQueue
}

func NewQueue() *Queue {
	q := &Queue{items: make(deadlineQueue, 0)}
	heap.Init(&q.items)
	return q
}

func (q *Queue) Push(d domain.Deadline) {
	heap.Push(&q.items, &item{deadline: d})
}

func (q *Queue) Next() (domain.Deadline, bool) {
	if q.items.Len() == 0 {
		return domain.Deadline{}, false
	}
	return q.items[0].deadline, true
}

func (q *Queue) PopDue(now time.Time) []domain.Deadline {
	out := make([]domain.Deadline, 0)
	for q.items.Len() > 0 {
		next := q.items[0].deadline
		if next.At.After(now) {
			break
		}
		out = append(out, heap.Pop(&q.items).(*item).deadline)
	}
	return out
}
