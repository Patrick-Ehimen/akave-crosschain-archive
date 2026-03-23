package axelar

import (
	"math/big"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecoderProtocol(t *testing.T) {
	d := NewAxelarDecoder()
	assert.Equal(t, ProtocolName, d.Protocol())
}

func TestDecoderContractAddresses(t *testing.T) {
	d := NewAxelarDecoder()

	// Known chains should return addresses.
	assert.NotEmpty(t, d.ContractAddresses(1))
	assert.NotEmpty(t, d.ContractAddresses(43114))
	assert.NotEmpty(t, d.ContractAddresses(137))

	// Unknown chain returns nil.
	assert.Empty(t, d.ContractAddresses(999999))
}

func TestDecoderEventTopics(t *testing.T) {
	d := NewAxelarDecoder()
	topics := d.EventTopics()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	expectedSet := map[common.Hash]struct{}{
		parsedABI.Events["ContractCall"].ID:         {},
		parsedABI.Events["ContractCallApproved"].ID: {},
		parsedABI.Events["Executed"].ID:             {},
	}

	require.Len(t, topics, len(expectedSet))
	for _, topic := range topics {
		_, ok := expectedSet[topic]
		assert.True(t, ok, "unexpected topic %s", topic.Hex())
		delete(expectedSet, topic)
	}
	assert.Empty(t, expectedSet)
}

func TestDecodeContractCall(t *testing.T) {
	d := NewAxelarDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	contractCallEvent := parsedABI.Events["ContractCall"]

	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	payloadHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	payload := []byte{0xde, 0xad, 0xbe, 0xef}

	data, err := contractCallEvent.Inputs.NonIndexed().Pack(
		"Avalanche",
		"0x2222222222222222222222222222222222222222",
		payload,
	)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			contractCallEvent.ID,
			common.BytesToHash(sender.Bytes()),
			payloadHash,
		},
		Data:        data,
		BlockNumber: 19000000,
		Index:       5,
	}

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "ContractCall", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(1), rawEvent.ChainID)
	assert.Equal(t, uint64(19000000), rawEvent.BlockNumber)

	assert.Equal(t, sender.Hex(), rawEvent.Data["sender"])
	assert.Equal(t, payloadHash.Hex(), rawEvent.Data["payload_hash"])
	assert.Equal(t, "Avalanche", rawEvent.Data["destination_chain"])
	assert.Equal(t, "43114", rawEvent.Data["dst_chain_id"])
	assert.Equal(t, "0x2222222222222222222222222222222222222222", rawEvent.Data["destination_contract_address"])
	assert.Equal(t, "deadbeef", rawEvent.Data["payload"])
}

func TestDecodeContractCall_UnknownChain(t *testing.T) {
	d := NewAxelarDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	contractCallEvent := parsedABI.Events["ContractCall"]

	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	payloadHash := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")

	data, err := contractCallEvent.Inputs.NonIndexed().Pack(
		"SomeUnknownChain",
		"0x2222222222222222222222222222222222222222",
		[]byte{0x01},
	)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			contractCallEvent.ID,
			common.BytesToHash(sender.Bytes()),
			payloadHash,
		},
		Data: data,
	}

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "SomeUnknownChain", rawEvent.Data["destination_chain"])
	assert.Equal(t, "unknown", rawEvent.Data["dst_chain_id"])
}

func TestDecodeContractCallApproved(t *testing.T) {
	d := NewAxelarDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	approvedEvent := parsedABI.Events["ContractCallApproved"]

	commandId := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	contractAddress := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sourceTxHash := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	payloadHash := [32]byte{}
	copy(payloadHash[:], common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd").Bytes())

	data, err := approvedEvent.Inputs.NonIndexed().Pack(
		"ethereum",
		"0x1111111111111111111111111111111111111111",
		payloadHash,
		big.NewInt(7),
	)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			approvedEvent.ID,
			commandId,
			common.BytesToHash(contractAddress.Bytes()),
			sourceTxHash,
		},
		Data:        data,
		BlockNumber: 50000000,
		Index:       2,
	}

	rawEvent, err := d.Decode(log, 43114)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "ContractCallApproved", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(43114), rawEvent.ChainID)

	assert.Equal(t, commandId.Hex(), rawEvent.Data["command_id"])
	assert.Equal(t, contractAddress.Hex(), rawEvent.Data["contract_address"])
	assert.Equal(t, sourceTxHash.Hex(), rawEvent.Data["source_tx_hash"])
	assert.Equal(t, "ethereum", rawEvent.Data["source_chain"])
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"])
	assert.Equal(t, "0x1111111111111111111111111111111111111111", rawEvent.Data["source_address"])
	assert.Equal(t, common.BytesToHash(payloadHash[:]).Hex(), rawEvent.Data["payload_hash"])
	assert.Equal(t, "7", rawEvent.Data["source_event_index"])
}

func TestDecodeContractCallApproved_UnknownChain(t *testing.T) {
	d := NewAxelarDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	approvedEvent := parsedABI.Events["ContractCallApproved"]

	commandId := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	contractAddress := common.HexToAddress("0x3333333333333333333333333333333333333333")
	sourceTxHash := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	payloadHash := [32]byte{}

	data, err := approvedEvent.Inputs.NonIndexed().Pack(
		"UnknownChainXYZ",
		"0x1111111111111111111111111111111111111111",
		payloadHash,
		big.NewInt(0),
	)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			approvedEvent.ID,
			commandId,
			common.BytesToHash(contractAddress.Bytes()),
			sourceTxHash,
		},
		Data: data,
	}

	rawEvent, err := d.Decode(log, 43114)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "UnknownChainXYZ", rawEvent.Data["source_chain"])
	assert.Equal(t, "unknown", rawEvent.Data["src_chain_id"])
}

func TestDecodeExecuted(t *testing.T) {
	d := NewAxelarDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(axelarABI))
	require.NoError(t, err)

	executedEvent := parsedABI.Events["Executed"]

	commandId := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")

	log := ethtypes.Log{
		Topics: []common.Hash{
			executedEvent.ID,
			commandId,
		},
		BlockNumber: 50000100,
		Index:       8,
	}

	rawEvent, err := d.Decode(log, 43114)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "Executed", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(43114), rawEvent.ChainID)
	assert.Equal(t, commandId.Hex(), rawEvent.Data["command_id"])
}

func TestDecode_NoTopics(t *testing.T) {
	d := NewAxelarDecoder()

	log := ethtypes.Log{
		Topics: []common.Hash{},
	}

	_, err := d.Decode(log, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no topics")
}

func TestDecode_UnknownTopic(t *testing.T) {
	d := NewAxelarDecoder()

	log := ethtypes.Log{
		Topics: []common.Hash{
			common.HexToHash("0x0000000000000000000000000000000000000000000000000000000000000001"),
		},
	}

	_, err := d.Decode(log, 1)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event topic")
}

func TestChainNameMapping(t *testing.T) {
	// Verify all major EVM chains have mappings.
	expectedChains := map[string]uint64{
		"ethereum":  1,
		"Ethereum":  1,
		"avalanche": 43114,
		"Avalanche": 43114,
		"Polygon":   137,
		"polygon":   137,
		"binance":   56,
		"Binance":   56,
		"arbitrum":  42161,
		"Arbitrum":  42161,
		"optimism":  10,
		"Optimism":  10,
		"base":      8453,
		"Base":      8453,
		"Fantom":    250,
		"fantom":    250,
	}

	for name, expectedID := range expectedChains {
		id, ok := ChainNameToID[name]
		assert.True(t, ok, "chain name %q should be mapped", name)
		assert.Equal(t, expectedID, id, "chain name %q should map to %d", name, expectedID)
	}

	// Verify reverse mapping exists for all IDs.
	expectedIDs := []uint64{1, 43114, 137, 56, 42161, 10, 8453, 250}
	for _, id := range expectedIDs {
		name, ok := ChainIDToName[id]
		assert.True(t, ok, "chain ID %d should have reverse mapping", id)
		assert.NotEmpty(t, name, "chain ID %d should have non-empty name", id)
	}
}

func TestGatewayAddresses(t *testing.T) {
	// Verify all expected chains have gateway addresses.
	expectedChains := []uint64{1, 43114, 137, 56, 42161, 10, 8453, 250}
	for _, chainID := range expectedChains {
		addr, ok := GatewayAddresses[chainID]
		assert.True(t, ok, "chain ID %d should have a gateway address", chainID)
		assert.NotEqual(t, common.Address{}, addr, "chain ID %d gateway address should not be zero", chainID)
	}
}
