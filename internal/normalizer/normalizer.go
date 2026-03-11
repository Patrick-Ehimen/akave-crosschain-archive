package normalizer

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// Normalize converts a decoded RawEvent into a unified Message.
// For PacketSent events, a new message is created with status "pending".
// For PacketReceived events, only destination details are returned (correlation handled separately).
// For OFTSent events, a token transfer message is created.
func Normalize(event *decoder.RawEvent) (*types.Message, error) {
	if event == nil {
		return nil, fmt.Errorf("nil event")
	}

	now := time.Now().UTC()

	switch event.EventType {
	case decoder.EventPacketSent:
		return normalizePacketSent(event, now)
	case decoder.EventPacketReceived:
		return normalizePacketReceived(event, now)
	case decoder.EventOFTSent:
		return normalizeOFTSent(event, now)
	default:
		return nil, fmt.Errorf("unknown event type: %s", event.EventType)
	}
}

// MessageID generates a deterministic message ID from a GUID or event fields.
// For LayerZero events, the GUID from the packet is used when available.
// Otherwise, a composite key is constructed from protocol+chain+tx+logIndex.
func MessageID(event *decoder.RawEvent) string {
	if guid, ok := event.Data["guid"]; ok && guid != "" {
		return guid
	}
	return fmt.Sprintf("%s-%d-%s-%d", event.Protocol, event.ChainID, event.TxHash, event.LogIndex)
}

func normalizePacketSent(event *decoder.RawEvent, now time.Time) (*types.Message, error) {
	msgID := MessageID(event)

	nonce, err := parseRequiredUint(event.Data, "nonce")
	if err != nil {
		return nil, fmt.Errorf("parsing PacketSent nonce: %w", err)
	}
	dstChainID, err := parseOptionalChainID(event.Data, "dst_chain_id")
	if err != nil {
		return nil, fmt.Errorf("parsing PacketSent destination chain: %w", err)
	}

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
			Data:  event.Data["message"],
			Nonce: nonce,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Set destination chain ID if known (from dstEid mapping).
	if dstChainID > 0 {
		msg.Destination = &types.Destination{
			ChainID:  dstChainID,
			Receiver: event.Data["receiver"],
		}
	}

	return msg, nil
}

func normalizePacketReceived(event *decoder.RawEvent, now time.Time) (*types.Message, error) {
	nonce, err := parseRequiredUint(event.Data, "nonce")
	if err != nil {
		return nil, fmt.Errorf("parsing PacketReceived nonce: %w", err)
	}
	srcChainID, err := parseOptionalChainID(event.Data, "src_chain_id")
	if err != nil {
		return nil, fmt.Errorf("parsing PacketReceived source chain: %w", err)
	}

	// For PacketReceived, we return a partial message that the correlator will use
	// to update an existing pending message to executed.
	msg := &types.Message{
		Protocol: event.Protocol,
		Type:     types.TypeMessage,
		Status:   types.StatusExecuted,
		Source: types.Source{
			ChainID: srcChainID,
			Sender:  event.Data["sender"],
		},
		Destination: &types.Destination{
			ChainID:     event.ChainID,
			TxHash:      event.TxHash,
			BlockNumber: event.BlockNumber,
			Timestamp:   event.Timestamp,
			Receiver:    event.Data["receiver"],
			LogIndex:    event.LogIndex,
		},
		Payload: &types.Payload{
			Nonce: nonce,
		},
		UpdatedAt: now,
	}

	return msg, nil
}

func normalizeOFTSent(event *decoder.RawEvent, now time.Time) (*types.Message, error) {
	msgID := MessageID(event)

	dstChainID, err := parseOptionalChainID(event.Data, "dst_chain_id")
	if err != nil {
		return nil, fmt.Errorf("parsing OFTSent destination chain: %w", err)
	}

	msg := &types.Message{
		MessageID: msgID,
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
		CreatedAt: now,
		UpdatedAt: now,
	}

	if dstChainID > 0 {
		msg.Destination = &types.Destination{
			ChainID: dstChainID,
		}
	}

	return msg, nil
}

func parseRequiredUint(data map[string]string, field string) (uint64, error) {
	value, ok := data[field]
	if !ok || value == "" {
		return 0, fmt.Errorf("missing %s", field)
	}

	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}

	return parsed, nil
}

func parseOptionalChainID(data map[string]string, field string) (uint64, error) {
	value := data[field]
	if value == "" || value == "unknown" {
		return 0, nil
	}

	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", field, value, err)
	}

	return parsed, nil
}
