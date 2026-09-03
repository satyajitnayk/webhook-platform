package main

import (
	"context"
	"log"
	"net/http"
)

func main() {
	ctx := context.Background()

	db, err := connectDB(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	queue := NewQueue(1000)

	for i := 1; i <= 3; i++ {
		worker := NewWorker(i, queue, db)

		go worker.Start(ctx)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc(
		"POST /webhooks",
		createWebhookHandler(db),
	)

	mux.HandleFunc(
		"POST /events",
		createEventHandler(db, queue),
	)

	log.Println("server running on :8080")

	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
