package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"github.com/joho/godotenv"
)

var requestCount atomic.Int32

func generateWebhookSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func main() {
	godotenv.Load()

	mux := http.NewServeMux()

	mux.HandleFunc("POST /webhook", func(w http.ResponseWriter, r *http.Request) {
		log.Println("🔥 SUBSCRIBER REQUEST RECEIVED")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		signature := r.Header.Get("X-Webhook-Signature")

		webhookSecret := os.Getenv("WEBHOOK_SECRET")

		expected := generateWebhookSignature(webhookSecret, body)

		if !hmac.Equal(
			[]byte(signature),
			[]byte("sha256="+expected),
		) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}

		count := requestCount.Add(1)

		log.Printf(
			"received request=%d body=%s",
			count,
			string(body),
		)

		// Generate a NEW random outcome for EVERY request.
		outcome := rand.Intn(100)

		log.Printf(
			"request=%d random_number=%d",
			count,
			outcome,
		)

		switch {
		case outcome < 50:
			log.Printf(
				"request=%d outcome=success returning=200",
				count,
			)

			w.WriteHeader(http.StatusOK)

		case outcome < 70:
			log.Printf(
				"request=%d outcome=temporary_failure returning=500",
				count,
			)

			http.Error(
				w,
				"temporary failure",
				http.StatusInternalServerError,
			)

		case outcome < 85:
			log.Printf(
				"request=%d outcome=permanent_failure returning=400",
				count,
			)

			http.Error(
				w,
				"invalid payload",
				http.StatusBadRequest,
			)

		default:
			log.Printf(
				"request=%d outcome=timeout sleeping=6s",
				count,
			)

			time.Sleep(6 * time.Second)

			log.Printf(
				"request=%d timeout sleep finished",
				count,
			)

			w.WriteHeader(http.StatusOK)
		}
	})

	log.Println("subscriber running on :9000")

	log.Fatal(
		http.ListenAndServe(":9000", mux),
	)
}
