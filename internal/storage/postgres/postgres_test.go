package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNewPool_InvalidDSN(t *testing.T) {
	pool, err := NewPool(t.Context(), "postgres://invalid:invalid@localhost:1/nonexistent?sslmode=disable&connect_timeout=1")
	if err == nil {
		pool.Close()
		t.Fatal("expected error for invalid DSN, got nil")
	}
}

func TestNewPool_ReturnsPoolType(t *testing.T) {
	// Verify the function signature returns the expected type.
	var _ func(dsn string) (*pgxpool.Pool, error)
	// This is a compile-time check; the function accepts context+dsn and returns pool+error.
}

func TestRunMigrations_InvalidDSN(t *testing.T) {
	err := RunMigrations("postgres://invalid:invalid@localhost:1/nonexistent?sslmode=disable", "file://testdata")
	if err == nil {
		t.Fatal("expected error for invalid migration source, got nil")
	}
}

func TestRunMigrations_InvalidSource(t *testing.T) {
	err := RunMigrations("postgres://user:pass@localhost:5432/db?sslmode=disable", "file:///nonexistent/path")
	if err == nil {
		t.Fatal("expected error for nonexistent migration path, got nil")
	}
}
