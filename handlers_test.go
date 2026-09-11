package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateEventBodyTooLarge(t *testing.T) {
	q := NewQueue(10)
	defer q.Close()

	payload := strings.Repeat("a", 1<<20)

	body := `{
		"event_type": "order.created",
		"payload": "` + payload + `"
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/events",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	handler := createEventHandler(testDB, q)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}
}

func TestCreateWebhookBodyTooLarge(t *testing.T) {
	largeURL := "https://example.com/" + strings.Repeat("a", 64<<10)

	body := `{
		"url": "` + largeURL + `",
		"events": ["order.created"]
	}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/webhooks",
		strings.NewReader(body),
	)

	rec := httptest.NewRecorder()

	handler := createWebhookHandler(testDB)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf(
			"expected status %d, got %d",
			http.StatusBadRequest,
			rec.Code,
		)
	}
}
