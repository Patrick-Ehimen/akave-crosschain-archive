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
	case "PacketSent":
		return normalizePacketSent(event, now)
	case "PacketReceived":
		return normalizePacketReceived(event, now)
	case "OFTSent":
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

	nonce, _ := strconv.ParseUint(event.Data["nonce"], 10, 64)
	dstChainID, _ := strconv.ParseUint(event.Data["dst_chain_id"], 10, 64)

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
	nonce, _ := strconv.ParseUint(event.Data["nonce"], 10, 64)
	srcChainID, _ := strconv.ParseUint(event.Data["src_chain_id"], 10, 64)

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

	dstChainID, _ := strconv.ParseUint(event.Data["dst_chain_id"], 10, 64)

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
