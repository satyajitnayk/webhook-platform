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

	"github.com/joho/godotenv"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	godotenv.Load()

	initMetrics()

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

	workerPool := NewWorkerPool(
		3,
		queue,
		db,
	)

	workerPool.Start(ctx)

	go startRetryScheduler(
		ctx,
		db,
		queue,
	)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.Handle(
		"POST /webhooks",
		authMiddleware(http.HandlerFunc(createWebhookHandler(db))),
	)

	mux.Handle(
		"POST /events",
		authMiddleware(http.HandlerFunc(createEventHandler(db, queue))),
	)

	mux.Handle(
		"GET /deliveries/{id}",
		authMiddleware(http.HandlerFunc(getDeliveryHandler(db))),
	)

	mux.Handle("/metrics", promhttp.Handler())

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

	// Stop accepting new requests and wait for
	// in-flight requests to finish.
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

	log.Println("waiting for workers to finish")

	// Workers drain remaining jobs and exit.
	workerPool.Wait()

	log.Println("shutdown complete")
}
