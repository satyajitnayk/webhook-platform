package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

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

	mux.HandleFunc(
		"GET /deliveries/{id}",
		getDeliveryHandler(db),
	)

	server := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	// Start HTTP server
	go func() {
		log.Println("server running on :8080")

		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {

			log.Fatalf("server failed: %v", err)
		}
	}()

	// Wait for Ctrl+C / SIGTERM
	<-ctx.Done()

	log.Println("shutdown started")

	// Give HTTP requests time to finish
	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	// Stop HTTP server
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	// stop workers
	queue.Close()

	log.Println("shutdown complete")

}
