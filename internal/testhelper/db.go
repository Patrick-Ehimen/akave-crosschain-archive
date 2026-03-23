package testhelper

import (
	"os"
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
	"github.com/jackc/pgx/v5/pgxpool"
	"context"
)

// TestDSN returns the PostgreSQL DSN for tests.
// It reads from the TEST_DATABASE_URL environment variable,
// falling back to the local docker-compose default.
func TestDSN() string {
	if dsn := os.Getenv("TEST_DATABASE_URL"); dsn != "" {
		return dsn
	}
	return "postgres://crosschain:crosschain@localhost:5432/crosschain_test?sslmode=disable"
}

// NewTestPool creates a pgxpool for tests, skipping if the DB is unavailable.
func NewTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := postgres.NewPool(context.Background(), TestDSN())
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}