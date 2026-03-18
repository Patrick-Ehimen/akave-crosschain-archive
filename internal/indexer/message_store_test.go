package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

const testDSN = "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable"

func newTestMessageStore(t *testing.T) *PostgresMessageStore {
	t.Helper()
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, testDSN)
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewPostgresMessageStore(pool)
}

func cleanupMessage(t *testing.T, store *PostgresMessageStore, messageID string) {
	t.Helper()
	ctx := context.Background()
	// Delete in order respecting foreign keys (CASCADE handles this, but be explicit)
	store.pool.Exec(ctx, "DELETE FROM messages WHERE message_id = $1", messageID)
}

func newTestMessage(messageID string) *types.Message {
	now := time.Now()
	return &types.Message{
		MessageID: messageID,
		Protocol:  "layerzero_v2",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xabc123",
			BlockNumber: 19000000,
			Timestamp:   1706000000,
			Sender:      "0xSender1111111111111111111111111111111111",
			LogIndex:    5,
		},
		Payload: &types.Payload{
			Data:  "0xdeadbeef",
			Nonce: 42,
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func TestPostgresMessageStore_UpsertMessage(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-upsert-0x1234"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	msg := newTestMessage(msgID)
	err := store.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	// Verify message exists
	var status string
	err = store.pool.QueryRow(ctx, "SELECT status FROM messages WHERE message_id = $1", msgID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query message: %v", err)
	}
	if status != "pending" {
		t.Errorf("expected status pending, got %s", status)
	}

	// Verify source exists
	var chainID int64
	err = store.pool.QueryRow(ctx, "SELECT chain_id FROM message_sources WHERE message_id = $1", msgID).Scan(&chainID)
	if err != nil {
		t.Fatalf("failed to query source: %v", err)
	}
	if chainID != 1 {
		t.Errorf("expected chain_id 1, got %d", chainID)
	}

	// Verify payload exists
	var nonce int64
	err = store.pool.QueryRow(ctx, "SELECT nonce FROM message_payloads WHERE message_id = $1", msgID).Scan(&nonce)
	if err != nil {
		t.Fatalf("failed to query payload: %v", err)
	}
	if nonce != 42 {
		t.Errorf("expected nonce 42, got %d", nonce)
	}
}

func TestPostgresMessageStore_UpsertMessage_Idempotent(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-idempotent-0x5678"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	msg := newTestMessage(msgID)

	// Insert twice
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("first UpsertMessage failed: %v", err)
	}
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("second UpsertMessage failed: %v", err)
	}

	// Should still be one row
	var count int
	err := store.pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE message_id = $1", msgID).Scan(&count)
	if err != nil {
		t.Fatalf("failed to count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 message, got %d", count)
	}
}

func TestPostgresMessageStore_UpsertMessage_OFTUpgradesType(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-type-upgrade-0xabcd"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	// First insert as message type (from PacketSent)
	msg := newTestMessage(msgID)
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("first UpsertMessage failed: %v", err)
	}

	// Second insert as token_transfer type (from OFTSent)
	msg.Type = types.TypeTokenTransfer
	msg.Payload.Amount = "1000000"
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("second UpsertMessage failed: %v", err)
	}

	// Type should be upgraded to token_transfer
	var msgType string
	err := store.pool.QueryRow(ctx, "SELECT type FROM messages WHERE message_id = $1", msgID).Scan(&msgType)
	if err != nil {
		t.Fatalf("failed to query type: %v", err)
	}
	if msgType != "token_transfer" {
		t.Errorf("expected type token_transfer, got %s", msgType)
	}
}

func TestPostgresMessageStore_FindByCorrelationKey_Found(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-find-0x9999"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	msg := newTestMessage(msgID)
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	key := &normalizer.CorrelationKey{
		Protocol:   "layerzero_v2",
		Nonce:      42,
		Sender:     "0xSender1111111111111111111111111111111111",
		SrcChainID: 1,
	}

	found, err := store.FindByCorrelationKey(ctx, key)
	if err != nil {
		t.Fatalf("FindByCorrelationKey failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find message, got nil")
	}
	if found.MessageID != msgID {
		t.Errorf("expected message_id %s, got %s", msgID, found.MessageID)
	}
}

func TestPostgresMessageStore_FindByCorrelationKey_NotFound(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()

	key := &normalizer.CorrelationKey{
		Protocol:   "layerzero_v2",
		Nonce:      999999,
		Sender:     "0xNonExistent",
		SrcChainID: 1,
	}

	found, err := store.FindByCorrelationKey(ctx, key)
	if err != nil {
		t.Fatalf("FindByCorrelationKey failed: %v", err)
	}
	if found != nil {
		t.Errorf("expected nil, got message %s", found.MessageID)
	}
}

func TestPostgresMessageStore_FindByCorrelationKey_OnlyPending(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-only-pending-0xaaaa"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	msg := newTestMessage(msgID)
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	// Mark as executed
	_, err := store.pool.Exec(ctx, "UPDATE messages SET status = 'executed' WHERE message_id = $1", msgID)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	key := &normalizer.CorrelationKey{
		Protocol:   "layerzero_v2",
		Nonce:      42,
		Sender:     "0xSender1111111111111111111111111111111111",
		SrcChainID: 1,
	}

	found, err := store.FindByCorrelationKey(ctx, key)
	if err != nil {
		t.Fatalf("FindByCorrelationKey failed: %v", err)
	}
	if found != nil {
		t.Error("expected nil for executed message, but found one")
	}
}

func TestPostgresMessageStore_UpdateDestination(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-update-dest-0xbbbb"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	msg := newTestMessage(msgID)
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	dest := &types.Destination{
		ChainID:     42161,
		TxHash:      "0xdef456",
		BlockNumber: 180000000,
		Timestamp:   1706000045,
		Receiver:    "0xReceiver2222222222222222222222222222222222",
		LogIndex:    3,
	}

	err := store.UpdateDestination(ctx, msgID, dest, types.StatusExecuted)
	if err != nil {
		t.Fatalf("UpdateDestination failed: %v", err)
	}

	// Verify status updated
	var status string
	err = store.pool.QueryRow(ctx, "SELECT status FROM messages WHERE message_id = $1", msgID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query status: %v", err)
	}
	if status != "executed" {
		t.Errorf("expected status executed, got %s", status)
	}

	// Verify destination exists
	var destChainID int64
	err = store.pool.QueryRow(ctx, "SELECT chain_id FROM message_destinations WHERE message_id = $1", msgID).Scan(&destChainID)
	if err != nil {
		t.Fatalf("failed to query destination: %v", err)
	}
	if destChainID != 42161 {
		t.Errorf("expected dest chain_id 42161, got %d", destChainID)
	}

	// Verify latency computed (45 seconds = 1706000045 - 1706000000)
	var latency int64
	err = store.pool.QueryRow(ctx, "SELECT latency_seconds FROM message_metadata WHERE message_id = $1", msgID).Scan(&latency)
	if err != nil {
		t.Fatalf("failed to query latency: %v", err)
	}
	if latency != 45 {
		t.Errorf("expected latency 45, got %d", latency)
	}
}

func TestPostgresMessageStore_FullCorrelationFlow(t *testing.T) {
	store := newTestMessageStore(t)
	ctx := context.Background()
	msgID := "test-full-flow-0xcccc"
	t.Cleanup(func() { cleanupMessage(t, store, msgID) })

	// Step 1: PacketSent creates pending message
	msg := newTestMessage(msgID)
	if err := store.UpsertMessage(ctx, msg); err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	// Step 2: Find by correlation key
	key := &normalizer.CorrelationKey{
		Protocol:   "layerzero_v2",
		Nonce:      42,
		Sender:     "0xSender1111111111111111111111111111111111",
		SrcChainID: 1,
	}
	found, err := store.FindByCorrelationKey(ctx, key)
	if err != nil {
		t.Fatalf("FindByCorrelationKey failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find pending message")
	}

	// Step 3: PacketReceived updates destination
	dest := &types.Destination{
		ChainID:     42161,
		TxHash:      "0xdef456",
		BlockNumber: 180000000,
		Timestamp:   1706000045,
		Receiver:    "0xReceiver",
		LogIndex:    3,
	}
	if err := store.UpdateDestination(ctx, found.MessageID, dest, types.StatusExecuted); err != nil {
		t.Fatalf("UpdateDestination failed: %v", err)
	}

	// Step 4: Verify final state
	var status string
	err = store.pool.QueryRow(ctx, "SELECT status FROM messages WHERE message_id = $1", msgID).Scan(&status)
	if err != nil {
		t.Fatalf("failed to query final status: %v", err)
	}
	if status != "executed" {
		t.Errorf("expected final status executed, got %s", status)
	}

	// Verify can no longer find by correlation key (no longer pending)
	found, err = store.FindByCorrelationKey(ctx, key)
	if err != nil {
		t.Fatalf("second FindByCorrelationKey failed: %v", err)
	}
	if found != nil {
		t.Error("expected nil after message is executed")
	}
}
