package normalizer

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// Result represents the outcome of normalizing a RawEvent.
type Result struct {
	// Message is the normalized message. For source events (PacketSent, OFTSent),
	// this is a complete message. For destination events (PacketReceived), this
	// contains only the Destination field for correlation.
	Message *types.Message

	// IsCorrelation indicates this event should update an existing message
	// rather than create a new one. True for PacketReceived events.
	IsCorrelation bool

	// CorrelationKey holds the fields for matching to an existing pending message.
	// Only set when IsCorrelation is true.
	CorrelationKey *CorrelationKey
}

// CorrelationKey contains the fields needed to find an existing pending message.
// For LayerZero PacketReceived: matches by (protocol, nonce, sender, src_chain_id).
// For Axelar Executed: matches by (protocol, message_id) using the commandId.
type CorrelationKey struct {
	Protocol   string
	MessageID  string // Direct message_id lookup (Axelar commandId, etc.)
	Nonce      uint64
	Sender     string
	SrcChainID uint64
}

// Normalize converts a decoder.RawEvent into a normalizer.Result.
func Normalize(event *decoder.RawEvent) (*Result, error) {
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}

	switch event.EventType {
	case "PacketSent":
		return normalizePacketSent(event)
	case "PacketReceived":
		return normalizePacketReceived(event)
	case "OFTSent":
		return normalizeOFTSent(event)
	case "ContractCall":
		return normalizeAxelarContractCall(event)
	case "ContractCallApproved":
		return normalizeAxelarContractCallApproved(event)
	case "Executed":
		return normalizeAxelarExecuted(event)
	case "CCIPSendRequested":
		return normalizeCCIPSendRequested(event)
	case "ExecutionStateChanged":
		return normalizeCCIPExecutionStateChanged(event)
	case "LogMessagePublished":
		return normalizeLogMessagePublished(event)
	case "TransferRedeemed":
		return normalizeTransferRedeemed(event)
	default:
		return nil, fmt.Errorf("unsupported event type: %s", event.EventType)
	}
}

// normalizePacketSent creates a new pending message from a PacketSent event.
func normalizePacketSent(event *decoder.RawEvent) (*Result, error) {
	guid := event.Data["guid"]
	if guid == "" {
		return nil, fmt.Errorf("PacketSent missing guid")
	}

	nonce, err := parseUint64(event.Data["nonce"])
	if err != nil {
		return nil, fmt.Errorf("PacketSent invalid nonce: %w", err)
	}

	now := time.Now()
	msg := &types.Message{
		MessageID: guid,
		Protocol:  event.Protocol,
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Sender:      event.Data["sender"],
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Data:  event.Data["message"],
			Nonce: nonce,
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizePacketReceived creates a correlation result for matching an existing message.
func normalizePacketReceived(event *decoder.RawEvent) (*Result, error) {
	nonce, err := parseUint64(event.Data["nonce"])
	if err != nil {
		return nil, fmt.Errorf("PacketReceived invalid nonce: %w", err)
	}

	sender := event.Data["sender"]
	if sender == "" {
		return nil, fmt.Errorf("PacketReceived missing sender")
	}

	srcChainID, err := parseUint64(event.Data["src_chain_id"])
	if err != nil {
		return nil, fmt.Errorf("PacketReceived invalid src_chain_id: %w", err)
	}

	// The chain where PacketReceived is observed is the destination chain.
	msg := &types.Message{
		Destination: &types.Destination{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Receiver:    event.Data["receiver"],
			LogIndex:    event.LogIndex,
		},
	}

	return &Result{
		Message:       msg,
		IsCorrelation: true,
		CorrelationKey: &CorrelationKey{
			Protocol:   event.Protocol,
			Nonce:      nonce,
			Sender:     sender,
			SrcChainID: srcChainID,
		},
	}, nil
}

// normalizeOFTSent creates a token transfer message from an OFTSent event.
func normalizeOFTSent(event *decoder.RawEvent) (*Result, error) {
	guid := event.Data["guid"]
	if guid == "" {
		return nil, fmt.Errorf("OFTSent missing guid")
	}

	now := time.Now()
	msg := &types.Message{
		MessageID: guid,
		Protocol:  event.Protocol,
		Type:      types.TypeTokenTransfer,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Sender:      event.Data["from_address"],
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Amount: event.Data["amount_sent"],
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizeAxelarContractCall creates a pending contract_call message from a ContractCall event.
func normalizeAxelarContractCall(event *decoder.RawEvent) (*Result, error) {
	payloadHash := event.Data["payload_hash"]
	if payloadHash == "" {
		return nil, fmt.Errorf("ContractCall missing payload_hash")
	}

	sender := event.Data["sender"]
	if sender == "" {
		return nil, fmt.Errorf("ContractCall missing sender")
	}

	now := time.Now()
	msg := &types.Message{
		MessageID: payloadHash,
		Protocol:  event.Protocol,
		Type:      types.TypeContractCall,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Sender:      sender,
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Data: event.Data["payload"],
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizeAxelarContractCallApproved creates a pending contract_call message
// with commandId as the messageID.
func normalizeAxelarContractCallApproved(event *decoder.RawEvent) (*Result, error) {
	commandID := event.Data["command_id"]
	if commandID == "" {
		return nil, fmt.Errorf("ContractCallApproved missing command_id")
	}

	srcChainID, _ := parseUint64(event.Data["src_chain_id"])

	now := time.Now()
	msg := &types.Message{
		MessageID: commandID,
		Protocol:  event.Protocol,
		Type:      types.TypeContractCall,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     srcChainID,
			TxHash:      event.Data["source_tx_hash"],
			Sender:      event.Data["source_address"],
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			LogIndex:    event.LogIndex,
		},
		Payload:   &types.Payload{},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizeAxelarExecuted creates a correlation result for matching via commandId.
func normalizeAxelarExecuted(event *decoder.RawEvent) (*Result, error) {
	commandID := event.Data["command_id"]
	if commandID == "" {
		return nil, fmt.Errorf("Executed missing command_id")
	}

	msg := &types.Message{
		Destination: &types.Destination{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			LogIndex:    event.LogIndex,
		},
	}

	return &Result{
		Message:       msg,
		IsCorrelation: true,
		CorrelationKey: &CorrelationKey{
			Protocol:  event.Protocol,
			MessageID: commandID,
		},
	}, nil
}

// normalizeLogMessagePublished creates a new pending message from a Wormhole
// LogMessagePublished event. The message_id is the canonical Wormhole VAA ID:
// "emitterChain/emitterAddress/sequence".
func normalizeLogMessagePublished(event *decoder.RawEvent) (*Result, error) {
	msgID := event.Data["message_id"]
	if msgID == "" {
		return nil, fmt.Errorf("LogMessagePublished missing message_id")
	}

	now := time.Now()
	msg := &types.Message{
		MessageID: msgID,
		Protocol:  event.Protocol,
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Sender:      event.Data["sender"],
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Data: event.Data["payload"],
		},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizeTransferRedeemed creates a correlation result for matching an existing
// pending Wormhole message. The correlation key uses the canonical VAA ID
// stored directly as message_id since Wormhole uniquely identifies messages that way.
func normalizeTransferRedeemed(event *decoder.RawEvent) (*Result, error) {
	msgID := event.Data["message_id"]
	if msgID == "" {
		return nil, fmt.Errorf("TransferRedeemed missing message_id")
	}

	sender := event.Data["sender"]
	if sender == "" {
		return nil, fmt.Errorf("TransferRedeemed missing sender")
	}

	srcChainID, err := parseUint64(event.Data["src_chain_id"])
	if err != nil {
		return nil, fmt.Errorf("TransferRedeemed invalid src_chain_id: %w", err)
	}

	sequence, err := parseUint64(event.Data["sequence"])
	if err != nil {
		return nil, fmt.Errorf("TransferRedeemed invalid sequence: %w", err)
	}

	msg := &types.Message{
		Destination: &types.Destination{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Receiver:    event.Data["receiver"],
			LogIndex:    event.LogIndex,
		},
	}

	return &Result{
		Message:       msg,
		IsCorrelation: true,
		CorrelationKey: &CorrelationKey{
			Protocol:   event.Protocol,
			Nonce:      sequence, // Wormhole uses sequence as the nonce equivalent
			Sender:     sender,
			SrcChainID: srcChainID,
		},
	}, nil
}

// normalizeCCIPSendRequested creates a new pending message from a
// CCIPSendRequested event emitted by a CCIP OnRamp contract.
//
// The messageId field is the globally unique CCIP message identifier and serves
// as both the message_id stored in the messages table and the correlation key
// used by ExecutionStateChanged. It is a bytes32 stored as a 0x-prefixed hex
// string (e.g. "0xabcdef...").
//
// Token transfer vs. pure message classification:
//   - If token_count > 0, the message type is TypeTokenTransfer.
//   - Otherwise, the message type is TypeMessage.
func normalizeCCIPSendRequested(event *decoder.RawEvent) (*Result, error) {
	msgID := event.Data["message_id"]
	if msgID == "" {
		return nil, fmt.Errorf("CCIPSendRequested missing message_id")
	}

	sender := event.Data["sender"]
	if sender == "" {
		return nil, fmt.Errorf("CCIPSendRequested missing sender")
	}

	srcChainID, err := parseUint64(event.Data["src_chain_id"])
	if err != nil {
		return nil, fmt.Errorf("CCIPSendRequested invalid src_chain_id: %w", err)
	}

	nonce, err := parseUint64(event.Data["nonce"])
	if err != nil {
		return nil, fmt.Errorf("CCIPSendRequested invalid nonce: %w", err)
	}

	// Determine message type from token_count.
	msgType := types.TypeMessage
	if tc := event.Data["token_count"]; tc != "" && tc != "0" {
		msgType = types.TypeTokenTransfer
	}

	now := time.Now()
	msg := &types.Message{
		MessageID: msgID,
		Protocol:  event.Protocol,
		Type:      msgType,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     srcChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Sender:      sender,
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Token:  event.Data["token"],
			Amount: event.Data["amount"],
			Data:   event.Data["data"],
			Nonce:  nonce,
		},
		Metadata: &types.Metadata{
			Fee: event.Data["fee_token_amount"],
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	return &Result{Message: msg}, nil
}

// normalizeCCIPExecutionStateChanged creates a correlation result from a
// CCIPExecutionStateChanged event emitted by a CCIP OffRamp contract.
//
// CCIP's ExecutionStateChanged has three meaningful outcome states:
//   - Success (2): the message was executed successfully → StatusExecuted
//   - Failure (3): the message execution reverted → StatusFailed
//   - Untouched / InProgress: not a terminal state; we skip these.
//
// messageId is used directly as the message_id correlation key (not nonce+sender),
// because CCIP messageIds are globally unique and guaranteed unique per message.
func normalizeCCIPExecutionStateChanged(event *decoder.RawEvent) (*Result, error) {
	msgID := event.Data["message_id"]
	if msgID == "" {
		return nil, fmt.Errorf("ExecutionStateChanged missing message_id")
	}

	stateStr := event.Data["state"]
	if stateStr == "" {
		return nil, fmt.Errorf("ExecutionStateChanged missing state")
	}

	stateNum, err := parseUint64(stateStr)
	if err != nil {
		return nil, fmt.Errorf("ExecutionStateChanged invalid state: %w", err)
	}

	// Map CCIP execution state to our unified MessageStatus.
	// Only Success (2) and Failure (3) are terminal states worth persisting.
	// Untouched (0) and InProgress (1) are transient — skip them entirely
	// to avoid the correlator accidentally updating messages.
	var status types.MessageStatus
	switch stateNum {
	case 2: // ExecutionStateSuccess
		status = types.StatusExecuted
	case 3: // ExecutionStateFailure
		status = types.StatusFailed
	default:
		return nil, fmt.Errorf("ExecutionStateChanged non-terminal state %d, skipping", stateNum)
	}

	msg := &types.Message{
		Status: status,
		Destination: &types.Destination{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			LogIndex:    event.LogIndex,
		},
	}

	return &Result{
		Message:       msg,
		IsCorrelation: true,
		CorrelationKey: &CorrelationKey{
			Protocol:  event.Protocol,
			MessageID: msgID, // Direct message_id lookup — same as Axelar commandId pattern
		},
	}, nil
}

func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	return strconv.ParseUint(s, 10, 64)
}
