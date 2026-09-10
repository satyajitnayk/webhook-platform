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

	// Use your test database connection here.
	db, err := connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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

	db, err := connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	deliveryID := createTestDelivery(t, ctx, db)

	// Simulate a worker that claimed the delivery.
	_, err = db.Exec(
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

	db, err := connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	deliveryID := createTestDelivery(t, ctx, db)

	_, err = db.Exec(
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

	db, err := connectTestDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

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
		t.Fatal(err)
	}

	// Simulate application restart by closing/reopening DB.
	db.Close()

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
