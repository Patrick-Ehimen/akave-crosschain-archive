package wormhole

import (
	"math/big"
	"strings"
	"testing"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoderProtocol(t *testing.T) {
	d := NewWormholeDecoder()
	assert.Equal(t, ProtocolName, d.Protocol())
}

func TestDecoderContractAddresses_KnownChain(t *testing.T) {
	d := NewWormholeDecoder()
	// Ethereum has both Core Bridge and Token Bridge
	addrs := d.ContractAddresses(1)
	assert.Len(t, addrs, 2)
}

func TestDecoderContractAddresses_UnknownChain(t *testing.T) {
	d := NewWormholeDecoder()
	addrs := d.ContractAddresses(999999)
	assert.Empty(t, addrs)
}

func TestDecoderEventTopics(t *testing.T) {
	d := NewWormholeDecoder()
	topics := d.EventTopics()
	require.Len(t, topics, 2)

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	assert.Equal(t, parsedABI.Events["LogMessagePublished"].ID, topics[0])
	assert.Equal(t, parsedABI.Events["TransferRedeemed"].ID, topics[1])
}

func TestDecodeLogMessagePublished(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	sender := common.HexToAddress("0x3ee18B2214AFF97000D974cf647E7C347E8fa585")
	sequence := uint64(42)
	nonce := uint32(7)
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	consistencyLevel := uint8(15)

	event := parsedABI.Events["LogMessagePublished"]
	// Only pack non-indexed fields
	data, err := event.Inputs.NonIndexed().Pack(sequence, nonce, payload, consistencyLevel)
	require.NoError(t, err)

	log := ethtypes.Log{
		Address: common.HexToAddress("0x98f3c9e6E3fAce36bAAd05FE09d375Ef1464288B"),
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(sender.Bytes()), // indexed sender
		},
		Data:        data,
		BlockNumber: 19000000,
	}

	rawEvent, err := d.Decode(log, 1) // chainID 1 = Ethereum
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "LogMessagePublished", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(1), rawEvent.ChainID)

	assert.Equal(t, sender.Hex(), rawEvent.Data["sender"])
	assert.Equal(t, "42", rawEvent.Data["sequence"])
	assert.Equal(t, "7", rawEvent.Data["nonce"])
	assert.Equal(t, "15", rawEvent.Data["consistency_level"])
	assert.Equal(t, "0xdeadbeef", rawEvent.Data["payload"])

	// Ethereum = Wormhole chain 2
	assert.Equal(t, "2", rawEvent.Data["emitter_chain"])

	// message_id format: "emitterChain/emitterAddress/sequence"
	assert.Contains(t, rawEvent.Data["message_id"], "/42")
	assert.Contains(t, rawEvent.Data["message_id"], "2/")
}

func TestDecodeLogMessagePublished_EmptyPayload(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	sender := common.HexToAddress("0x1234567890123456789012345678901234567890")
	event := parsedABI.Events["LogMessagePublished"]
	data, err := event.Inputs.NonIndexed().Pack(uint64(1), uint32(0), []byte{}, uint8(1))
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(sender.Bytes()),
		},
		Data: data,
	}

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	assert.Equal(t, "0x", rawEvent.Data["payload"])
}

func TestDecodeLogMessagePublished_UnknownChain(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	sender := common.HexToAddress("0x1234567890123456789012345678901234567890")
	event := parsedABI.Events["LogMessagePublished"]
	data, err := event.Inputs.NonIndexed().Pack(uint64(99), uint32(0), []byte{}, uint8(1))
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(sender.Bytes()),
		},
		Data: data,
	}

	// chainID 99999 has no Wormhole mapping
	rawEvent, err := d.Decode(log, 99999)
	require.NoError(t, err)
	assert.Equal(t, "unknown", rawEvent.Data["emitter_chain"])
}

func TestDecodeTransferRedeemed(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	emitterChainID := uint16(2) // Wormhole chain 2 = Ethereum
	emitterAddress := common.HexToHash("0x0000000000000000000000003ee18B2214AFF97000D974cf647E7C347E8fa585")
	sequence := uint64(42)

	event := parsedABI.Events["TransferRedeemed"]

	// All three fields are indexed — pack them as topics
	emitterChainTopic := common.BigToHash(big.NewInt(int64(emitterChainID)))
	sequenceTopic := common.BigToHash(new(big.Int).SetUint64(sequence))

	log := ethtypes.Log{
		Address: common.HexToAddress("0x0b2402144Bb366A632D14B83F244D2e0e21bD39c"),
		Topics: []common.Hash{
			event.ID,
			emitterChainTopic,
			emitterAddress,
			sequenceTopic,
		},
		Data:        []byte{}, // no non-indexed fields
		BlockNumber: 180000000,
	}

	rawEvent, err := d.Decode(log, 42161) // chainID 42161 = Arbitrum (destination)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "TransferRedeemed", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(42161), rawEvent.ChainID)

	assert.Equal(t, "2", rawEvent.Data["emitter_chain"])
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"]) // Wormhole 2 → EVM 1 (Ethereum)
	assert.Equal(t, emitterAddress.Hex(), rawEvent.Data["emitter_address"])
	assert.Equal(t, "42", rawEvent.Data["sequence"])

	// message_id must match what LogMessagePublished would have produced
	assert.Equal(t, rawEvent.Data["message_id"],
		fmt.Sprintf("2/%s/42", emitterAddress.Hex()))
}

func TestDecodeTransferRedeemed_UnknownEmitterChain(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	event := parsedABI.Events["TransferRedeemed"]
	unknownChain := common.BigToHash(big.NewInt(9999))
	emitterAddress := common.HexToHash("0x0000000000000000000000003ee18B2214AFF97000D974cf647E7C347E8fa585")
	sequenceTopic := common.BigToHash(big.NewInt(1))

	log := ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			unknownChain,
			emitterAddress,
			sequenceTopic,
		},
		Data: []byte{},
	}

	rawEvent, err := d.Decode(log, 42161)
	require.NoError(t, err)
	assert.Equal(t, "unknown", rawEvent.Data["src_chain_id"])
}

func TestDecodeTransferRedeemed_InsufficientTopics(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	event := parsedABI.Events["TransferRedeemed"]

	log := ethtypes.Log{
		// Only 2 topics instead of required 4
		Topics: []common.Hash{event.ID, common.Hash{}},
		Data:   []byte{},
	}

	_, err = d.Decode(log, 42161)
	assert.Error(t, err)
}

func TestDecodeUnknownTopic(t *testing.T) {
	d := NewWormholeDecoder()

	log := ethtypes.Log{
		Topics: []common.Hash{common.HexToHash("0xdeadbeef")},
		Data:   []byte{},
	}

	_, err := d.Decode(log, 1)
	assert.Error(t, err)
}

func TestDecodeNoTopics(t *testing.T) {
	d := NewWormholeDecoder()

	log := ethtypes.Log{Topics: []common.Hash{}, Data: []byte{}}

	_, err := d.Decode(log, 1)
	assert.Error(t, err)
}

func TestDecodeLogMessagePublished_MalformedData(t *testing.T) {
	d := NewWormholeDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(wormholeABI))
	require.NoError(t, err)

	event := parsedABI.Events["LogMessagePublished"]
	sender := common.HexToAddress("0x3ee18B2214AFF97000D974cf647E7C347E8fa585")

	log := ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			common.BytesToHash(sender.Bytes()),
		},
		Data: []byte{0x01, 0x02, 0x03}, // truncated, cannot be ABI-decoded
	}

	_, err = d.Decode(log, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unpack")
}

func TestChainIDMapping_Roundtrip(t *testing.T) {
	for whID, evmID := range WormholeChainIDToEVM {
		gotWH, ok := EVMToWormholeChainID[evmID]
		assert.True(t, ok, "EVM chain %d has no reverse Wormhole mapping", evmID)
		assert.Equal(t, whID, gotWH, "round-trip mismatch for EVM chain %d", evmID)
	}
}