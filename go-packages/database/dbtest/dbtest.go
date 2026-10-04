// Package dbtest gives integration tests a real database or a skip.
package dbtest

import (
	"context"
	"os"
	"testing"

	"github.com/bernardoforcillo/ignition/go-packages/database"
)

// Open returns a DB for TEST_DATABASE_URL, closed on test cleanup. The test
// is skipped when the variable is unset so `go test` works with no database.
func Open(t testing.TB) *database.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	db, err := database.Open(context.Background(), database.Config{DSN: dsn})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
