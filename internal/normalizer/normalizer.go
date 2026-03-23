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

// CorrelationKey contains the fields needed to find an existing pending message
// for PacketReceived correlation. LayerZero PacketReceived events do not include
// a GUID, so we match by (protocol, nonce, sender, src_chain_id).
type CorrelationKey struct {
	Protocol   string
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

func parseUint64(s string) (uint64, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	return strconv.ParseUint(s, 10, 64)
}
