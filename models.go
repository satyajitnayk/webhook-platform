package main

import "time"

type Webhook struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	ID        string         `json:"id"`
	EventType string         `json:"event_type"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"created_at"`
}

type Subscription struct {
	ID        string `json:"id"`
	WebhookID string `json:"webhook_id"`
	EventType string `json:"event_type"`
}

type Delivery struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	WebhookID string    `json:"webhook_id"`
	Status    string    `json:"status"`
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
}

type CreateWebhookRequest struct {
	URL    string   `json:"url"`
	Events []string `json:"events"`
}

type CreateEventRequest struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload"`
}

const (
	DeliveryPending    = "pending"
	DeliveryProcessing = "processing"
	DeliverySuccess    = "success"
	DeliveryFailed     = "failed"

	MaxAttempts = 4
)
