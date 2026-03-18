package correlator

import (
	"context"
	"fmt"
	"testing"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// mockMessageStore implements MessageStore for testing.
type mockMessageStore struct {
	upserted       []*types.Message
	findResult     *types.Message
	findErr        error
	updateDestCall *updateDestCall
	updateDestErr  error
}

type updateDestCall struct {
	MessageID string
	Dest      *types.Destination
	Status    types.MessageStatus
}

func (m *mockMessageStore) UpsertMessage(_ context.Context, msg *types.Message) error {
	m.upserted = append(m.upserted, msg)
	return nil
}

func (m *mockMessageStore) FindByCorrelationKey(_ context.Context, _ *normalizer.CorrelationKey) (*types.Message, error) {
	return m.findResult, m.findErr
}

func (m *mockMessageStore) UpdateDestination(_ context.Context, messageID string, dest *types.Destination, status types.MessageStatus) error {
	m.updateDestCall = &updateDestCall{
		MessageID: messageID,
		Dest:      dest,
		Status:    status,
	}
	return m.updateDestErr
}

func newTestCorrelator(store *mockMessageStore) *Correlator {
	log := zerolog.Nop()
	return New(store, log)
}

func TestProcess_PacketSent_UpsertsCalled(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    5,
		Timestamp:   1706000000,
		EventType:   "PacketSent",
		Data: map[string]string{
			"guid":    "0x1234",
			"nonce":   "42",
			"sender":  "0xSender",
			"message": "0xdeadbeef",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}

	msg := store.upserted[0]
	if msg.MessageID != "0x1234" {
		t.Errorf("expected message_id 0x1234, got %s", msg.MessageID)
	}
	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestProcess_OFTSent_UpsertsCalled(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    6,
		Timestamp:   1706000000,
		EventType:   "OFTSent",
		Data: map[string]string{
			"guid":            "0x5678",
			"from_address":    "0xSender",
			"amount_sent":     "1000",
			"amount_received": "999",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(store.upserted))
	}

	msg := store.upserted[0]
	if msg.Type != types.TypeTokenTransfer {
		t.Errorf("expected type token_transfer, got %s", msg.Type)
	}
}

func TestProcess_PacketReceived_CorrelatesExisting(t *testing.T) {
	existing := &types.Message{
		MessageID: "0x1234",
		Protocol:  "layerzero_v2",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{findResult: existing}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xdef456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "PacketReceived",
		Data: map[string]string{
			"nonce":        "42",
			"sender":       "0xSender",
			"receiver":     "0xReceiver",
			"src_chain_id": "1",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not have upserted a new message
	if len(store.upserted) != 0 {
		t.Errorf("expected 0 upserts, got %d", len(store.upserted))
	}

	// Should have called UpdateDestination
	if store.updateDestCall == nil {
		t.Fatal("expected UpdateDestination to be called")
	}
	if store.updateDestCall.MessageID != "0x1234" {
		t.Errorf("expected message_id 0x1234, got %s", store.updateDestCall.MessageID)
	}
	if store.updateDestCall.Status != types.StatusExecuted {
		t.Errorf("expected status executed, got %s", store.updateDestCall.Status)
	}
	if store.updateDestCall.Dest.ChainID != 42161 {
		t.Errorf("expected dest chain_id 42161, got %d", store.updateDestCall.Dest.ChainID)
	}
	if store.updateDestCall.Dest.Receiver != "0xReceiver" {
		t.Errorf("expected dest receiver 0xReceiver, got %s", store.updateDestCall.Dest.Receiver)
	}
}

func TestProcess_PacketReceived_NoMatchSkips(t *testing.T) {
	store := &mockMessageStore{findResult: nil}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xdef456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "PacketReceived",
		Data: map[string]string{
			"nonce":        "99",
			"sender":       "0xUnknown",
			"receiver":     "0xReceiver",
			"src_chain_id": "1",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not have called UpdateDestination
	if store.updateDestCall != nil {
		t.Error("expected UpdateDestination to NOT be called for unmatched event")
	}
}

func TestProcess_PacketReceived_FindError(t *testing.T) {
	store := &mockMessageStore{findErr: fmt.Errorf("db connection lost")}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:  "layerzero_v2",
		ChainID:   42161,
		EventType: "PacketReceived",
		Data: map[string]string{
			"nonce":        "42",
			"sender":       "0xSender",
			"receiver":     "0xReceiver",
			"src_chain_id": "1",
		},
	}

	err := c.Process(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when FindByCorrelationKey fails")
	}
}

func TestProcess_PacketReceived_UpdateDestError(t *testing.T) {
	existing := &types.Message{
		MessageID: "0x1234",
		Protocol:  "layerzero_v2",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{
		findResult:    existing,
		updateDestErr: fmt.Errorf("db write failed"),
	}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:  "layerzero_v2",
		ChainID:   42161,
		EventType: "PacketReceived",
		Data: map[string]string{
			"nonce":        "42",
			"sender":       "0xSender",
			"receiver":     "0xReceiver",
			"src_chain_id": "1",
		},
	}

	err := c.Process(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when UpdateDestination fails")
	}
}

func TestProcess_UnsupportedEventType(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		EventType: "UnknownEvent",
		Data:      map[string]string{},
	}

	err := c.Process(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for unsupported event type")
	}
}
