package indexer

import (
	"context"
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
)

func TestPostgresCursorStore_LoadCursor_NotExists(t *testing.T) {
	ctx := context.Background()

	// Create test database connection
	pool, err := postgres.NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}
	defer pool.Close()

	store := NewPostgresCursorStore(pool)

	// Load cursor that doesn't exist
	cursor, err := store.LoadCursor(ctx, 1, "test_protocol")
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	if cursor != 0 {
		t.Errorf("expected cursor 0 for non-existent entry, got %d", cursor)
	}
}

func TestPostgresCursorStore_UpdateAndLoad(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}
	defer pool.Close()

	store := NewPostgresCursorStore(pool)

	chainID := uint64(42161)
	protocol := "layerzero_v2"
	blockNumber := uint64(12345678)

	// Update cursor
	err = store.UpdateCursor(ctx, chainID, protocol, blockNumber)
	if err != nil {
		t.Fatalf("UpdateCursor failed: %v", err)
	}

	// Load cursor back
	cursor, err := store.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	if cursor != blockNumber {
		t.Errorf("expected cursor %d, got %d", blockNumber, cursor)
	}

	// Update to new block
	newBlockNumber := uint64(12345700)
	err = store.UpdateCursor(ctx, chainID, protocol, newBlockNumber)
	if err != nil {
		t.Fatalf("UpdateCursor failed: %v", err)
	}

	// Verify update
	cursor, err = store.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}

	if cursor != newBlockNumber {
		t.Errorf("expected cursor %d, got %d", newBlockNumber, cursor)
	}

	// Cleanup
	_, err = pool.Exec(ctx, "DELETE FROM indexer_cursors WHERE chain_id = $1 AND protocol = $2", chainID, protocol)
	if err != nil {
		t.Logf("Cleanup failed: %v", err)
	}
}

func TestPostgresCursorStore_MultipleChains(t *testing.T) {
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}
	defer pool.Close()

	store := NewPostgresCursorStore(pool)

	// Set up test data
	testCases := []struct {
		chainID uint64
		protocol string
		block   uint64
	}{
		{1, "layerzero_v2", 19000100},
		{42161, "layerzero_v2", 175000200},
		{10, "wormhole", 123000000},
	}

	// Update all cursors
	for _, tc := range testCases {
		err := store.UpdateCursor(ctx, tc.chainID, tc.protocol, tc.block)
		if err != nil {
			t.Fatalf("UpdateCursor failed for chain %d protocol %s: %v", tc.chainID, tc.protocol, err)
		}
	}

	// Verify all cursors
	for _, tc := range testCases {
		cursor, err := store.LoadCursor(ctx, tc.chainID, tc.protocol)
		if err != nil {
			t.Fatalf("LoadCursor failed for chain %d protocol %s: %v", tc.chainID, tc.protocol, err)
		}

		if cursor != tc.block {
			t.Errorf("chain %d protocol %s: expected cursor %d, got %d", tc.chainID, tc.protocol, tc.block, cursor)
		}
	}

	// Cleanup
	for _, tc := range testCases {
		_, err := pool.Exec(ctx, "DELETE FROM indexer_cursors WHERE chain_id = $1 AND protocol = $2", tc.chainID, tc.protocol)
		if err != nil {
			t.Logf("Cleanup failed for chain %d protocol %s: %v", tc.chainID, tc.protocol, err)
		}
	}
}
