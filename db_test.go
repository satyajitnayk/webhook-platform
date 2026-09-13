package main

import (
	"context"
	"log"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

var testDB *pgxpool.Pool

func TestMain(m *testing.M) {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, relying on system environment variables")
	}

	if os.Getenv("TEST_DATABASE_URL") == "" {
		log.Fatal("TEST_DATABASE_URL environment variable is required to run tests")
	}

	ctx := context.Background()

	var err error
	testDB, err = connectTestDB(ctx)
	if err != nil {
		log.Fatalf("connect test db: %v", err)
	}
	defer testDB.Close()

	// Run the test suite once package setup is complete
	code := m.Run()
	os.Exit(code)
}

// Add this helper function to use at the top of your individual tests
func setupTest(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	if err := cleanDB(ctx); err != nil {
		t.Fatalf("failed to clean test db before test %s: %v", t.Name(), err)
	}

	t.Cleanup(func() {
		if err := cleanDB(ctx); err != nil {
			t.Logf("warning: failed to clean test db after test %s: %v", t.Name(), err)
		}
	})

	return testDB
}

func cleanDB(ctx context.Context) error {
	_, err := testDB.Exec(ctx, `
		TRUNCATE TABLE
			deliveries,
			subscriptions,
			events,
			webhooks
		RESTART IDENTITY CASCADE
	`)
	return err
}
