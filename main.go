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

	apiKey := os.Getenv("WEBHOOK_API_KEY")

	if apiKey == "" {
		log.Fatal("WEBHOOK_API_KEY is required")
	}

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

	privateMux := http.NewServeMux()

	privateMux.Handle(
		"POST /webhooks",
		createWebhookHandler(db),
	)

	privateMux.Handle(
		"POST /events",
		createEventHandler(db, queue),
	)

	privateMux.Handle(
		"GET /deliveries/{id}",
		getDeliveryHandler(db),
	)

	authProvider := authMiddleware(apiKey)
	protectedHandler := authProvider(privateMux)

	globalMux := http.NewServeMux()

	globalMux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	globalMux.Handle("/metrics", promhttp.Handler())

	// Fallback/Catch-all route passes everything else to the protected handler
	globalMux.Handle("/", protectedHandler)

	server := &http.Server{
		Addr:    ":8080",
		Handler: globalMux,
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
