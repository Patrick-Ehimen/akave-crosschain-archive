package indexer

import (
	"context"
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/testhelper"
)

func TestPostgresCursorStore_LoadCursor_NotExists(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	store := NewPostgresCursorStore(pool)

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
	pool := testhelper.NewTestPool(t)
	store := NewPostgresCursorStore(pool)

	chainID := uint64(42161)
	protocol := "layerzero_v2"
	blockNumber := uint64(12345678)

	err := store.UpdateCursor(ctx, chainID, protocol, blockNumber)
	if err != nil {
		t.Fatalf("UpdateCursor failed: %v", err)
	}

	cursor, err := store.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}
	if cursor != blockNumber {
		t.Errorf("expected cursor %d, got %d", blockNumber, cursor)
	}

	newBlockNumber := uint64(12345700)
	err = store.UpdateCursor(ctx, chainID, protocol, newBlockNumber)
	if err != nil {
		t.Fatalf("UpdateCursor failed: %v", err)
	}

	cursor, err = store.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		t.Fatalf("LoadCursor failed: %v", err)
	}
	if cursor != newBlockNumber {
		t.Errorf("expected cursor %d, got %d", newBlockNumber, cursor)
	}

	pool.Exec(ctx, "DELETE FROM indexer_cursors WHERE chain_id = $1 AND protocol = $2", chainID, protocol)
}

func TestPostgresCursorStore_MultipleChains(t *testing.T) {
	ctx := context.Background()
	pool := testhelper.NewTestPool(t)
	store := NewPostgresCursorStore(pool)

	testCases := []struct {
		chainID  uint64
		protocol string
		block    uint64
	}{
		{1, "layerzero_v2", 19000100},
		{42161, "layerzero_v2", 175000200},
		{10, "wormhole", 123000000},
	}

	for _, tc := range testCases {
		if err := store.UpdateCursor(ctx, tc.chainID, tc.protocol, tc.block); err != nil {
			t.Fatalf("UpdateCursor failed for chain %d protocol %s: %v", tc.chainID, tc.protocol, err)
		}
	}

	for _, tc := range testCases {
		cursor, err := store.LoadCursor(ctx, tc.chainID, tc.protocol)
		if err != nil {
			t.Fatalf("LoadCursor failed for chain %d protocol %s: %v", tc.chainID, tc.protocol, err)
		}
		if cursor != tc.block {
			t.Errorf("chain %d protocol %s: expected %d, got %d", tc.chainID, tc.protocol, tc.block, cursor)
		}
	}

	for _, tc := range testCases {
		pool.Exec(ctx, "DELETE FROM indexer_cursors WHERE chain_id = $1 AND protocol = $2", tc.chainID, tc.protocol)
	}
}