package main

import (
	"context"
	"sync"
)

type Queue struct {
	jobs      chan Delivery
	done      chan struct{}
	closeOnce sync.Once
}

func NewQueue(size int) *Queue {
	return &Queue{
		jobs: make(chan Delivery, size),
		done: make(chan struct{}),
	}
}

func (q *Queue) Enqueue(
	ctx context.Context,
	delivery Delivery,
) bool {
	select {
	case <-q.done:
		return false

	case <-ctx.Done():
		return false

	case q.jobs <- delivery:
		return true
	}
}

func (q *Queue) Jobs() <-chan Delivery {
	return q.jobs
}

func (q *Queue) Done() <-chan struct{} {
	return q.done
}

func (q *Queue) Close() {
	q.closeOnce.Do(func() {
		close(q.done)
	})
}

func (q *Queue) TryEnqueue(delivery Delivery) bool {
	select {
	case <-q.done:
		return false
	case q.jobs <- delivery:
		return true
	default:
		return false
	}
}
