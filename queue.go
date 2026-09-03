package main

type Queue struct {
	jobs chan Delivery
}

func NewQueue(size int) *Queue {
	return &Queue{
		jobs: make(chan Delivery, size),
	}
}

func (q *Queue) Enqueue(delivery Delivery) {
	q.jobs <- delivery
}

func (q *Queue) Jobs() <-chan Delivery {
	return q.jobs
}
