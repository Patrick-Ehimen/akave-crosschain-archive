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

func TestProcess_CCIPExecutionStateChanged_SuccessStatus(t *testing.T) {
	existing := &types.Message{
		MessageID: "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
		Protocol:  "ccip",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{findResult: existing}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "ccip",
		ChainID:     42161,
		BlockNumber: 210000000,
		TxHash:      "0xccip333ccc",
		LogIndex:    2,
		Timestamp:   1708000090,
		EventType:   "ExecutionStateChanged",
		Data: map[string]string{
			"message_id":      "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
			"sequence_number": "42",
			"state":           "2",
			"state_name":      "success",
			"return_data":     "0x",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.updateDestCall == nil {
		t.Fatal("expected UpdateDestination to be called")
	}
	if store.updateDestCall.Status != types.StatusExecuted {
		t.Errorf("expected status executed for state=2, got %s", store.updateDestCall.Status)
	}
}

func TestProcess_CCIPExecutionStateChanged_FailureStatus(t *testing.T) {
	existing := &types.Message{
		MessageID: "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		Protocol:  "ccip",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{findResult: existing}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "ccip",
		ChainID:     8453,
		BlockNumber: 5100000,
		TxHash:      "0xccip444ddd",
		LogIndex:    9,
		Timestamp:   1708000200,
		EventType:   "ExecutionStateChanged",
		Data: map[string]string{
			"message_id":      "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"sequence_number": "1",
			"state":           "3",
			"state_name":      "failure",
			"return_data":     "0x08c379a0",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.updateDestCall == nil {
		t.Fatal("expected UpdateDestination to be called")
	}
	if store.updateDestCall.Status != types.StatusFailed {
		t.Errorf("expected status failed for state=3, got %s", store.updateDestCall.Status)
	}
}

func TestProcess_CCIPExecutionStateChanged_NonTerminalSkipped(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "ccip",
		ChainID:     42161,
		BlockNumber: 210000000,
		TxHash:      "0xccip555eee",
		LogIndex:    1,
		Timestamp:   1708000050,
		EventType:   "ExecutionStateChanged",
		Data: map[string]string{
			"message_id":      "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"sequence_number": "10",
			"state":           "1",
			"state_name":      "in_progress",
			"return_data":     "0x",
		},
	}

	// Non-terminal states (0, 1) should error during normalization
	err := c.Process(context.Background(), event)
	if err == nil {
		t.Fatal("expected error for non-terminal execution state")
	}

	// Should NOT have called UpdateDestination
	if store.updateDestCall != nil {
		t.Error("expected UpdateDestination to NOT be called for non-terminal state")
	}
}

// ─── Axelar correlator tests ──────────────────────────────────────────────

func TestProcess_ContractCallApproved_UpsertsCalled(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "axelar",
		ChainID:     43114,
		BlockNumber: 50000000,
		TxHash:      "0xaxelar456",
		LogIndex:    2,
		Timestamp:   1707000060,
		EventType:   "ContractCallApproved",
		Data: map[string]string{
			"command_id":     "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"source_chain":   "ethereum",
			"source_address": "0xSender1111111111111111111111111111111111",
			"src_chain_id":   "1",
			"source_tx_hash": "0xaxelar123",
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
	if msg.Protocol != "axelar" {
		t.Errorf("expected protocol axelar, got %s", msg.Protocol)
	}
	if msg.Type != types.TypeContractCall {
		t.Errorf("expected type contract_call, got %s", msg.Type)
	}
	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestProcess_AxelarExecuted_CorrelatesExisting(t *testing.T) {
	existing := &types.Message{
		MessageID: "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		Protocol:  "axelar",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{findResult: existing}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "axelar",
		ChainID:     43114,
		BlockNumber: 50000100,
		TxHash:      "0xaxelar789",
		LogIndex:    8,
		Timestamp:   1707000120,
		EventType:   "Executed",
		Data: map[string]string{
			"command_id": "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.upserted) != 0 {
		t.Errorf("expected 0 upserts for correlation event, got %d", len(store.upserted))
	}
	if store.updateDestCall == nil {
		t.Fatal("expected UpdateDestination to be called")
	}
	if store.updateDestCall.Status != types.StatusExecuted {
		t.Errorf("expected status executed, got %s", store.updateDestCall.Status)
	}
	if store.updateDestCall.Dest.ChainID != 43114 {
		t.Errorf("expected dest chain_id 43114, got %d", store.updateDestCall.Dest.ChainID)
	}
}

func TestProcess_AxelarExecuted_NoMatchSkips(t *testing.T) {
	store := &mockMessageStore{findResult: nil}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "axelar",
		ChainID:     43114,
		BlockNumber: 50000100,
		TxHash:      "0xaxelar789",
		LogIndex:    8,
		Timestamp:   1707000120,
		EventType:   "Executed",
		Data: map[string]string{
			"command_id": "0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.updateDestCall != nil {
		t.Error("expected UpdateDestination to NOT be called for unmatched event")
	}
}

// ─── Wormhole correlator tests ────────────────────────────────────────────

func TestProcess_LogMessagePublished_UpsertsCalled(t *testing.T) {
	store := &mockMessageStore{}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "wormhole",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xwh123",
		LogIndex:    7,
		Timestamp:   1706000000,
		EventType:   "LogMessagePublished",
		Data: map[string]string{
			"sender":     "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
			"sequence":   "42",
			"payload":    "0xdeadbeef",
			"message_id": "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
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
	if msg.Protocol != "wormhole" {
		t.Errorf("expected protocol wormhole, got %s", msg.Protocol)
	}
	if msg.Type != types.TypeMessage {
		t.Errorf("expected type message, got %s", msg.Type)
	}
	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestProcess_TransferRedeemed_CorrelatesExisting(t *testing.T) {
	existing := &types.Message{
		MessageID: "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
		Protocol:  "wormhole",
		Status:    types.StatusPending,
	}
	store := &mockMessageStore{findResult: existing}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "wormhole",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xwh456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "TransferRedeemed",
		Data: map[string]string{
			"sender":       "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
			"sequence":     "42",
			"src_chain_id": "1",
			"receiver":     "0x0b2402144Bb366A632D14B83F244D2e0e21bD39c",
			"message_id":   "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(store.upserted) != 0 {
		t.Errorf("expected 0 upserts for correlation event, got %d", len(store.upserted))
	}
	if store.updateDestCall == nil {
		t.Fatal("expected UpdateDestination to be called")
	}
	if store.updateDestCall.Status != types.StatusExecuted {
		t.Errorf("expected status executed, got %s", store.updateDestCall.Status)
	}
	if store.updateDestCall.Dest.ChainID != 42161 {
		t.Errorf("expected dest chain_id 42161, got %d", store.updateDestCall.Dest.ChainID)
	}
	if store.updateDestCall.Dest.Receiver != "0x0b2402144Bb366A632D14B83F244D2e0e21bD39c" {
		t.Errorf("expected dest receiver 0x0b2402..., got %s", store.updateDestCall.Dest.Receiver)
	}
}

func TestProcess_TransferRedeemed_NoMatchSkips(t *testing.T) {
	store := &mockMessageStore{findResult: nil}
	c := newTestCorrelator(store)

	event := &decoder.RawEvent{
		Protocol:    "wormhole",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xwh456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "TransferRedeemed",
		Data: map[string]string{
			"sender":       "0xUnknownSender",
			"sequence":     "999",
			"src_chain_id": "1",
			"receiver":     "0xReceiver",
			"message_id":   "2/0xUnknownEmitter/999",
		},
	}

	err := c.Process(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if store.updateDestCall != nil {
		t.Error("expected UpdateDestination to NOT be called for unmatched event")
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
