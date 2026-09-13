# Webhook Delivery Platform

A minimal webhook delivery platform built with Go and PostgreSQL.

It accepts events, finds subscribed webhooks, queues deliveries, and delivers them asynchronously with retries, HMAC signatures, SSRF protection, and basic observability.

## Features

- Webhook registration
- Event subscriptions
- Asynchronous delivery using Go workers
- In-memory job queue
- Retry with exponential backoff and jitter
- Durable delivery state in PostgreSQL
- Delivery lease and stuck-delivery recovery
- HMAC-SHA256 webhook signatures
- SSRF protection for webhook URLs
- API key authentication
- Structured delivery logs
- Prometheus metrics
- Minimal web UI
- Demo subscriber for testing failures and retries

## Architecture

```text
                    ┌─────────────────┐
                    │    Web UI       │
                    └────────┬────────┘
                             │
                             ▼
                    ┌─────────────────┐
                    │    Go API       │
                    └────────┬────────┘
                             │
              ┌──────────────┴──────────────┐
              ▼                             ▼
       ┌─────────────┐               ┌─────────────┐
       │ PostgreSQL  │               │ In-Memory   │
       │             │               │ Queue       │
       └─────────────┘               └──────┬──────┘
                                            │
                                            ▼
                                     ┌─────────────┐
                                     │   Workers   │
                                     └──────┬──────┘
                                            │
                                            │ HTTP POST
                                            ▼
                                     ┌─────────────┐
                                     │  Subscriber │
                                     └─────────────┘
```

## Delivery Flow

```text
Create Webhook
      ↓
Create Event
      ↓
Find Subscribers
      ↓
Create Deliveries in PostgreSQL
      ↓
Queue
      ↓
Worker
      ↓
HTTP POST + HMAC Signature
      ↓
Subscriber
      ↓
Success / Retry / Failure
```

## Requirements

- Go 1.22+
- PostgreSQL

## Setup

Clone the repository:

```bash
git clone <repository-url>
cd webhook-platform
```

Create a PostgreSQL database:

```sql
CREATE DATABASE webhooks;
```

Create `.env`:

```env
WEBHOOK_API_KEY=my-demo-key
```

Update the database connection if required.

Run the migrations:

```bash
psql postgres://postgres:postgres@localhost:5432/webhooks \
  -f migrations/001_init.sql
```

## Run

Start the webhook platform:

```bash
go run .
```

The platform runs on:

```text
http://localhost:8080
```

Open the UI:

```text
http://localhost:8080
```

Start the demo subscriber in another terminal:

```bash
go run ./subscriber
```

The subscriber runs on:

```text
http://localhost:9000
```

## Demo Subscriber

The subscriber verifies the `X-Webhook-Signature` header and randomly returns:

```text
200 → successful delivery
500 → retry
400 → permanent failure
timeout → retry
```

This makes it easy to demonstrate the delivery and retry behavior.

## API

### Create Webhook

```http
POST /webhooks
X-API-Key: my-demo-key
Content-Type: application/json
```

Example:

```json
{
  "url": "http://localhost:9000/webhook",
  "events": ["order.created", "order.updated"]
}
```

The response contains the generated webhook secret.

### Create Event

```http
POST /events
X-API-Key: my-demo-key
Content-Type: application/json
```

Example:

```json
{
  "event_type": "order.created",
  "payload": {
    "order_id": "123"
  }
}
```

### List Webhooks

```http
GET /webhooks
X-API-Key: my-demo-key
```

### Get Webhook

```http
GET /webhooks/{id}
X-API-Key: my-demo-key
```

### List Deliveries

```http
GET /deliveries
X-API-Key: my-demo-key
```

### Get Delivery

```http
GET /deliveries/{id}
X-API-Key: my-demo-key
```

### Health

```http
GET /health
```

### Metrics

```http
GET /metrics
```

## Webhook Signature

Every delivery contains:

```text
X-Webhook-Signature: sha256=<signature>
```

The signature is generated using HMAC-SHA256:

```text
HMAC-SHA256(webhook_secret, request_body)
```

The subscriber verifies the signature before processing the event.

## Retry Behavior

Temporary failures such as:

- HTTP 5xx
- network errors
- timeouts

are retried with exponential backoff and jitter.

Permanent failures such as HTTP 4xx are not retried.

A delivery can make up to 4 attempts.

## Tests

Run all tests:

```bash
go test ./...
```

Run with the race detector:

```bash
go test -race ./...
```

## Local Demo

The demo subscriber runs on `localhost:9000`.

For local development, enable:

```env
ALLOW_LOCAL_WEBHOOKS=true
```
