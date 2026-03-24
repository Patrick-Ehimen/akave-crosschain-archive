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


func newLogMessagePublishedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "wormhole",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    7,
		Timestamp:   1706000000,
		EventType:   "LogMessagePublished",
		Data: map[string]string{
			"sender":          "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
			"emitter_address": "0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585",
			"sequence":        "42",
			"nonce":           "7",
			"consistency_level": "15",
			"payload":         "0xdeadbeef",
			"emitter_chain":   "2",
			"message_id":      "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
		},
	}
}

func newTransferRedeemedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "wormhole",
		ChainID:     42161,
		BlockNumber: 180000000,
		TxHash:      "0xdef456",
		LogIndex:    3,
		Timestamp:   1706000045,
		EventType:   "TransferRedeemed",
		Data: map[string]string{
			// sender is the EVM address (last 20 bytes of emitter_address)
			// This MUST match what was stored as message_sources.sender
			"sender":          "0x3ee18B2214AFF97000D974cf647E7C347E8fa585",
			"emitter_address": "0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585",
			"emitter_chain":   "2",
			"src_chain_id":    "1",
			"sequence":        "42",
			"receiver":        "0x0b2402144Bb366A632D14B83F244D2e0e21bD39c",
			"message_id":      "2/0x0000000000000000000000003ee18b2214aff97000d974cf647e7c347e8fa585/42",
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

// --- Axelar event helpers ---

func newContractCallEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "axelar",
		ChainID:     1,
		BlockNumber: 19500000,
		TxHash:      "0xaxelar123",
		LogIndex:    3,
		Timestamp:   1707000000,
		EventType:   "ContractCall",
		Data: map[string]string{
			"sender":                       "0xSender1111111111111111111111111111111111",
			"destination_chain":            "Avalanche",
			"destination_contract_address": "0xDest2222222222222222222222222222222222222222",
			"payload_hash":                 "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"payload":                      "deadbeef",
			"dst_chain_id":                 "43114",
		},
	}
}

func newContractCallApprovedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "axelar",
		ChainID:     43114,
		BlockNumber: 50000000,
		TxHash:      "0xaxelar456",
		LogIndex:    2,
		Timestamp:   1707000060,
		EventType:   "ContractCallApproved",
		Data: map[string]string{
			"command_id":         "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			"source_chain":       "ethereum",
			"source_address":     "0xSender1111111111111111111111111111111111",
			"contract_address":   "0xDest2222222222222222222222222222222222222222",
			"payload_hash":       "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			"source_tx_hash":     "0xaxelar123",
			"source_event_index": "3",
			"src_chain_id":       "1",
		},
	}
}

func newExecutedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
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
}

// --- Axelar ContractCall tests ---

func TestNormalizeContractCall_BasicFields(t *testing.T) {
	event := newContractCallEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("ContractCall should not be a correlation event")
	}

	msg := result.Message
	if msg.MessageID != event.Data["payload_hash"] {
		t.Errorf("expected MessageID %s, got %s", event.Data["payload_hash"], msg.MessageID)
	}
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

func TestNormalizeContractCall_SourceFields(t *testing.T) {
	event := newContractCallEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := result.Message.Source
	if src.ChainID != 1 {
		t.Errorf("expected chain_id 1, got %d", src.ChainID)
	}
	if src.TxHash != "0xaxelar123" {
		t.Errorf("expected tx_hash 0xaxelar123, got %s", src.TxHash)
	}
	if src.Sender != "0xSender1111111111111111111111111111111111" {
		t.Errorf("expected sender, got %s", src.Sender)
	}
	if src.LogIndex != 3 {
		t.Errorf("expected log_index 3, got %d", src.LogIndex)
	}
}

func TestNormalizeContractCall_PayloadFields(t *testing.T) {
	event := newContractCallEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := result.Message.Payload
	if payload == nil {
		t.Fatal("expected payload to be set")
	}
	if payload.Data != "deadbeef" {
		t.Errorf("expected data deadbeef, got %s", payload.Data)
	}
}

func TestNormalizeContractCall_MissingPayloadHash(t *testing.T) {
	event := newContractCallEvent()
	delete(event.Data, "payload_hash")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing payload_hash")
	}
}

func TestNormalizeContractCall_MissingSender(t *testing.T) {
	event := newContractCallEvent()
	delete(event.Data, "sender")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
}

// --- Axelar ContractCallApproved tests ---

func TestNormalizeContractCallApproved_BasicFields(t *testing.T) {
	event := newContractCallApprovedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("ContractCallApproved should not be a correlation event")
	}

	msg := result.Message
	if msg.MessageID != event.Data["command_id"] {
		t.Errorf("expected MessageID %s, got %s", event.Data["command_id"], msg.MessageID)
	}
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

func TestNormalizeContractCallApproved_SourceInfo(t *testing.T) {
	event := newContractCallApprovedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := result.Message.Source
	if src.ChainID != 1 {
		t.Errorf("expected src chain_id 1, got %d", src.ChainID)
	}
	if src.TxHash != "0xaxelar123" {
		t.Errorf("expected source_tx_hash, got %s", src.TxHash)
	}
	if src.Sender != "0xSender1111111111111111111111111111111111" {
		t.Errorf("expected source_address as sender, got %s", src.Sender)
	}
}

func TestNormalizeContractCallApproved_MissingCommandID(t *testing.T) {
	event := newContractCallApprovedEvent()
	delete(event.Data, "command_id")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing command_id")
	}
}

// --- Axelar Executed tests ---

func TestNormalizeExecuted_CorrelationKey(t *testing.T) {
	event := newExecutedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsCorrelation {
		t.Error("Executed should be a correlation event")
	}

	key := result.CorrelationKey
	if key == nil {
		t.Fatal("expected correlation key to be set")
	}
	if key.Protocol != "axelar" {
		t.Errorf("expected protocol axelar, got %s", key.Protocol)
	}
	if key.MessageID != "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Errorf("expected MessageID to be commandId, got %s", key.MessageID)
	}
}

func TestNormalizeExecuted_DestinationFields(t *testing.T) {
	event := newExecutedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	dest := result.Message.Destination
	if dest == nil {
		t.Fatal("expected destination to be set")
	}
	if dest.ChainID != 43114 {
		t.Errorf("expected chain_id 43114, got %d", dest.ChainID)
	}
	if dest.TxHash != "0xaxelar789" {
		t.Errorf("expected tx_hash 0xaxelar789, got %s", dest.TxHash)
	}
	if dest.BlockNumber != 50000100 {
		t.Errorf("expected block_number 50000100, got %d", dest.BlockNumber)
	}
	if dest.Timestamp != 1707000120 {
		t.Errorf("expected timestamp 1707000120, got %d", dest.Timestamp)
	}
}

func TestNormalizeExecuted_MissingCommandID(t *testing.T) {
	event := newExecutedEvent()
	delete(event.Data, "command_id")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing command_id")
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

func TestNormalizeLogMessagePublished_BasicFields(t *testing.T) {
	event := newLogMessagePublishedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("LogMessagePublished should not be a correlation event")
	}

	msg := result.Message
	if msg.MessageID != event.Data["message_id"] {
		t.Errorf("expected MessageID %s, got %s", event.Data["message_id"], msg.MessageID)
	}
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

func TestNormalizeLogMessagePublished_SourceFields(t *testing.T) {
	event := newLogMessagePublishedEvent()
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
	// CRITICAL: sender must be the EVM address, not the bytes32 emitter_address
	// This is what gets stored in message_sources.sender and queried during correlation
	if src.Sender != "0x3ee18B2214AFF97000D974cf647E7C347E8fa585" {
		t.Errorf("expected EVM sender address, got %s", src.Sender)
	}
}

func TestNormalizeLogMessagePublished_MissingMessageID(t *testing.T) {
	event := newLogMessagePublishedEvent()
	delete(event.Data, "message_id")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing message_id")
	}
}

func TestNormalizeLogMessagePublished_NoDestination(t *testing.T) {
	event := newLogMessagePublishedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message.Destination != nil {
		t.Error("LogMessagePublished should not have a destination")
	}
}

func TestNormalizeTransferRedeemed_CorrelationKey(t *testing.T) {
	event := newTransferRedeemedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsCorrelation {
		t.Error("TransferRedeemed should be a correlation event")
	}

	key := result.CorrelationKey
	if key == nil {
		t.Fatal("expected correlation key to be set")
	}
	if key.Protocol != "wormhole" {
		t.Errorf("expected protocol wormhole, got %s", key.Protocol)
	}
	// Nonce = sequence number
	if key.Nonce != 42 {
		t.Errorf("expected nonce (sequence) 42, got %d", key.Nonce)
	}
	// CRITICAL: Sender must be the EVM address — same format as stored in message_sources.sender
	// If this is bytes32 (0x0000...3ee18B) instead of EVM address (0x3ee18B...),
	// FindByCorrelationKey will never match and correlation silently fails.
	if key.Sender != "0x3ee18B2214AFF97000D974cf647E7C347E8fa585" {
		t.Errorf("expected EVM address for sender, got %s — this will break correlation", key.Sender)
	}
	if key.SrcChainID != 1 {
		t.Errorf("expected src_chain_id 1, got %d", key.SrcChainID)
	}
}

func TestNormalizeTransferRedeemed_SenderEVMAddressMatchesSource(t *testing.T) {
	// This test verifies the exact address format consistency between
	// LogMessagePublished (source) and TransferRedeemed (correlation).
	// If these don't produce the same sender string, DB correlation fails silently.

	sourceEvent := newLogMessagePublishedEvent()
	destEvent := newTransferRedeemedEvent()

	sourceResult, err := Normalize(sourceEvent)
	if err != nil {
		t.Fatalf("failed to normalize source: %v", err)
	}

	destResult, err := Normalize(destEvent)
	if err != nil {
		t.Fatalf("failed to normalize dest: %v", err)
	}

	sourceSender := sourceResult.Message.Source.Sender
	correlationSender := destResult.CorrelationKey.Sender

	if sourceSender != correlationSender {
		t.Errorf(
			"CORRELATION MISMATCH: source stores sender=%q but TransferRedeemed correlates with sender=%q — these must be identical for FindByCorrelationKey to work",
			sourceSender,
			correlationSender,
		)
	}
}

func TestNormalizeTransferRedeemed_DestinationFields(t *testing.T) {
	event := newTransferRedeemedEvent()
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
	if dest.Receiver != "0x0b2402144Bb366A632D14B83F244D2e0e21bD39c" {
		t.Errorf("expected receiver, got %s", dest.Receiver)
	}
}

func TestNormalizeTransferRedeemed_MissingMessageID(t *testing.T) {
	event := newTransferRedeemedEvent()
	delete(event.Data, "message_id")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing message_id")
	}
}

func TestNormalizeTransferRedeemed_MissingSender(t *testing.T) {
	event := newTransferRedeemedEvent()
	delete(event.Data, "sender")

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
}

func TestNormalizeTransferRedeemed_InvalidSrcChainID(t *testing.T) {
	event := newTransferRedeemedEvent()
	event.Data["src_chain_id"] = "unknown"

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for non-numeric src_chain_id")
	}
}

func TestNormalizeTransferRedeemed_InvalidSequence(t *testing.T) {
	event := newTransferRedeemedEvent()
	event.Data["sequence"] = "not-a-number"

	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for invalid sequence")
	}
}


// ─── CCIP normalizer test additions ──────────────────────────────────────

// newCCIPSendRequestedEvent returns a sample CCIPSendRequested raw event
// representing a token transfer from Ethereum to Arbitrum.
func newCCIPSendRequestedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "ccip",
		ChainID:     1,
		BlockNumber: 19500000,
		TxHash:      "0xccip111aaa",
		LogIndex:    5,
		Timestamp:   1708000000,
		EventType:   "CCIPSendRequested",
		Data: map[string]string{
			"message_id":           "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab",
			"sequence_number":      "42",
			"sender":               "0x1111111111111111111111111111111111111111",
			"receiver":             "0x2222222222222222222222222222222222222222",
			"nonce":                "7",
			"src_chain_id":         "1",
			"source_chain_selector": "5009297550715157269",
			"fee_token":            "0x3333333333333333333333333333333333333333",
			"fee_token_amount":     "100000000000000",
			"gas_limit":            "200000",
			"data":                 "0x",
			"token_count":          "1",
			"token":                "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			"amount":               "1000000000",
		},
	}
}

// newCCIPMessageEvent returns a CCIPSendRequested for a pure message (no tokens).
func newCCIPMessageEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
		Protocol:    "ccip",
		ChainID:     42161,
		BlockNumber: 200000000,
		TxHash:      "0xccip222bbb",
		LogIndex:    3,
		Timestamp:   1708000100,
		EventType:   "CCIPSendRequested",
		Data: map[string]string{
			"message_id":           "0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			"sequence_number":      "1",
			"sender":               "0x4444444444444444444444444444444444444444",
			"receiver":             "0x5555555555555555555555555555555555555555",
			"nonce":                "1",
			"src_chain_id":         "42161",
			"source_chain_selector": "4949039107694359620",
			"fee_token":            "0x6666666666666666666666666666666666666666",
			"fee_token_amount":     "50000000000000",
			"gas_limit":            "150000",
			"data":                 "0xdeadbeef",
			"token_count":          "0",
		},
	}
}

// newCCIPExecutionStateChangedEvent returns a successful ExecutionStateChanged event.
func newCCIPExecutionStateChangedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
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
}

// newCCIPExecutionFailedEvent returns a failed ExecutionStateChanged event.
func newCCIPExecutionFailedEvent() *decoder.RawEvent {
	return &decoder.RawEvent{
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
}

// ─── CCIPSendRequested normalizer tests ───────────────────────────────────

func TestNormalizeCCIPSendRequested_TokenTransfer(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.IsCorrelation {
		t.Error("CCIPSendRequested should not be a correlation event")
	}

	msg := result.Message
	if msg.MessageID != event.Data["message_id"] {
		t.Errorf("expected message_id %s, got %s", event.Data["message_id"], msg.MessageID)
	}
	if msg.Protocol != "ccip" {
		t.Errorf("expected protocol ccip, got %s", msg.Protocol)
	}
	if msg.Type != types.TypeTokenTransfer {
		t.Errorf("expected type token_transfer (token_count=1), got %s", msg.Type)
	}
	if msg.Status != types.StatusPending {
		t.Errorf("expected status pending, got %s", msg.Status)
	}
}

func TestNormalizeCCIPSendRequested_PureMessage(t *testing.T) {
	event := newCCIPMessageEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Message.Type != types.TypeMessage {
		t.Errorf("expected type message (token_count=0), got %s", result.Message.Type)
	}
}

func TestNormalizeCCIPSendRequested_SourceFields(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := result.Message.Source
	if src.ChainID != 1 {
		t.Errorf("expected chain_id 1, got %d", src.ChainID)
	}
	if src.TxHash != "0xccip111aaa" {
		t.Errorf("expected tx_hash, got %s", src.TxHash)
	}
	if src.Sender != "0x1111111111111111111111111111111111111111" {
		t.Errorf("expected sender, got %s", src.Sender)
	}
	if src.LogIndex != 5 {
		t.Errorf("expected log_index 5, got %d", src.LogIndex)
	}
	if src.Timestamp != 1708000000 {
		t.Errorf("expected timestamp 1708000000, got %d", src.Timestamp)
	}
}

func TestNormalizeCCIPSendRequested_PayloadFields(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	payload := result.Message.Payload
	if payload == nil {
		t.Fatal("expected payload to be set")
	}
	if payload.Token != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
		t.Errorf("expected token USDC, got %s", payload.Token)
	}
	if payload.Amount != "1000000000" {
		t.Errorf("expected amount 1000000000, got %s", payload.Amount)
	}
	if payload.Nonce != 7 {
		t.Errorf("expected nonce 7, got %d", payload.Nonce)
	}
}

func TestNormalizeCCIPSendRequested_MetadataFee(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	meta := result.Message.Metadata
	if meta == nil {
		t.Fatal("expected metadata to be set")
	}
	if meta.Fee != "100000000000000" {
		t.Errorf("expected fee 100000000000000, got %s", meta.Fee)
	}
}

func TestNormalizeCCIPSendRequested_NoDestination(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message.Destination != nil {
		t.Error("CCIPSendRequested should not have a destination")
	}
}

func TestNormalizeCCIPSendRequested_MissingMessageID(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	delete(event.Data, "message_id")
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing message_id")
	}
}

func TestNormalizeCCIPSendRequested_MissingSender(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	delete(event.Data, "sender")
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing sender")
	}
}

func TestNormalizeCCIPSendRequested_InvalidSrcChainID(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	event.Data["src_chain_id"] = "not-a-number"
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for invalid src_chain_id")
	}
}

func TestNormalizeCCIPSendRequested_InvalidNonce(t *testing.T) {
	event := newCCIPSendRequestedEvent()
	event.Data["nonce"] = "nan"
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for invalid nonce")
	}
}

// ─── ExecutionStateChanged normalizer tests ───────────────────────────────

func TestNormalizeCCIPExecutionStateChanged_CorrelationKey(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent()
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsCorrelation {
		t.Error("ExecutionStateChanged should be a correlation event")
	}

	key := result.CorrelationKey
	if key == nil {
		t.Fatal("expected correlation key to be set")
	}
	if key.Protocol != "ccip" {
		t.Errorf("expected protocol ccip, got %s", key.Protocol)
	}
	// CCIP uses direct MessageID lookup (same pattern as Axelar commandId)
	if key.MessageID != "0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ab" {
		t.Errorf("expected MessageID to be CCIP messageId, got %s", key.MessageID)
	}
	// Nonce and Sender should be zero/empty — CCIP correlates by messageId only
	if key.Nonce != 0 {
		t.Errorf("expected zero Nonce for CCIP correlation, got %d", key.Nonce)
	}
	if key.Sender != "" {
		t.Errorf("expected empty Sender for CCIP correlation, got %s", key.Sender)
	}
}

func TestNormalizeCCIPExecutionStateChanged_DestinationFields(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent()
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
	if dest.TxHash != "0xccip333ccc" {
		t.Errorf("expected tx_hash, got %s", dest.TxHash)
	}
	if dest.BlockNumber != 210000000 {
		t.Errorf("expected block_number 210000000, got %d", dest.BlockNumber)
	}
	if dest.Timestamp != 1708000090 {
		t.Errorf("expected timestamp 1708000090, got %d", dest.Timestamp)
	}
}

func TestNormalizeCCIPExecutionStateChanged_SuccessStatus(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent() // state=2
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message.Status != types.StatusExecuted {
		t.Errorf("expected StatusExecuted for state=2, got %s", result.Message.Status)
	}
}

func TestNormalizeCCIPExecutionStateChanged_FailureStatus(t *testing.T) {
	event := newCCIPExecutionFailedEvent() // state=3
	result, err := Normalize(event)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Message.Status != types.StatusFailed {
		t.Errorf("expected StatusFailed for state=3, got %s", result.Message.Status)
	}
}

func TestNormalizeCCIPExecutionStateChanged_MissingMessageID(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent()
	delete(event.Data, "message_id")
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing message_id")
	}
}

func TestNormalizeCCIPExecutionStateChanged_MissingState(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent()
	delete(event.Data, "state")
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for missing state")
	}
}

func TestNormalizeCCIPExecutionStateChanged_InvalidState(t *testing.T) {
	event := newCCIPExecutionStateChangedEvent()
	event.Data["state"] = "not-a-number"
	_, err := Normalize(event)
	if err == nil {
		t.Fatal("expected error for invalid state")
	}
}

// ─── Cross-event message ID consistency ───────────────────────────────────

// TestCCIPMessageIDConsistencyAcrossEvents verifies that the message_id field
// produced by CCIPSendRequested normalisation exactly matches the MessageID
// used as the correlation key in ExecutionStateChanged normalisation.
// A mismatch here would cause FindByCorrelationKey to return nil for every
// CCIP message, silently preventing any CCIP message from reaching 'executed'.
func TestCCIPMessageIDConsistencyAcrossEvents(t *testing.T) {
	sendEvent := newCCIPSendRequestedEvent()
	execEvent := newCCIPExecutionStateChangedEvent()

	// Both events share the same message_id value.
	sendResult, err := Normalize(sendEvent)
	if err != nil {
		t.Fatalf("failed to normalize CCIPSendRequested: %v", err)
	}

	execResult, err := Normalize(execEvent)
	if err != nil {
		t.Fatalf("failed to normalize ExecutionStateChanged: %v", err)
	}

	sourceMessageID := sendResult.Message.MessageID
	correlationMessageID := execResult.CorrelationKey.MessageID

	if sourceMessageID != correlationMessageID {
		t.Errorf(
			"CORRELATION MISMATCH: CCIPSendRequested stores message_id=%q but "+
				"ExecutionStateChanged correlates with MessageID=%q — "+
				"these MUST be identical for FindByCorrelationKey to work",
			sourceMessageID,
			correlationMessageID,
		)
	}
}