package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

func setupTestRepo(t *testing.T) (*MessageRepository, func()) {
	t.Helper()
	ctx := context.Background()

	pool, err := NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}

	// Run migrations
	err = RunMigrations(
		"postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable",
		"file://../../../migrations",
	)
	if err != nil {
		t.Skip("Skipping test: migrations failed:", err)
	}

	repo := NewMessageRepository(pool)

	cleanup := func() {
		// Clean up test data
		pool.Exec(ctx, "DELETE FROM message_metadata")
		pool.Exec(ctx, "DELETE FROM message_payloads")
		pool.Exec(ctx, "DELETE FROM message_destinations")
		pool.Exec(ctx, "DELETE FROM message_sources")
		pool.Exec(ctx, "DELETE FROM messages")
		pool.Close()
	}

	return repo, cleanup
}

func TestMessageRepository_UpsertAndGet(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	msg := &types.Message{
		MessageID: "test-guid-001",
		Protocol:  "layerzero_v2",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xabc123",
			BlockNumber: 19000000,
			Timestamp:   1700000000,
			Sender:      "0x1111111111111111111111111111111111111111",
			LogIndex:    3,
		},
		Payload: &types.Payload{
			Data:  "0xdeadbeef",
			Nonce: 42,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Upsert
	err := repo.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	// Get
	got, err := repo.GetMessage(ctx, "test-guid-001")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}

	if got.MessageID != msg.MessageID {
		t.Errorf("expected message ID %s, got %s", msg.MessageID, got.MessageID)
	}
	if got.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", got.Status)
	}
	if got.Source.ChainID != 1 {
		t.Errorf("expected source chain ID 1, got %d", got.Source.ChainID)
	}
	if got.Payload == nil || got.Payload.Nonce != 42 {
		t.Error("expected payload with nonce 42")
	}
}

func TestMessageRepository_UpdateDestination(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create pending message
	msg := &types.Message{
		MessageID: "test-guid-002",
		Protocol:  "layerzero_v2",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xsrc",
			BlockNumber: 19000000,
			Timestamp:   1700000000,
			Sender:      "0x1111111111111111111111111111111111111111",
			LogIndex:    0,
		},
		Payload: &types.Payload{
			Nonce: 99,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := repo.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	// Update with destination (simulates PacketReceived correlation)
	dst := &types.Destination{
		ChainID:     56,
		TxHash:      "0xdst",
		BlockNumber: 35000000,
		Timestamp:   1700000120,
		Receiver:    "0x2222222222222222222222222222222222222222",
		LogIndex:    1,
	}

	latency := int64(120) // 120 seconds
	err = repo.UpdateDestination(ctx, "test-guid-002", dst, latency)
	if err != nil {
		t.Fatalf("UpdateDestination failed: %v", err)
	}

	// Verify message is now executed with destination
	got, err := repo.GetMessage(ctx, "test-guid-002")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}

	if got.Status != types.StatusExecuted {
		t.Errorf("expected status executed, got %s", got.Status)
	}
	if got.Destination == nil {
		t.Fatal("expected destination to be set")
	}
	if got.Destination.ChainID != 56 {
		t.Errorf("expected dst chain ID 56, got %d", got.Destination.ChainID)
	}
	if got.Destination.TxHash != "0xdst" {
		t.Errorf("expected dst tx hash 0xdst, got %s", got.Destination.TxHash)
	}
	if got.Metadata == nil || got.Metadata.LatencySeconds != 120 {
		t.Error("expected metadata with latency 120")
	}
}

func TestMessageRepository_ListByProtocol(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	// Create 3 messages
	for i, id := range []string{"list-001", "list-002", "list-003"} {
		msg := &types.Message{
			MessageID: id,
			Protocol:  "layerzero_v2",
			Type:      types.TypeMessage,
			Status:    types.StatusPending,
			Source: types.Source{
				ChainID:     1,
				TxHash:      "0x" + id,
				BlockNumber: uint64(19000000 + i),
				Timestamp:   int64(1700000000 + i),
				Sender:      "0x1111111111111111111111111111111111111111",
				LogIndex:    0,
			},
			CreatedAt: now.Add(time.Duration(i) * time.Second),
			UpdatedAt: now.Add(time.Duration(i) * time.Second),
		}
		if err := repo.UpsertMessage(ctx, msg); err != nil {
			t.Fatalf("UpsertMessage failed for %s: %v", id, err)
		}
	}

	// List
	messages, err := repo.ListByProtocol(ctx, "layerzero_v2", 10)
	if err != nil {
		t.Fatalf("ListByProtocol failed: %v", err)
	}
	if len(messages) < 3 {
		t.Errorf("expected at least 3 messages, got %d", len(messages))
	}
}

func TestMessageRepository_UpsertIdempotent(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	msg := &types.Message{
		MessageID: "test-guid-idem",
		Protocol:  "layerzero_v2",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xidem",
			BlockNumber: 19000000,
			Timestamp:   1700000000,
			Sender:      "0x1111111111111111111111111111111111111111",
			LogIndex:    0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Upsert twice — should not error
	err := repo.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("First UpsertMessage failed: %v", err)
	}

	err = repo.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("Second UpsertMessage (idempotent) failed: %v", err)
	}
}

func TestMessageRepository_DeleteMessage(t *testing.T) {
	repo, cleanup := setupTestRepo(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	msg := &types.Message{
		MessageID: "test-guid-del",
		Protocol:  "layerzero_v2",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xdel",
			BlockNumber: 19000000,
			Timestamp:   1700000000,
			Sender:      "0x1111111111111111111111111111111111111111",
			LogIndex:    0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := repo.UpsertMessage(ctx, msg)
	if err != nil {
		t.Fatalf("UpsertMessage failed: %v", err)
	}

	err = repo.DeleteMessage(ctx, "test-guid-del")
	if err != nil {
		t.Fatalf("DeleteMessage failed: %v", err)
	}

	// Should fail to get deleted message
	_, err = repo.GetMessage(ctx, "test-guid-del")
	if err == nil {
		t.Error("expected error getting deleted message")
	}
}
