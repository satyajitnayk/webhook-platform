package main

import (
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
)

func createWebhookHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		id, err := createWebhook(r.Context(), db, req)
		if err != nil {
			http.Error(w, "failed to create webhook", http.StatusInternalServerError)
			return
		}

		response := map[string]string{
			"id": id,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		json.NewEncoder(w).Encode(response)
	}
}

func createEventHandler(
	db *pgxpool.Pool,
	queue *Queue,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

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

		for _, delivery := range deliveries {
			queue.Enqueue(delivery)
		}

		response := map[string]any{
			"event_id":   eventID,
			"deliveries": deliveryCount,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)

		json.NewEncoder(w).Encode(response)
	}
}
