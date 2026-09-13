package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func createWebhookHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		r.Body = http.MaxBytesReader(w, r.Body, 64<<10) // 64 KB

		var req CreateWebhookRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.URL == "" {
			http.Error(w, "url is required", http.StatusBadRequest)
			return
		}

		if len(req.Events) == 0 {
			http.Error(w, "at least one event is required", http.StatusBadRequest)
			return
		}

		validateWebhookURL(req.URL)

		if hasDuplicateEvents(req.Events) {
			http.Error(
				w,
				"duplicate event type",
				http.StatusBadRequest,
			)
			return
		}

		webhookID, secret, err := createWebhook(r.Context(), db, req)
		if err != nil {
			http.Error(w, "failed to create webhook", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		json.NewEncoder(w).Encode(CreateWebhookResponse{
			ID:     webhookID,
			Secret: secret,
		})
	}
}

func createEventHandler(
	db *pgxpool.Pool,
	queue *Queue,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// restrict request body to 1MB
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

		var req CreateEventRequest

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		if req.Type == "" {
			http.Error(w, "type is required", http.StatusBadRequest)
			return
		}

		if req.Payload == nil {
			http.Error(w, "payload is required", http.StatusBadRequest)
			return
		}

		eventID, deliveryCount, deliveries, err :=
			createEvent(r.Context(), db, req)

		if err != nil {
			http.Error(
				w,
				"failed to create event",
				http.StatusInternalServerError,
			)
			return
		}

		deliveryIDs := make([]string, 0, len(deliveries))

		for _, delivery := range deliveries {
			deliveryIDs = append(deliveryIDs, delivery.ID)

			if !queue.TryEnqueue(delivery) {
				http.Error(
					w,
					"delivery queue is full",
					http.StatusServiceUnavailable,
				)
				return
			}
		}

		response := map[string]any{
			"event_id":     eventID,
			"deliveries":   deliveryCount,
			"delivery_ids": deliveryIDs,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		json.NewEncoder(w).Encode(response)
	}
}

func getDeliveryHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deliveryID := r.PathValue("id")

		if deliveryID == "" {
			http.Error(w, "delivery id is required", http.StatusBadRequest)
			return
		}

		var delivery Delivery

		err := db.QueryRow(
			r.Context(),
			`
			SELECT
				id,
				event_id,
				webhook_id,
				status,
				attempts,
				created_at
			FROM deliveries
			WHERE id = $1
			`,
			deliveryID,
		).Scan(
			&delivery.ID,
			&delivery.EventID,
			&delivery.WebhookID,
			&delivery.Status,
			&delivery.Attempts,
			&delivery.CreatedAt,
		)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(w, "delivery not found", http.StatusNotFound)
				return
			}

			http.Error(
				w,
				"failed to get delivery",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(delivery); err != nil {
			http.Error(
				w,
				"failed to encode response",
				http.StatusInternalServerError,
			)
		}
	}
}

func getWebhooksHandler(
	db *pgxpool.Pool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhooks, err := getWebhooks(r.Context(), db)
		if err != nil {
			log.Printf("failed to get webhooks: %v", err)
			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(webhooks); err != nil {
			log.Printf("failed encoding webhooks: %v", err)
		}
	}
}

func getWebhookHandler(
	db *pgxpool.Pool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webhookID := r.PathValue("id")

		webhook, err := getWebhook(
			r.Context(),
			db,
			webhookID,
		)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				http.Error(
					w,
					"webhook not found",
					http.StatusNotFound,
				)
				return
			}

			log.Printf(
				"failed to get webhook=%s: %v",
				webhookID,
				err,
			)

			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(webhook); err != nil {
			log.Printf(
				"failed encoding webhook=%s: %v",
				webhookID,
				err,
			)
		}
	}
}

func getDeliveriesHandler(
	db *pgxpool.Pool,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deliveries, err := getDeliveries(
			r.Context(),
			db,
		)
		if err != nil {
			log.Printf("failed to get deliveries: %v", err)

			http.Error(
				w,
				"internal server error",
				http.StatusInternalServerError,
			)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := json.NewEncoder(w).Encode(deliveries); err != nil {
			log.Printf("failed encoding deliveries: %v", err)
		}
	}
}
