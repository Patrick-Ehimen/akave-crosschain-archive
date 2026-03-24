package ccip

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

// buildEVM2EVMMessage constructs an ABI-encoded CCIPSendRequested log Data field
// using the EVM2EVMMessage tuple defined in ccipABI.
func buildCCIPSendRequestedData(t *testing.T, msg evm2EVMMessage) []byte {
	t.Helper()

	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)

	event := parsedABI.Events["CCIPSendRequested"]

	// Build the struct map that matches the ABI tuple layout.
	// go-ethereum's abi package can encode tuples from Go structs via Copy/Pack.
	// We use a map of field name → value to pack the tuple.
	tokenAmounts := make([]struct {
		Token  common.Address
		Amount *big.Int
	}, len(msg.TokenAmounts))
	for i, ta := range msg.TokenAmounts {
		tokenAmounts[i].Token = ta.Token
		tokenAmounts[i].Amount = ta.Amount
	}

	packed, err := event.Inputs.Pack(msg)
	require.NoError(t, err)
	return packed
}

// newTestDecoder returns a CCIPDecoder for use in tests.
func newTestDecoder() *CCIPDecoder {
	return NewCCIPDecoder()
}

// ─── Protocol / registration ───────────────────────────────────────────────

func TestDecoderProtocol(t *testing.T) {
	d := newTestDecoder()
	assert.Equal(t, ProtocolName, d.Protocol())
}

func TestDecoderEventTopics(t *testing.T) {
	d := newTestDecoder()
	topics := d.EventTopics()
	require.Len(t, topics, 2)

	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)

	assert.Equal(t, parsedABI.Events["CCIPSendRequested"].ID, topics[0])
	assert.Equal(t, parsedABI.Events["ExecutionStateChanged"].ID, topics[1])
}

func TestDecoderContractAddresses_KnownChain(t *testing.T) {
	d := newTestDecoder()
	// Ethereum has several OnRamps and OffRamps configured.
	addrs := d.ContractAddresses(1)
	assert.NotEmpty(t, addrs)

	// All addresses must be distinct (deduplication check).
	seen := make(map[common.Address]struct{})
	for _, addr := range addrs {
		_, dup := seen[addr]
		assert.False(t, dup, "duplicate address %s", addr.Hex())
		seen[addr] = struct{}{}
	}
}

func TestDecoderContractAddresses_ArbitrumHasAddresses(t *testing.T) {
	d := newTestDecoder()
	addrs := d.ContractAddresses(42161)
	assert.NotEmpty(t, addrs)
}

func TestDecoderContractAddresses_UnknownChain(t *testing.T) {
	d := newTestDecoder()
	addrs := d.ContractAddresses(999999)
	assert.Empty(t, addrs)
}

// ─── Chain selector mapping ────────────────────────────────────────────────

func TestChainSelectorToEVM_MajorChains(t *testing.T) {
	expected := map[uint64]uint64{
		5009297550715157269:  1,     // Ethereum
		4949039107694359620:  42161, // Arbitrum
		3734403246176062136:  10,    // Optimism
		15971525489660198786: 8453,  // Base
		4051577828743386545:  137,   // Polygon
		6433500567565415381:  43114, // Avalanche
		11344663589394136015: 56,    // BSC
	}
	for sel, wantEVM := range expected {
		gotEVM, ok := ChainSelectorToEVM[sel]
		assert.True(t, ok, "selector %d should be mapped", sel)
		assert.Equal(t, wantEVM, gotEVM, "selector %d → wrong EVM ID", sel)
	}
}

func TestEVMToChainSelector_Roundtrip(t *testing.T) {
	for sel, evmID := range ChainSelectorToEVM {
		gotSel, ok := EVMToChainSelector[evmID]
		assert.True(t, ok, "EVM chain %d should have a reverse selector", evmID)
		assert.Equal(t, sel, gotSel, "round-trip mismatch for EVM chain %d", evmID)
	}
}

// ─── ExecutionState ────────────────────────────────────────────────────────

func TestExecutionState_String(t *testing.T) {
	cases := []struct {
		state ExecutionState
		want  string
	}{
		{ExecutionStateUntouched, "untouched"},
		{ExecutionStateInProgress, "in_progress"},
		{ExecutionStateSuccess, "success"},
		{ExecutionStateFailure, "failure"},
		{ExecutionState(99), "unknown(99)"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.state.String())
	}
}

// ─── CCIPSendRequested decoding ────────────────────────────────────────────

func buildSendRequestedLog(t *testing.T, msg evm2EVMMessage) ethtypes.Log {
	t.Helper()
	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)
	event := parsedABI.Events["CCIPSendRequested"]
	data, err := event.Inputs.Pack(msg)
	require.NoError(t, err)
	return ethtypes.Log{
		Topics:      []common.Hash{event.ID},
		Data:        data,
		BlockNumber: 19500000,
		Index:       3,
	}
}

func TestDecodeCCIPSendRequested_BasicMessage(t *testing.T) {
	d := newTestDecoder()

	msgID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	receiver := common.HexToAddress("0x2222222222222222222222222222222222222222")
	feeToken := common.HexToAddress("0x3333333333333333333333333333333333333333")

	var msgIDArr [32]byte
	copy(msgIDArr[:], msgID.Bytes())

	msg := evm2EVMMessage{
		SequenceNumber:      42,
		FeeTokenAmount:      big.NewInt(100000000000000),
		Sender:              sender,
		Nonce:               7,
		GasLimit:            big.NewInt(200000),
		Strict:              false,
		SourceChainSelector: 5009297550715157269, // Ethereum
		Receiver:            receiver,
		Data:                []byte{0xde, 0xad, 0xbe, 0xef},
		TokenAmounts:        []evmTokenAmount{},
		FeeToken:            feeToken,
		MessageId:           msgIDArr,
	}

	log := buildSendRequestedLog(t, msg)
	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "CCIPSendRequested", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(1), rawEvent.ChainID)
	assert.Equal(t, uint64(19500000), rawEvent.BlockNumber)

	assert.Equal(t, msgID.Hex(), rawEvent.Data["message_id"])
	assert.Equal(t, "42", rawEvent.Data["sequence_number"])
	assert.Equal(t, sender.Hex(), rawEvent.Data["sender"])
	assert.Equal(t, receiver.Hex(), rawEvent.Data["receiver"])
	assert.Equal(t, "7", rawEvent.Data["nonce"])
	assert.Equal(t, "5009297550715157269", rawEvent.Data["source_chain_selector"])
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"]) // Ethereum selector → EVM 1
	assert.Equal(t, feeToken.Hex(), rawEvent.Data["fee_token"])
	assert.Equal(t, "100000000000000", rawEvent.Data["fee_token_amount"])
	assert.Equal(t, "200000", rawEvent.Data["gas_limit"])
	assert.Equal(t, "0xdeadbeef", rawEvent.Data["data"])
	assert.Equal(t, "0", rawEvent.Data["token_count"])
}

func TestDecodeCCIPSendRequested_WithTokenTransfer(t *testing.T) {
	d := newTestDecoder()

	var msgIDArr [32]byte
	msgIDArr[31] = 0x42

	tokenAddr := common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48") // USDC
	tokenAmount := big.NewInt(1000_000000) // 1000 USDC

	msg := evm2EVMMessage{
		SequenceNumber:      1,
		FeeTokenAmount:      big.NewInt(50000000000000),
		Sender:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:               1,
		GasLimit:            big.NewInt(150000),
		Strict:              false,
		SourceChainSelector: 5009297550715157269,
		Receiver:            common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Data:                []byte{},
		TokenAmounts: []evmTokenAmount{
			{Token: tokenAddr, Amount: tokenAmount},
		},
		FeeToken:  common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MessageId: msgIDArr,
	}

	log := buildSendRequestedLog(t, msg)
	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)

	assert.Equal(t, "1", rawEvent.Data["token_count"])
	assert.Equal(t, tokenAddr.Hex(), rawEvent.Data["token"])
	assert.Equal(t, "1000000000", rawEvent.Data["amount"])
	assert.Equal(t, "0x", rawEvent.Data["data"])
}

func TestDecodeCCIPSendRequested_EmptyPayload(t *testing.T) {
	d := newTestDecoder()

	var msgIDArr [32]byte
	msg := evm2EVMMessage{
		SequenceNumber:      5,
		FeeTokenAmount:      big.NewInt(1),
		Sender:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:               5,
		GasLimit:            big.NewInt(100000),
		Strict:              false,
		SourceChainSelector: 4949039107694359620, // Arbitrum
		Receiver:            common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Data:                []byte{},
		TokenAmounts:        []evmTokenAmount{},
		FeeToken:            common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MessageId:           msgIDArr,
	}

	log := buildSendRequestedLog(t, msg)
	rawEvent, err := d.Decode(log, 42161)
	require.NoError(t, err)

	assert.Equal(t, "0x", rawEvent.Data["data"])
	assert.Equal(t, "0", rawEvent.Data["token_count"])
	// Arbitrum selector → EVM 42161
	assert.Equal(t, "42161", rawEvent.Data["src_chain_id"])
}

func TestDecodeCCIPSendRequested_UnknownSelector(t *testing.T) {
	d := newTestDecoder()

	var msgIDArr [32]byte
	msg := evm2EVMMessage{
		SequenceNumber:      1,
		FeeTokenAmount:      big.NewInt(1),
		Sender:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:               1,
		GasLimit:            big.NewInt(100000),
		Strict:              false,
		SourceChainSelector: 9999999999999999999, // unknown
		Receiver:            common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Data:                []byte{},
		TokenAmounts:        []evmTokenAmount{},
		FeeToken:            common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MessageId:           msgIDArr,
	}

	log := buildSendRequestedLog(t, msg)
	// observed on chainID 1 — fallback to that
	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)

	assert.Equal(t, "9999999999999999999", rawEvent.Data["source_chain_selector"])
	// Falls back to the chain the event was observed on
	assert.Equal(t, "1", rawEvent.Data["src_chain_id"])
}

func TestDecodeCCIPSendRequested_MalformedData(t *testing.T) {
	d := newTestDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)
	event := parsedABI.Events["CCIPSendRequested"]

	log := ethtypes.Log{
		Topics: []common.Hash{event.ID},
		Data:   []byte{0x01, 0x02, 0x03}, // truncated
	}

	_, err = d.Decode(log, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to unpack")
}

// ─── ExecutionStateChanged decoding ───────────────────────────────────────

func buildExecutionStateChangedLog(
	t *testing.T,
	seqNum uint64,
	messageID common.Hash,
	state uint8,
	returnData []byte,
) ethtypes.Log {
	t.Helper()
	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)
	event := parsedABI.Events["ExecutionStateChanged"]

	data, err := event.Inputs.NonIndexed().Pack(state, returnData)
	require.NoError(t, err)

	seqTopic := common.BigToHash(new(big.Int).SetUint64(seqNum))

	return ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			seqTopic,
			messageID,
		},
		Data:        data,
		BlockNumber: 180000000,
		Index:       7,
	}
}

func TestDecodeExecutionStateChanged_Success(t *testing.T) {
	d := newTestDecoder()

	msgID := common.HexToHash("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	log := buildExecutionStateChangedLog(t, 42, msgID, uint8(ExecutionStateSuccess), nil)

	rawEvent, err := d.Decode(log, 42161)
	require.NoError(t, err)
	require.NotNil(t, rawEvent)

	assert.Equal(t, "ExecutionStateChanged", rawEvent.EventType)
	assert.Equal(t, ProtocolName, rawEvent.Protocol)
	assert.Equal(t, uint64(42161), rawEvent.ChainID)
	assert.Equal(t, uint64(180000000), rawEvent.BlockNumber)

	assert.Equal(t, "42", rawEvent.Data["sequence_number"])
	assert.Equal(t, msgID.Hex(), rawEvent.Data["message_id"])
	assert.Equal(t, "2", rawEvent.Data["state"])
	assert.Equal(t, "success", rawEvent.Data["state_name"])
	assert.Equal(t, "0x", rawEvent.Data["return_data"])
}

func TestDecodeExecutionStateChanged_Failure(t *testing.T) {
	d := newTestDecoder()

	msgID := common.HexToHash("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	returnData := []byte{0x08, 0xc3, 0x79, 0xa0} // Error(string) selector
	log := buildExecutionStateChangedLog(t, 99, msgID, uint8(ExecutionStateFailure), returnData)

	rawEvent, err := d.Decode(log, 1)
	require.NoError(t, err)

	assert.Equal(t, "99", rawEvent.Data["sequence_number"])
	assert.Equal(t, msgID.Hex(), rawEvent.Data["message_id"])
	assert.Equal(t, "3", rawEvent.Data["state"])
	assert.Equal(t, "failure", rawEvent.Data["state_name"])
	assert.Equal(t, "0x08c379a0", rawEvent.Data["return_data"])
}

func TestDecodeExecutionStateChanged_InProgress(t *testing.T) {
	d := newTestDecoder()

	msgID := common.HexToHash("0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc")
	log := buildExecutionStateChangedLog(t, 1, msgID, uint8(ExecutionStateInProgress), nil)

	rawEvent, err := d.Decode(log, 10)
	require.NoError(t, err)

	assert.Equal(t, "1", rawEvent.Data["state"])
	assert.Equal(t, "in_progress", rawEvent.Data["state_name"])
}

func TestDecodeExecutionStateChanged_Untouched(t *testing.T) {
	d := newTestDecoder()

	msgID := common.HexToHash("0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd")
	log := buildExecutionStateChangedLog(t, 7, msgID, uint8(ExecutionStateUntouched), nil)

	rawEvent, err := d.Decode(log, 8453)
	require.NoError(t, err)

	assert.Equal(t, "0", rawEvent.Data["state"])
	assert.Equal(t, "untouched", rawEvent.Data["state_name"])
}

func TestDecodeExecutionStateChanged_InsufficientTopics(t *testing.T) {
	d := newTestDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)
	event := parsedABI.Events["ExecutionStateChanged"]

	log := ethtypes.Log{
		// Only 2 topics instead of required 3+
		Topics: []common.Hash{event.ID, common.Hash{}},
		Data:   []byte{},
	}

	_, err = d.Decode(log, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expected at least 3 topics")
}

func TestDecodeExecutionStateChanged_MalformedData(t *testing.T) {
	d := newTestDecoder()

	parsedABI, err := abi.JSON(strings.NewReader(ccipABI))
	require.NoError(t, err)
	event := parsedABI.Events["ExecutionStateChanged"]

	log := ethtypes.Log{
		Topics: []common.Hash{
			event.ID,
			common.BigToHash(big.NewInt(1)),
			common.HexToHash("0xaaaa"),
		},
		Data: []byte{0x99}, // incomplete
	}

	_, err = d.Decode(log, 1)
	assert.Error(t, err)
}

// ─── Error paths ───────────────────────────────────────────────────────────

func TestDecode_NoTopics(t *testing.T) {
	d := newTestDecoder()

	log := ethtypes.Log{
		Topics: []common.Hash{},
		Data:   []byte{},
	}

	_, err := d.Decode(log, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no topics")
}

func TestDecode_UnknownTopic(t *testing.T) {
	d := newTestDecoder()

	log := ethtypes.Log{
		Topics: []common.Hash{
			common.HexToHash("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"),
		},
		Data: []byte{},
	}

	_, err := d.Decode(log, 1)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown event topic")
}

// ─── Message ID correlation consistency ───────────────────────────────────

// TestMessageIDConsistency verifies that the messageId extracted from
// CCIPSendRequested and ExecutionStateChanged use the same hex format,
// which is essential for FindByCorrelationKey to match them in the DB.
func TestMessageIDConsistency_SendAndExecution(t *testing.T) {
	d := newTestDecoder()

	// The message ID as a bytes32.
	msgID := common.HexToHash("0xabcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890")

	// Build CCIPSendRequested with this messageId.
	var msgIDArr [32]byte
	copy(msgIDArr[:], msgID.Bytes())

	sendMsg := evm2EVMMessage{
		SequenceNumber:      100,
		FeeTokenAmount:      big.NewInt(1),
		Sender:              common.HexToAddress("0x1111111111111111111111111111111111111111"),
		Nonce:               100,
		GasLimit:            big.NewInt(200000),
		Strict:              false,
		SourceChainSelector: 5009297550715157269,
		Receiver:            common.HexToAddress("0x2222222222222222222222222222222222222222"),
		Data:                []byte{},
		TokenAmounts:        []evmTokenAmount{},
		FeeToken:            common.HexToAddress("0x3333333333333333333333333333333333333333"),
		MessageId:           msgIDArr,
	}

	sendLog := buildSendRequestedLog(t, sendMsg)
	sendRaw, err := d.Decode(sendLog, 1)
	require.NoError(t, err)

	// Build ExecutionStateChanged with the same messageId.
	execLog := buildExecutionStateChangedLog(t, 100, msgID, uint8(ExecutionStateSuccess), nil)
	execRaw, err := d.Decode(execLog, 42161)
	require.NoError(t, err)

	// Both must produce the exact same message_id string.
	assert.Equal(t, sendRaw.Data["message_id"], execRaw.Data["message_id"],
		"message_id format MUST be identical between CCIPSendRequested and "+
			"ExecutionStateChanged for DB correlation to work")
}