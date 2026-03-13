package normalizer

import (
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

func newPacketSentEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    5,
		Timestamp:   1706000000,
		EventType:   "PacketSent",
		Data: map[string]string{
			"guid":         "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"nonce":        "42",
			"sender":       "0xSender1111111111111111111111111111111111",
			"receiver":     "0xReceiver2222222222222222222222222222222222",
			"src_eid":      "30101",
			"src_chain_id": "1",
			"dst_eid":      "30110",
			"dst_chain_id": "42161",
			"message":      "0xdeadbeef",
			"version":      "1",
		},
	}
}

func newPacketReceivedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xdef456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "PacketReceived",
		Data: map[string]string{
			"nonce":        "42",
			"sender":       "0xSender1111111111111111111111111111111111",
			"receiver":     "0xReceiver2222222222222222222222222222222222",
			"src_eid":      "30101",
			"src_chain_id": "1",
		},
	}
}

func newOFTSentEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    6,
		Timestamp:   1706000000,
		EventType:   "OFTSent",
		Data: map[string]string{
			"guid":            "0x1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			"from_address":    "0xSender1111111111111111111111111111111111",
			"dst_eid":         "30110",
			"dst_chain_id":    "42161",
			"amount_sent":     "1000000000",
			"amount_received": "999000000",
		},
	}
}

func TestNormalizePacketSent_BasicFields(t *testing.T) {
	event := newPacketSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("PacketSent should not be a correlation event")
	}

	msg := result.Message
	if msg.MessageID != event.Data["guid"] {
		t.Errorf("expected MessageID %s, got %s", event.Data["guid"], msg.MessageID)
	}
	if msg.Protocol != "layerzero_v2" {
		t.Errorf("expected protocol layerzero_v2, got %s", msg.Protocol)
	}
	if msg.Type != types.TypeMessage {
		t.Errorf("expected type message, got %s", msg.Type)
	}
	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestNormalizePacketSent_SourceFields(t *testing.T) {
	event := newPacketSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := result.Message.Source
	if src.ChainID != 1 {
		t.Errorf("expected chain_id 1, got %d", src.ChainID)
	}
	if src.TxHash != "0xabc123" {
		t.Errorf("expected tx_hash 0xabc123, got %s", src.TxHash)
	}
	if src.BlockNumber != 19000000 {
		t.Errorf("expected block_number 19000000, got %d", src.BlockNumber)
	}
	if src.Timestamp != 1706000000 {
		t.Errorf("expected timestamp 1706000000, got %d", src.Timestamp)
	}
	if src.Sender != "0xSender1111111111111111111111111111111111" {
		t.Errorf("expected sender 0xSender..., got %s", src.Sender)
	}
	if src.LogIndex != 5 {
		t.Errorf("expected log_index 5, got %d", src.LogIndex)
	}
}

func TestNormalizePacketSent_PayloadFields(t *testing.T) {
	event := newPacketSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := result.Message.Payload
	if payload == nil {
		t.Fatal("expected payload to be set")
	}
	if payload.Nonce != 42 {
		t.Errorf("expected nonce 42, got %d", payload.Nonce)
	}
	if payload.Data != "0xdeadbeef" {
		t.Errorf("expected data 0xdeadbeef, got %s", payload.Data)
	}
}

func TestNormalizePacketSent_NoDestination(t *testing.T) {
	event := newPacketSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message.Destination != nil {
		t.Error("PacketSent should not have a destination")
	}
}

func TestNormalizePacketSent_MissingGUID(t *testing.T) {
	event := newPacketSentEvent()
	delete(event.Data, "guid")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing guid")
	}
}

func TestNormalizePacketSent_InvalidNonce(t *testing.T) {
	event := newPacketSentEvent()
	event.Data["nonce"] = "not-a-number"

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for invalid nonce")
	}
}

func TestNormalizePacketReceived_CorrelationKey(t *testing.T) {
	event := newPacketReceivedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsCorrelation {
		t.Error("PacketReceived should be a correlation event")
	}

	key := result.CorrelationKey
	if key == nil {
		t.Fatal("expected correlation key to be set")
	}
	if key.Protocol != "layerzero_v2" {
		t.Errorf("expected protocol layerzero_v2, got %s", key.Protocol)
	}
	if key.Nonce != 42 {
		t.Errorf("expected nonce 42, got %d", key.Nonce)
	}
	if key.Sender != "0xSender1111111111111111111111111111111111" {
		t.Errorf("expected sender 0xSender..., got %s", key.Sender)
	}
	if key.SrcChainID != 1 {
		t.Errorf("expected src_chain_id 1, got %d", key.SrcChainID)
	}
}

func TestNormalizePacketReceived_DestinationFields(t *testing.T) {
	event := newPacketReceivedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := result.Message.Destination
	if dest == nil {
		t.Fatal("expected destination to be set")
	}
	if dest.ChainID != 42161 {
		t.Errorf("expected chain_id 42161, got %d", dest.ChainID)
	}
	if dest.TxHash != "0xdef456" {
		t.Errorf("expected tx_hash 0xdef456, got %s", dest.TxHash)
	}
	if dest.BlockNumber != 180000000 {
		t.Errorf("expected block_number 180000000, got %d", dest.BlockNumber)
	}
	if dest.Timestamp != 1706000045 {
		t.Errorf("expected timestamp 1706000045, got %d", dest.Timestamp)
	}
	if dest.Receiver != "0xReceiver2222222222222222222222222222222222" {
		t.Errorf("expected receiver 0xReceiver..., got %s", dest.Receiver)
	}
}

func TestNormalizePacketReceived_MissingSender(t *testing.T) {
	event := newPacketReceivedEvent()
	delete(event.Data, "sender")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
}

func TestNormalizeOFTSent_TokenTransferType(t *testing.T) {
	event := newOFTSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("OFTSent should not be a correlation event")
	}

	msg := result.Message
	if msg.Type != types.TypeTokenTransfer {
		t.Errorf("expected type token_transfer, got %s", msg.Type)
	}
	if msg.MessageID != event.Data["guid"] {
		t.Errorf("expected MessageID %s, got %s", event.Data["guid"], msg.MessageID)
	}
}

func TestNormalizeOFTSent_AmountFields(t *testing.T) {
	event := newOFTSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := result.Message.Payload
	if payload == nil {
		t.Fatal("expected payload to be set")
	}
	if payload.Amount != "1000000000" {
		t.Errorf("expected amount 1000000000, got %s", payload.Amount)
	}
}

func TestNormalizeOFTSent_SenderIsFromAddress(t *testing.T) {
	event := newOFTSentEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message.Source.Sender != "0xSender1111111111111111111111111111111111" {
		t.Errorf("expected sender from from_address, got %s", result.Message.Source.Sender)
	}
}

func TestNormalize_UnknownEventType(t *testing.T) {
	event := &decoder.RawEvent{
		EventType: "UnknownEvent",
		Data:      map[string]string{},
	}
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for unknown event type")
	}
}

func TestNormalize_NilEvent(t *testing.T) {
	_, err := Normalize(nil)
	if err == nil {
		t.Fatal("expected error for nil event")
	}
}
