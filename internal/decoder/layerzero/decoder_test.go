package layerzero

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
	d := NewLayerZeroDecoder()
	assert.Equal(t, ProtocolName, d.Protocol())
}

func TestDecoderContractAddresses(t *testing.T) {
	d := NewLayerZeroDecoder()
	addrs := d.ContractAddresses(1)
	assert.NotEmpty(t, addrs)
}

func TestDecoderEventTopics(t *testing.T) {
	d := NewLayerZeroDecoder()
	topics := d.EventTopics()
	assert.Len(t, topics, 4)
}

func TestDecodePacketSent(t *testing.T) {
	d := NewLayerZeroDecoder()

	// Prepare encodedPayload
	payload := make([]byte, 113)
	payload[0] = 1                                         // version
	new(big.Int).SetUint64(123).FillBytes(payload[1:9])    // nonce
	new(big.Int).SetUint64(30101).FillBytes(payload[9:13]) // srcEid
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	copy(payload[13:45], common.LeftPadBytes(sender.Bytes(), 32)) // sender
	new(big.Int).SetUint64(30102).FillBytes(payload[45:49])       // dstEid
	receiver := common.HexToAddress("0x2222222222222222222222222222222222222222")
	copy(payload[49:81], common.LeftPadBytes(receiver.Bytes(), 32)) // receiver
	guid := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")
	copy(payload[81:113], guid.Bytes()) // guid

	// Prepare PacketSent log
	parsedABI, err := abi.JSON(strings.NewReader(lzABI))
	require.NoError(t, err)

	packetSentEvent := parsedABI.Events["PacketSent"]
	data, err := packetSentEvent.Inputs.Pack(payload, []byte("options"), sender)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{packetSentEvent.ID},
		Data:   data,
		Index:  1,
	}

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "PacketSent", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(1), rawEvent.ChainID)

	assert.Equal(t, "1", rawEvent.Data["version"])
	assert.Equal(t, "123", rawEvent.Data["nonce"])
	assert.Equal(t, "30101", rawEvent.Data["src_eid"])
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"])
	assert.Equal(t, "30102", rawEvent.Data["dst_eid"])
	assert.Equal(t, "56", rawEvent.Data["dst_chain_id"])
	assert.Equal(t, sender.Hex(), rawEvent.Data["sender"])
	assert.Equal(t, receiver.Hex(), rawEvent.Data["receiver"])
	assert.Equal(t, guid.Hex(), rawEvent.Data["guid"])
}

func TestDecodePacketDelivered(t *testing.T) {
	d := NewLayerZeroDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(lzABI))
	require.NoError(t, err)

	packetDeliveredEvent := parsedABI.Events["PacketDelivered"]

	sender := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	receiver := common.HexToAddress("0x2222222222222222222222222222222222222222")

	origin := struct {
		SrcEid uint32
		Sender [32]byte
		Nonce  uint64
	}{
		SrcEid: 30101,
		Sender: sender,
		Nonce:  123,
	}

	data, err := packetDeliveredEvent.Inputs.Pack(origin, receiver)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{packetDeliveredEvent.ID},
		Data:   data,
	}

	rawEvent, err := d.Decode(log, 56)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "PacketReceived", rawEvent.EventType)
	assert.Equal(t, "30101", rawEvent.Data["src_eid"])
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"])
	assert.Equal(t, "123", rawEvent.Data["nonce"])
}

func TestDecodeOFTSent(t *testing.T) {
	d := NewLayerZeroDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(lzABI))
	require.NoError(t, err)

	oftSentEvent := parsedABI.Events["OFTSent"]

	guid := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")
	fromAddress := common.HexToAddress("0x1111111111111111111111111111111111111111")

	amountSent := big.NewInt(1000)
	amountReceived := big.NewInt(900)

	data, err := oftSentEvent.Inputs.NonIndexed().Pack(uint32(30102), amountSent, amountReceived)
	require.NoError(t, err)

	log := ethtypes.Log{
		Topics: []common.Hash{
			oftSentEvent.ID,
			guid,
			common.BytesToHash(fromAddress.Bytes()),
		},
		Data: data,
	}

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "OFTSent", rawEvent.EventType)
	assert.Equal(t, guid.Hex(), rawEvent.Data["guid"])
	assert.Equal(t, fromAddress.Hex(), rawEvent.Data["from_address"])
	assert.Equal(t, "30102", rawEvent.Data["dst_eid"])
	assert.Equal(t, "56", rawEvent.Data["dst_chain_id"])
	assert.Equal(t, "1000", rawEvent.Data["amount_sent"])
	assert.Equal(t, "900", rawEvent.Data["amount_received"])
}
