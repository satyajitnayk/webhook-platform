package main

import (
	"context"
	"fmt"
	"os"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func connectDB(ctx context.Context) (*pgxpool.Pool, error) {
	dsn := os.Getenv("DATABASE_URL")

	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("create db pool: %w", err)
	}

	if err := db.Ping(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}

func createWebhook(
	ctx context.Context,
	db *pgxpool.Pool,
	req CreateWebhookRequest,
) (string, string, error) {
	tx, err := db.Begin(ctx)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback(ctx)

	webhookID := uuid.New()

	secret, err := generateWebhookSecret()
	if err != nil {
		return "", "", err
	}

	_, err = tx.Exec(
		ctx,
		`INSERT INTO webhooks (id, url, secret)
		 VALUES ($1, $2, $3)`,
		webhookID,
		req.URL,
		secret,
	)
	if err != nil {
		return "", "", err
	}

	for _, eventType := range req.Events {
		_, err = tx.Exec(
			ctx,
			`INSERT INTO subscriptions (id, webhook_id, event_type)
			 VALUES ($1, $2, $3)`,
			uuid.New(),
			webhookID,
			eventType,
		)
		if err != nil {
			return "", "", err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return "", "", err
	}

	return webhookID.String(), secret, nil
}

func createEvent(
	ctx context.Context,
	db *pgxpool.Pool,
	req CreateEventRequest,
) (string, int, []Delivery, error) {

	tx, err := db.Begin(ctx)
	if err != nil {
		return "", 0, nil, err
	}
	defer tx.Rollback(ctx)

	eventID := uuid.New()

	_, err = tx.Exec(
		ctx,
		`INSERT INTO events (id, event_type, payload)
		 VALUES ($1, $2, $3)`,
		eventID,
		req.Type,
		req.Payload,
	)
	if err != nil {
		return "", 0, nil, err
	}

	rows, err := tx.Query(
		ctx,
		`SELECT webhook_id
		 FROM subscriptions
		 WHERE event_type = $1
		`,
		req.Type,
	)
	if err != nil {
		return "", 0, nil, err
	}

	var webhookIDs []uuid.UUID

	for rows.Next() {
		var webhookID uuid.UUID

		if err := rows.Scan(&webhookID); err != nil {
			return "", 0, nil, err
		}

		webhookIDs = append(webhookIDs, webhookID)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return "", 0, nil, err
	}

	rows.Close()

	var deliveries []Delivery

	for _, webhookID := range webhookIDs {

		deliveryID := uuid.New()

		_, err = tx.Exec(
			ctx,
			`INSERT INTO deliveries
			 (id, event_id, webhook_id, status)
			 VALUES ($1, $2, $3, $4)`,
			deliveryID,
			eventID,
			webhookID,
			"pending",
		)
		if err != nil {
			return "", 0, nil, err
		}

		deliveries = append(deliveries, Delivery{
			ID:        deliveryID.String(),
			EventID:   eventID.String(),
			WebhookID: webhookID.String(),
			Status:    "pending",
		})
	}

	if err := rows.Err(); err != nil {
		return "", 0, nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return "", 0, deliveries, err
	}

	return eventID.String(), len(deliveries), deliveries, nil
}

func getWebhooks(
	ctx context.Context,
	db *pgxpool.Pool,
) ([]Webhook, error) {
	rows, err := db.Query(
		ctx,
		`
		SELECT
			id,
			url,
			created_at
		FROM webhooks
		ORDER BY created_at DESC
		`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	webhooks := make([]Webhook, 0)

	for rows.Next() {
		var webhook Webhook

		if err := rows.Scan(
			&webhook.ID,
			&webhook.URL,
			&webhook.CreatedAt,
		); err != nil {
			return nil, err
		}

		webhooks = append(webhooks, webhook)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return webhooks, nil
}

func getWebhook(
	ctx context.Context,
	db *pgxpool.Pool,
	webhookID string,
) (WebhookDetails, error) {
	var webhook WebhookDetails

	err := db.QueryRow(
		ctx,
		`
		SELECT
			w.id,
			w.url,
			w.created_at,
			COALESCE(
				ARRAY(
					SELECT s.event_type
					FROM subscriptions s
					WHERE s.webhook_id = w.id
					ORDER BY s.event_type
				),
				ARRAY[]::text[]
			),
			(
				SELECT COUNT(*)
				FROM deliveries d
				WHERE d.webhook_id = w.id
			)
		FROM webhooks w
		WHERE w.id = $1
		`,
		webhookID,
	).Scan(
		&webhook.ID,
		&webhook.URL,
		&webhook.CreatedAt,
		&webhook.Events,
		&webhook.DeliveryCount,
	)

	if err != nil {
		return WebhookDetails{}, err
	}

	return webhook, nil
}

func getDeliveries(
	ctx context.Context,
	db *pgxpool.Pool,
) ([]Delivery, error) {
	rows, err := db.Query(
		ctx,
		`
		SELECT
			id,
			event_id,
			webhook_id,
			status,
			attempts,
			next_retry_at,
			lease_until,
			created_at
		FROM deliveries
		ORDER BY created_at DESC
		LIMIT 100
		`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deliveries := make([]Delivery, 0)

	for rows.Next() {
		var delivery Delivery

		if err := rows.Scan(
			&delivery.ID,
			&delivery.EventID,
			&delivery.WebhookID,
			&delivery.Status,
			&delivery.Attempts,
			&delivery.NextRetryAt,
			&delivery.LeaseUntil,
			&delivery.CreatedAt,
		); err != nil {
			return nil, err
		}

		deliveries = append(deliveries, delivery)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return deliveries, nil
}
