package normalizer

import (
	"testing"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize_NilEvent(t *testing.T) {
	_, err := Normalize(nil)
	require.Error(t, err)
}

func TestNormalize_UnknownEventType(t *testing.T) {
	event := &decoder.RawEvent{EventType: "UnknownEvent"}
	_, err := Normalize(event)
	require.Error(t, err)
}

func TestNormalize_PacketSent(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc123",
		LogIndex:    3,
		Timestamp:   1700000000,
		EventType:   decoder.EventPacketSent,
		Data: map[string]string{
			"version":      "1",
			"nonce":        "42",
			"src_eid":      "30101",
			"src_chain_id": "1",
			"dst_eid":      "30102",
			"dst_chain_id": "56",
			"sender":       "0x1111111111111111111111111111111111111111",
			"receiver":     "0x2222222222222222222222222222222222222222",
			"guid":         "0x3333333333333333333333333333333333333333333333333333333333333333",
			"message":      "0xdeadbeef",
		},
	}

	msg, err := Normalize(event)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// Message ID should be the GUID
	assert.Equal(t, "0x3333333333333333333333333333333333333333333333333333333333333333", msg.MessageID)
	assert.Equal(t, "layerzero_v2", msg.Protocol)
	assert.Equal(t, types.TypeMessage, msg.Type)
	assert.Equal(t, types.StatusPending, msg.Status)

	// Source
	assert.Equal(t, uint64(1), msg.Source.ChainID)
	assert.Equal(t, "0xabc123", msg.Source.TxHash)
	assert.Equal(t, uint64(19000000), msg.Source.BlockNumber)
	assert.Equal(t, int64(1700000000), msg.Source.Timestamp)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", msg.Source.Sender)
	assert.Equal(t, uint(3), msg.Source.LogIndex)

	// Destination (partial - only chain ID and receiver from PacketSent)
	require.NotNil(t, msg.Destination)
	assert.Equal(t, uint64(56), msg.Destination.ChainID)
	assert.Equal(t, "0x2222222222222222222222222222222222222222", msg.Destination.Receiver)

	// Payload
	require.NotNil(t, msg.Payload)
	assert.Equal(t, "0xdeadbeef", msg.Payload.Data)
	assert.Equal(t, uint64(42), msg.Payload.Nonce)
}

func TestNormalize_PacketReceived(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     56,
		BlockNumber: 35000000,
		TxHash:      "0xdef456",
		LogIndex:    1,
		Timestamp:   1700000120,
		EventType:   decoder.EventPacketReceived,
		Data: map[string]string{
			"src_eid":      "30101",
			"src_chain_id": "1",
			"sender":       "0x1111111111111111111111111111111111111111",
			"nonce":        "42",
			"receiver":     "0x2222222222222222222222222222222222222222",
		},
	}

	msg, err := Normalize(event)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, types.StatusExecuted, msg.Status)
	assert.Equal(t, "layerzero_v2", msg.Protocol)

	// Source should have the originating chain info
	assert.Equal(t, uint64(1), msg.Source.ChainID)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", msg.Source.Sender)

	// Destination should have the receiving chain info
	require.NotNil(t, msg.Destination)
	assert.Equal(t, uint64(56), msg.Destination.ChainID)
	assert.Equal(t, "0xdef456", msg.Destination.TxHash)
	assert.Equal(t, uint64(35000000), msg.Destination.BlockNumber)
	assert.Equal(t, int64(1700000120), msg.Destination.Timestamp)
	assert.Equal(t, "0x2222222222222222222222222222222222222222", msg.Destination.Receiver)

	// Payload nonce
	require.NotNil(t, msg.Payload)
	assert.Equal(t, uint64(42), msg.Payload.Nonce)
}

func TestNormalize_OFTSent(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000100,
		TxHash:      "0xoft789",
		LogIndex:    5,
		Timestamp:   1700000200,
		EventType:   decoder.EventOFTSent,
		Data: map[string]string{
			"guid":            "0x4444444444444444444444444444444444444444444444444444444444444444",
			"from_address":    "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			"dst_eid":         "30102",
			"dst_chain_id":    "56",
			"amount_sent":     "1000000000000000000",
			"amount_received": "999000000000000000",
		},
	}

	msg, err := Normalize(event)
	require.NoError(t, err)
	require.NotNil(t, msg)

	assert.Equal(t, "0x4444444444444444444444444444444444444444444444444444444444444444", msg.MessageID)
	assert.Equal(t, types.TypeTokenTransfer, msg.Type)
	assert.Equal(t, types.StatusPending, msg.Status)

	assert.Equal(t, "0xAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", msg.Source.Sender)

	require.NotNil(t, msg.Destination)
	assert.Equal(t, uint64(56), msg.Destination.ChainID)

	require.NotNil(t, msg.Payload)
	assert.Equal(t, "1000000000000000000", msg.Payload.Amount)
}

func TestMessageID_WithGUID(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol: "layerzero_v2",
		ChainID:  1,
		TxHash:   "0xabc",
		LogIndex: 0,
		Data:     map[string]string{"guid": "0xdeadbeef"},
	}

	id := MessageID(event)
	assert.Equal(t, "0xdeadbeef", id)
}

func TestMessageID_WithoutGUID(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol: "layerzero_v2",
		ChainID:  1,
		TxHash:   "0xabc",
		LogIndex: 5,
		Data:     map[string]string{},
	}

	id := MessageID(event)
	assert.Equal(t, "layerzero_v2-1-0xabc-5", id)
}

func TestNormalize_PacketSent_UnknownDstChain(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:  "layerzero_v2",
		ChainID:   1,
		TxHash:    "0xabc",
		EventType: decoder.EventPacketSent,
		Data: map[string]string{
			"nonce":        "1",
			"src_eid":      "30101",
			"src_chain_id": "1",
			"dst_eid":      "99999",
			"dst_chain_id": "unknown",
			"sender":       "0x1111111111111111111111111111111111111111",
			"receiver":     "0x2222222222222222222222222222222222222222",
			"guid":         "0xabcd",
			"message":      "0x",
		},
	}

	msg, err := Normalize(event)
	require.NoError(t, err)
	require.NotNil(t, msg)

	// "unknown" is preserved as an explicit unmapped chain sentinel,
	// so no destination should be set.
	assert.Nil(t, msg.Destination)
}

func TestNormalize_PacketSent_InvalidNonce(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:  "layerzero_v2",
		ChainID:   1,
		EventType: decoder.EventPacketSent,
		Data: map[string]string{
			"nonce":        "not-a-number",
			"dst_chain_id": "56",
			"sender":       "0x1111111111111111111111111111111111111111",
		},
	}

	_, err := Normalize(event)
	require.Error(t, err)
	assert.ErrorContains(t, err, "PacketSent nonce")
}

func TestNormalize_PacketReceived_InvalidSourceChain(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:  "layerzero_v2",
		ChainID:   56,
		EventType: decoder.EventPacketReceived,
		Data: map[string]string{
			"nonce":        "42",
			"src_chain_id": "bad-chain-id",
			"sender":       "0x1111111111111111111111111111111111111111",
			"receiver":     "0x2222222222222222222222222222222222222222",
		},
	}

	_, err := Normalize(event)
	require.Error(t, err)
	assert.ErrorContains(t, err, "PacketReceived source chain")
}
