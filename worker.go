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

func retryDelay(attempt int) time.Duration {
	return time.Second * time.Duration(1<<(attempt-1))
}

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

	for {
		select {
		case <-w.queue.done:
			log.Printf("worker %d stopped", w.id)
			return

		case delivery := <-w.queue.Jobs():
			w.process(context.Background(), delivery)
		}
	}
}

func (w *Worker) process(
	ctx context.Context,
	delivery Delivery,
) {
	attempts, ok := w.claimDelivery(ctx, delivery.ID)

	if !ok {
		return
	}

	delivery.Attempts = attempts

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
		log.Printf(
			"worker=%d delivery=%s failed: %v",
			w.id,
			delivery.ID,
			err,
		)
		w.handleFailure(ctx, delivery)
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

	w.handleFailure(ctx, delivery)

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
		SET status = $1,
				lease_until = NULL
		WHERE id = $2
			AND status = $3
		`,
		DeliverySuccess,
		deliveryID,
		DeliveryProcessing,
	)

	if err != nil {
		log.Printf(
			"failed marking delivery=%s success: %v",
			deliveryID,
			err,
		)
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

func (w *Worker) handleFailure(
	ctx context.Context,
	delivery Delivery,
) {

	if delivery.Attempts >= MaxAttempts {

		_, err := w.db.Exec(
			ctx,
			`
			UPDATE deliveries
			SET status = $1,
			    next_retry_at = NULL,
					lease_until = NULL
			WHERE id = $2
			`,
			DeliveryFailed,
			delivery.ID,
		)

		if err != nil {
			log.Printf(
				"failed marking delivery=%s failed: %v",
				delivery.ID,
				err,
			)
		}

		return
	}

	delay := retryDelay(delivery.Attempts)
	nextRetryAt := time.Now().Add(delay)

	_, err := w.db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET status = $1,
			  next_retry_at = $2,
				lease_until = NULL
		WHERE id = $3
			AND status = $4
		`,
		DeliveryPending,
		nextRetryAt,
		delivery.ID,
		DeliveryProcessing,
	)

	if err != nil {
		log.Printf(
			"failed scheduling retry delivery=%s: %v",
			delivery.ID,
			err,
		)
		return
	}

	log.Printf(
		"delivery=%s attempt=%d retry at=%s",
		delivery.ID,
		delivery.Attempts,
		nextRetryAt.Format(time.RFC3339),
	)

}

func (w *Worker) claimDelivery(
	ctx context.Context,
	deliveryID string,
) (int, bool) {

	var attempts int

	// We use QueryRow instead of Exec because the 'RETURNING' clause acts like a SELECT.
	//
	// WHY WE DO THIS:
	// 1. Efficiency: Avoids a second round-trip to the database (eliminates a separate SELECT query).
	// 2. Concurrency Safety: 'attempts = attempts + 1' is atomic inside the DB. RETURNING
	//    guarantees our Go code gets the exact value from *this* update, preventing race
	//    conditions if multiple concurrent workers process the same delivery.
	err := w.db.QueryRow(
		ctx,
		`
		UPDATE deliveries
		SET
			status = $1,
			attempts = attempts + 1,
			next_retry_at = NULL,
			lease_until = NOW() + INTERVAL '30 seconds'
		WHERE id = $2
		  AND status = $3
		RETURNING attempts
		`,
		DeliveryProcessing,
		deliveryID,
		DeliveryPending,
	).Scan(&attempts)

	if err != nil {
		log.Printf(
			"failed claiming delivery=%s: %v",
			deliveryID,
			err,
		)
		return 0, false
	}

	return attempts, true
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

func startRetryScheduler(
	ctx context.Context,
	db *pgxpool.Pool,
	queue *Queue,
) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	log.Println("retry scheduler started")

	for {
		select {
		case <-ctx.Done():
			log.Println("retry scheduler stopped")
			return

		case <-ticker.C:
			recoverStuckDeliveries(ctx, db)
			scheduleRetries(ctx, db, queue)
		}
	}
}

func scheduleRetries(
	ctx context.Context,
	db *pgxpool.Pool,
	queue *Queue,
) {
	rows, err := db.Query(
		ctx,
		`
		SELECT
			id,
			event_id,
			webhook_id,
			status,
			attempts
		FROM deliveries
		WHERE status = $1
		  AND attempts > 0
		  AND next_retry_at <= NOW()
		ORDER BY next_retry_at
		LIMIT 100
		`,
		DeliveryPending,
	)

	if err != nil {
		log.Printf("retry scheduler query failed: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var delivery Delivery

		if err := rows.Scan(
			&delivery.ID,
			&delivery.EventID,
			&delivery.WebhookID,
			&delivery.Status,
			&delivery.Attempts,
		); err != nil {
			log.Printf("retry scheduler scan failed: %v", err)
			continue
		}

		if !queue.Enqueue(ctx, delivery) {
			return
		}
	}
}

func recoverStuckDeliveries(
	ctx context.Context,
	db *pgxpool.Pool,
) {

	result, err := db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET 
				status = $1,
				next_retry_at = NOW(),
				lease_until = NULL	
		WHERE status = $2
		  AND lease_until <= NOW()
		`,
		DeliveryPending,
		DeliveryProcessing,
	)

	if err != nil {
		log.Printf(
			"failed recovering stuck deliveries: %v",
			err,
		)
		return
	}

	if result.RowsAffected() > 0 {
		log.Printf(
			"recovered %d stuck deliveries",
			result.RowsAffected(),
		)
	}
}
