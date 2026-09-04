package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Worker struct {
	id     int
	queue  *Queue
	db     *pgxpool.Pool
	client *http.Client
}

func NewWorker(
	id int,
	queue *Queue,
	db *pgxpool.Pool,
) *Worker {
	return &Worker{
		id:    id,
		queue: queue,
		db:    db,
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (w *Worker) Start() {
	log.Printf("worker %d started", w.id)

	for delivery := range w.queue.Jobs() {
		w.process(context.Background(), delivery)
	}

	log.Printf("worker %d stopped", w.id)
}

func (w *Worker) process(
	ctx context.Context,
	delivery Delivery,
) {
	log.Printf(
		"worker=%d processing delivery=%s",
		w.id,
		delivery.ID,
	)

	var (
		webhookURL string
		eventType  string
		payload    []byte
	)

	err := w.db.QueryRow(
		ctx,
		`
		SELECT
			w.url,
			e.event_type,
			e.payload
		FROM deliveries d
		JOIN webhooks w ON w.id = d.webhook_id
		JOIN events e ON e.id = d.event_id
		WHERE d.id = $1
		`,
		delivery.ID,
	).Scan(
		&webhookURL,
		&eventType,
		&payload,
	)

	if err != nil {
		log.Printf(
			"worker=%d failed loading delivery=%s: %v",
			w.id,
			delivery.ID,
			err,
		)

		return
	}

	body := struct {
		ID      string          `json:"id"`
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}{
		ID:      delivery.EventID,
		Type:    eventType,
		Payload: payload,
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		log.Printf("failed to marshal delivery: %v", err)
		return
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		webhookURL,
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		log.Printf("failed creating request: %v", err)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		w.markFailed(ctx, delivery.ID)
		log.Printf(
			"worker=%d delivery=%s failed: %v",
			w.id,
			delivery.ID,
			err,
		)
		return
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.markSuccess(ctx, delivery.ID)

		log.Printf(
			"worker=%d delivery=%s succeeded status=%d",
			w.id,
			delivery.ID,
			resp.StatusCode,
		)

		return
	}

	w.markFailed(ctx, delivery.ID)

	log.Printf(
		"worker=%d delivery=%s failed status=%d",
		w.id,
		delivery.ID,
		resp.StatusCode,
	)
}

func (w *Worker) markSuccess(
	ctx context.Context,
	deliveryID string,
) {
	_, err := w.db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET status = 'success',
		    attempts = attempts + 1
		WHERE id = $1
		`,
		deliveryID,
	)

	if err != nil {
		log.Printf("failed updating delivery: %v", err)
	}
}

func (w *Worker) markFailed(
	ctx context.Context,
	deliveryID string,
) {
	_, err := w.db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET status = 'failed',
		    attempts = attempts + 1
		WHERE id = $1
		`,
		deliveryID,
	)

	if err != nil {
		log.Printf("failed updating delivery: %v", err)
	}
}

// -------------------------
// Worker Pool
// -------------------------

type WorkerPool struct {
	queue   *Queue
	db      *pgxpool.Pool
	workers int
	wg      sync.WaitGroup
}

func NewWorkerPool(
	workers int,
	queue *Queue,
	db *pgxpool.Pool,
) *WorkerPool {
	return &WorkerPool{
		workers: workers,
		queue:   queue,
		db:      db,
	}
}

func (p *WorkerPool) Start() {
	for i := 1; i <= p.workers; i++ {
		p.wg.Add(1)

		go func(id int) {
			defer p.wg.Done()

			worker := NewWorker(id, p.queue, p.db)
			worker.Start()
		}(i)
	}
}

func (p *WorkerPool) Wait() {
	p.wg.Wait()
}
