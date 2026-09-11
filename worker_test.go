package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func connectTestDB(ctx context.Context) (*pgxpool.Pool, error) {
	return pgxpool.New(
		ctx,
		"postgres://postgres:postgres@localhost:5432/wh_platform_test",
	)
}

func createTestDelivery(
	t *testing.T,
	ctx context.Context,
	db *pgxpool.Pool,
) string {
	t.Helper()

	// Create the event.
	eventID := uuid.New()

	_, err := db.Exec(
		ctx,
		`
		INSERT INTO events (id, event_type, payload)
		VALUES ($1, 'test.event', '{}')
		`,
		eventID,
	)

	if err != nil {
		t.Fatal(err)
	}

	// Create webhook.
	webhookID := uuid.New()

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO webhooks (id, url)
		VALUES ($1, 'http://localhost:9000/webhook')
		`,
		webhookID,
	)

	if err != nil {
		t.Fatal(err)
	}

	// Create delivery.
	deliveryID := uuid.New()

	_, err = db.Exec(
		ctx,
		`
		INSERT INTO deliveries
		(id, event_id, webhook_id, status)
		VALUES ($1, $2, $3, 'pending')
		`,
		deliveryID,
		eventID,
		webhookID,
	)

	if err != nil {
		t.Fatal(err)
	}

	return deliveryID.String()
}

func TestClaimDeliveryOnlyOneWorkerWins(t *testing.T) {
	ctx := context.Background()

	// Uses setupTest to guarantee a blank canvas for the race conditions test
	db := setupTest(t)

	deliveryID := createTestDelivery(t, ctx, db)

	const workers = 10

	var wg sync.WaitGroup

	successes := make(chan bool, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			worker := NewWorker(
				1,
				nil,
				db,
			)

			_, ok := worker.claimDelivery(
				ctx,
				deliveryID,
			)

			successes <- ok
		}()
	}

	wg.Wait()

	close(successes)

	successCount := 0

	for ok := range successes {
		if ok {
			successCount++
		}
	}

	if successCount != 1 {
		t.Fatalf(
			"expected exactly 1 successful claim, got %d",
			successCount,
		)
	}
}

func TestRecoverStuckDelivery(t *testing.T) {
	ctx := context.Background()

	db := setupTest(t)

	deliveryID := createTestDelivery(t, ctx, db)

	// Simulate a worker that claimed the delivery.
	_, err := db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET
			status = 'processing',
			lease_until = NOW() - INTERVAL '1 second'
		WHERE id = $1
		`,
		deliveryID,
	)

	if err != nil {
		t.Fatal(err)
	}

	recoverStuckDeliveries(ctx, db)

	var status string

	err = db.QueryRow(
		ctx,
		`SELECT status FROM deliveries WHERE id = $1`,
		deliveryID,
	).Scan(&status)

	if err != nil {
		t.Fatal(err)
	}

	if status != DeliveryPending {
		t.Fatalf(
			"expected status=%s, got %s",
			DeliveryPending,
			status,
		)
	}
}

func TestActiveDeliveryIsNotRecovered(t *testing.T) {
	ctx := context.Background()

	db := setupTest(t)

	deliveryID := createTestDelivery(t, ctx, db)

	_, err := db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET
			status = 'processing',
			lease_until = NOW() + INTERVAL '30 seconds'
		WHERE id = $1
		`,
		deliveryID,
	)

	if err != nil {
		t.Fatal(err)
	}

	recoverStuckDeliveries(ctx, db)

	var status string

	err = db.QueryRow(
		ctx,
		`SELECT status FROM deliveries WHERE id = $1`,
		deliveryID,
	).Scan(&status)

	if err != nil {
		t.Fatal(err)
	}

	if status != DeliveryProcessing {
		t.Fatalf(
			"expected status=%s, got %s",
			DeliveryProcessing,
			status,
		)
	}
}

// retry survives restart
func TestRetryStateIsDurable(t *testing.T) {
	ctx := context.Background()

	setupTest(t)

	// Separate pool so we can close/reopen it without
	// affecting the shared testDB.
	db, err := connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}

	deliveryID := createTestDelivery(t, ctx, db)

	nextRetry := time.Now().Add(10 * time.Second)

	_, err = db.Exec(
		ctx,
		`
		UPDATE deliveries
		SET
			status = $1,
			next_retry_at = $2
		WHERE id = $3
		`,
		DeliveryPending,
		nextRetry,
		deliveryID,
	)

	if err != nil {
		db.Close()
		t.Fatal(err)
	}

	// Simulate application restart.
	db.Close()

	// Reopen a new pool.
	db, err = connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var (
		status      string
		storedRetry time.Time
	)

	err = db.QueryRow(
		ctx,
		`
		SELECT status, next_retry_at
		FROM deliveries
		WHERE id = $1
		`,
		deliveryID,
	).Scan(
		&status,
		&storedRetry,
	)

	if err != nil {
		t.Fatal(err)
	}

	if status != DeliveryPending {
		t.Fatalf(
			"expected pending, got %s",
			status,
		)
	}

	if storedRetry.IsZero() {
		t.Fatal("expected next_retry_at to survive restart")
	}
}

func TestIsRetryableStatus(t *testing.T) {
	tests := []struct {
		status    int
		retryable bool
	}{
		{200, false},
		{201, false},
		{301, false},
		{302, false},
		{400, false},
		{401, false},
		{404, false},
		{500, true},
		{502, true},
		{503, true},
		{599, true},
	}

	for _, tt := range tests {
		got := isRetryableStatus(tt.status)

		if got != tt.retryable {
			t.Fatalf(
				"status=%d got=%v want=%v",
				tt.status,
				got,
				tt.retryable,
			)
		}
	}
}

func TestRetryDelay(t *testing.T) {
	tests := []struct {
		name         string
		attempt      int
		expectedBase time.Duration
	}{
		{name: "First attempt (1s base)", attempt: 1, expectedBase: 1 * time.Second},
		{name: "Second attempt (2s base)", attempt: 2, expectedBase: 2 * time.Second},
		{name: "Third attempt (4s base)", attempt: 3, expectedBase: 4 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delay := retryDelay(tt.attempt)

			// Calculate boundaries directly from the base value
			minAllowed := tt.expectedBase
			maxAllowed := tt.expectedBase + time.Duration(float64(tt.expectedBase)*0.25)

			if delay < minAllowed || delay > maxAllowed {
				t.Fatalf(
					"Attempt %d out of bounds:\n  Got:      %v\n  Expected: %v to %v",
					tt.attempt, delay, minAllowed, maxAllowed,
				)
			}
		})
	}
}

func TestRetryDelayHasJitter(t *testing.T) {
	first := retryDelay(1)

	different := false

	for range 20 {
		if retryDelay(1) != first {
			different = true
			break
		}
	}

	if !different {
		t.Fatal("expected retry delay to include jitter")
	}
}

func TestScheduleRetries_NewPendingDelivery(t *testing.T) {

	ctx := context.Background()
	db := setupTest(t)
	q := NewQueue(10)
	defer q.Close()

	// Create webhook.
	_, err := createWebhook(ctx, db, CreateWebhookRequest{
		URL:    "http://example.com/webhook",
		Events: []string{"order.created"},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	// Create event.
	_, _, deliveries, err := createEvent(ctx, db, CreateEventRequest{
		Type:    "order.created",
		Payload: map[string]any{"order_id": "123"},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	deliveryID := deliveries[0].ID

	// Make sure this represents a brand-new pending delivery.
	_, err = db.Exec(ctx, `
		UPDATE deliveries
		SET status = 'pending',
		    attempts = 0,
		    next_retry_at = NULL,
		    lease_until = NULL
		WHERE id = $1
	`, deliveryID)
	if err != nil {
		t.Fatalf("reset delivery: %v", err)
	}

	// Run scheduler once.
	scheduleRetries(ctx, db, q)

	// It should now be in the queue.
	select {
	case got := <-q.Jobs():
		if got.ID != deliveryID {
			t.Fatalf(
				"expected delivery %s, got %s",
				deliveryID,
				got.ID,
			)
		}

	default:
		t.Fatal("expected new pending delivery to be queued")
	}
}

func TestScheduleRetries_DoesNotScheduleFutureRetry(t *testing.T) {

	ctx := context.Background()
	db := setupTest(t)
	q := NewQueue(10)
	defer q.Close()

	_, err := createWebhook(ctx, db, CreateWebhookRequest{
		URL:    "http://example.com/webhook",
		Events: []string{"order.created"},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	_, _, deliveries, err := createEvent(ctx, db, CreateEventRequest{
		Type:    "order.created",
		Payload: map[string]any{"order_id": "123"},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}

	deliveryID := deliveries[0].ID

	// Simulate a failed attempt waiting for its retry time.
	_, err = db.Exec(ctx, `
		UPDATE deliveries
		SET status = 'pending',
		    attempts = 1,
		    next_retry_at = NOW() + INTERVAL '10 seconds',
		    lease_until = NULL
		WHERE id = $1
	`, deliveryID)
	if err != nil {
		t.Fatalf("update delivery: %v", err)
	}

	scheduleRetries(ctx, db, q)

	select {
	case got := <-q.Jobs():
		t.Fatalf(
			"delivery %s should not have been queued, but was",
			got.ID,
		)

	default:
		// Expected.
	}
}

func TestScheduleRetries_DueRetry(t *testing.T) {

	ctx := context.Background()
	db := setupTest(t)
	q := NewQueue(10)
	defer q.Close()

	_, err := createWebhook(ctx, db, CreateWebhookRequest{
		URL:    "http://example.com/webhook",
		Events: []string{"order.created"},
	})
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	_, _, deliveries, err := createEvent(ctx, db, CreateEventRequest{
		Type:    "order.created",
		Payload: map[string]any{"order_id": "123"},
	})
	if err != nil {
		t.Fatalf("create event: %v", err)
	}

	deliveryID := deliveries[0].ID

	// Simulate a failed delivery whose retry time has arrived.
	_, err = db.Exec(ctx, `
		UPDATE deliveries
		SET status = 'pending',
		    attempts = 1,
		    next_retry_at = NOW() - INTERVAL '1 second',
		    lease_until = NULL
		WHERE id = $1
	`, deliveryID)
	if err != nil {
		t.Fatalf("update delivery: %v", err)
	}

	scheduleRetries(ctx, db, q)

	select {
	case got := <-q.Jobs():
		if got.ID != deliveryID {
			t.Fatalf(
				"expected delivery %s, got %s",
				deliveryID,
				got.ID,
			)
		}

	default:
		t.Fatal("expected due retry to be queued")
	}
}
