package correlator

import (
	"context"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

func setupTestCorrelator(t *testing.T) (*Correlator, *postgres.MessageRepository, func()) {
	t.Helper()
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}

	err = postgres.RunMigrations(
		"postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable",
		"file://../../migrations",
	)
	if err != nil {
		t.Skip("Skipping test: migrations failed:", err)
	}

	repo := postgres.NewMessageRepository(pool)
	log := zerolog.Nop()
	c := New(repo, log)

	cleanup := func() {
		pool.Exec(ctx, "DELETE FROM message_metadata")
		pool.Exec(ctx, "DELETE FROM message_payloads")
		pool.Exec(ctx, "DELETE FROM message_destinations")
		pool.Exec(ctx, "DELETE FROM message_sources")
		pool.Exec(ctx, "DELETE FROM messages")
		pool.Close()
	}

	return c, repo, cleanup
}

func TestCorrelator_PacketSent_CreatesPendingMessage(t *testing.T) {
	c, repo, cleanup := setupTestCorrelator(t)
	defer cleanup()

	ctx := context.Background()

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xcorr_src",
		LogIndex:    0,
		Timestamp:   1700000000,
		EventType:   decoder.EventPacketSent,
		Data: map[string]string{
			"version":      "1",
			"nonce":        "100",
			"src_eid":      "30101",
			"src_chain_id": "1",
			"dst_eid":      "30102",
			"dst_chain_id": "56",
			"sender":       "0x1111111111111111111111111111111111111111",
			"receiver":     "0x2222222222222222222222222222222222222222",
			"guid":         "0xaaaa",
			"message":      "0x",
		},
	}

	err := c.ProcessEvent(ctx, event)
	if err != nil {
		t.Fatalf("ProcessEvent failed: %v", err)
	}

	// Verify message was created as pending
	msg, err := repo.GetMessage(ctx, "0xaaaa")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}

	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
	if msg.Source.ChainID != 1 {
		t.Errorf("expected source chain ID 1, got %d", msg.Source.ChainID)
	}
}

func TestCorrelator_PacketReceivedCorrelatesWithSent(t *testing.T) {
	c, repo, cleanup := setupTestCorrelator(t)
	defer cleanup()

	ctx := context.Background()

	// First: PacketSent creates a pending message
	sentEvent := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xcorr_src2",
		LogIndex:    0,
		Timestamp:   1700000000,
		EventType:   decoder.EventPacketSent,
		Data: map[string]string{
			"version":      "1",
			"nonce":        "200",
			"src_eid":      "30101",
			"src_chain_id": "1",
			"dst_eid":      "30102",
			"dst_chain_id": "56",
			"sender":       "0x1111111111111111111111111111111111111111",
			"receiver":     "0x2222222222222222222222222222222222222222",
			"guid":         "0xbbbb",
			"message":      "0x",
		},
	}

	err := c.ProcessEvent(ctx, sentEvent)
	if err != nil {
		t.Fatalf("ProcessEvent (PacketSent) failed: %v", err)
	}

	// Verify pending
	msg, err := repo.GetMessage(ctx, "0xbbbb")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}
	if msg.Status != types.StatusPending {
		t.Fatalf("expected pending, got %s", msg.Status)
	}

	// Second: PacketReceived correlates and updates to executed
	receivedEvent := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     56,
		BlockNumber: 35000000,
		TxHash:      "0xcorr_dst2",
		LogIndex:    1,
		Timestamp:   1700000120,
		EventType:   decoder.EventPacketReceived,
		Data: map[string]string{
			"src_eid":      "30101",
			"src_chain_id": "1",
			"sender":       "0x1111111111111111111111111111111111111111",
			"nonce":        "200",
			"receiver":     "0x2222222222222222222222222222222222222222",
		},
	}

	err = c.ProcessEvent(ctx, receivedEvent)
	if err != nil {
		t.Fatalf("ProcessEvent (PacketReceived) failed: %v", err)
	}

	// Verify message is now executed with destination
	msg, err = repo.GetMessage(ctx, "0xbbbb")
	if err != nil {
		t.Fatalf("GetMessage after correlation failed: %v", err)
	}

	if msg.Status != types.StatusExecuted {
		t.Errorf("expected status executed, got %s", msg.Status)
	}
	if msg.Destination == nil {
		t.Fatal("expected destination to be set")
	}
	if msg.Destination.ChainID != 56 {
		t.Errorf("expected dst chain ID 56, got %d", msg.Destination.ChainID)
	}
	if msg.Destination.TxHash != "0xcorr_dst2" {
		t.Errorf("expected dst tx hash 0xcorr_dst2, got %s", msg.Destination.TxHash)
	}
	if msg.Metadata == nil || msg.Metadata.LatencySeconds != 120 {
		t.Errorf("expected latency 120 seconds, got %v", msg.Metadata)
	}
}

func TestCorrelator_NilEvent(t *testing.T) {
	c, _, cleanup := setupTestCorrelator(t)
	defer cleanup()

	err := c.ProcessEvent(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}

func TestCorrelator_PacketReceived_NoMatchingPending(t *testing.T) {
	c, _, cleanup := setupTestCorrelator(t)
	defer cleanup()

	ctx := context.Background()

	// PacketReceived without a matching PacketSent — should not error
	receivedEvent := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     56,
		BlockNumber: 35000000,
		TxHash:      "0xorphan",
		LogIndex:    0,
		Timestamp:   1700000120,
		EventType:   decoder.EventPacketReceived,
		Data: map[string]string{
			"src_eid":      "30101",
			"src_chain_id": "1",
			"sender":       "0x9999999999999999999999999999999999999999",
			"nonce":        "999999",
			"receiver":     "0x2222222222222222222222222222222222222222",
		},
	}

	err := c.ProcessEvent(ctx, receivedEvent)
	if err != nil {
		t.Fatalf("ProcessEvent should not error for orphan PacketReceived: %v", err)
	}
}

func TestCorrelator_OFTSent_CreatesPendingMessage(t *testing.T) {
	c, repo, cleanup := setupTestCorrelator(t)
	defer cleanup()

	ctx := context.Background()

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000100,
		TxHash:      "0xoft_corr",
		LogIndex:    5,
		Timestamp:   1700000200,
		EventType:   decoder.EventOFTSent,
		Data: map[string]string{
			"guid":            "0xcccc",
			"from_address":    "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"dst_eid":         "30102",
			"dst_chain_id":    "56",
			"amount_sent":     "1000000000000000000",
			"amount_received": "999000000000000000",
		},
	}

	err := c.ProcessEvent(ctx, event)
	if err != nil {
		t.Fatalf("ProcessEvent (OFTSent) failed: %v", err)
	}

	msg, err := repo.GetMessage(ctx, "0xcccc")
	if err != nil {
		t.Fatalf("GetMessage failed: %v", err)
	}

	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
	if msg.Type != types.TypeTokenTransfer {
		t.Errorf("expected type token_transfer, got %s", msg.Type)
	}
}
