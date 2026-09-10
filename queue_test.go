package main

import "testing"

func TestTryEnqueue(t *testing.T) {
	q := NewQueue(1)
	defer q.Close()

	delivery := Delivery{ID: "1"}

	if !q.TryEnqueue(delivery) {
		t.Fatal("expected enqueue to succeed")
	}

	if q.TryEnqueue(Delivery{ID: "2"}) {
		t.Fatal("expected enqueue to fail when queue is full")
	}
}

func TestTryEnqueueClosedQueue(t *testing.T) {
	q := NewQueue(1)
	q.Close()

	if q.TryEnqueue(Delivery{ID: "1"}) {
		t.Fatal("expected enqueue to fail when queue is closed")
	}
}
