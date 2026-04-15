package wormhole

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTransferPayload constructs a valid type-1 token bridge payload for testing.
func buildTransferPayload(
	amount [32]byte,
	tokenAddr [32]byte,
	tokenChain uint16,
	toAddr [32]byte,
	toChain uint16,
	fee [32]byte,
) []byte {
	buf := make([]byte, 133) // 1 + 32 + 32 + 2 + 32 + 2 + 32
	buf[0] = byte(PayloadTypeTransfer)
	copy(buf[1:33], amount[:])
	copy(buf[33:65], tokenAddr[:])
	binary.BigEndian.PutUint16(buf[65:67], tokenChain)
	copy(buf[67:99], toAddr[:])
	binary.BigEndian.PutUint16(buf[99:101], toChain)
	copy(buf[101:133], fee[:])
	return buf
}

// buildTransferWithMessagePayload constructs a type-3 payload.
func buildTransferWithMessagePayload(
	amount [32]byte,
	tokenAddr [32]byte,
	tokenChain uint16,
	toAddr [32]byte,
	toChain uint16,
	message []byte,
) []byte {
	buf := make([]byte, 101+len(message))
	buf[0] = byte(PayloadTypeTransferWithMessage)
	copy(buf[1:33], amount[:])
	copy(buf[33:65], tokenAddr[:])
	binary.BigEndian.PutUint16(buf[65:67], tokenChain)
	copy(buf[67:99], toAddr[:])
	binary.BigEndian.PutUint16(buf[99:101], toChain)
	copy(buf[101:], message)
	return buf
}

func TestParseTokenBridgePayload_Type1(t *testing.T) {
	var amount [32]byte
	// Encode 1000 USDC (6 decimals) = 1000_000000
	binary.BigEndian.PutUint64(amount[24:], 1_000_000_000)

	// USDC on Ethereum: 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48
	var tokenAddr [32]byte
	tokenAddrBytes := [20]byte{0xA0, 0xb8, 0x69, 0x91, 0xc6, 0x21, 0x8b, 0x36, 0xc1, 0xd1,
		0x9D, 0x4a, 0x2e, 0x9E, 0xb0, 0xce, 0x36, 0x06, 0xeB, 0x48}
	copy(tokenAddr[12:], tokenAddrBytes[:])

	// Recipient on Arbitrum
	var toAddr [32]byte
	recipientBytes := [20]byte{0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22,
		0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22, 0x22}
	copy(toAddr[12:], recipientBytes[:])

	var fee [32]byte

	payload := buildTransferPayload(amount, tokenAddr, 2, toAddr, 23, fee)

	result, err := ParseTokenBridgePayload(payload)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, PayloadTypeTransfer, result.PayloadType)
	assert.Equal(t, uint16(2), result.TokenChain)
	assert.Equal(t, uint64(1), result.TokenChainEVM) // Wormhole 2 = Ethereum
	assert.Equal(t, uint16(23), result.ToChain)
	assert.Equal(t, uint64(42161), result.ToChainEVM) // Wormhole 23 = Arbitrum
	assert.Equal(t, "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48", result.TokenAddress)
	assert.Equal(t, "0x2222222222222222222222222222222222222222", result.ToAddress)
	assert.NotEmpty(t, result.Amount)
	assert.NotEmpty(t, result.Fee)
	assert.Nil(t, result.ExtraPayload)
}

func TestParseTokenBridgePayload_Type3(t *testing.T) {
	var amount, tokenAddr, toAddr [32]byte
	message := []byte("hello from wormhole")

	payload := buildTransferWithMessagePayload(amount, tokenAddr, 2, toAddr, 23, message)

	result, err := ParseTokenBridgePayload(payload)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, PayloadTypeTransferWithMessage, result.PayloadType)
	assert.Equal(t, message, result.ExtraPayload)
	assert.Empty(t, result.Fee)
}

func TestParseTokenBridgePayload_Attestation(t *testing.T) {
	// Type 2 = attestation — should be silently skipped (nil, nil)
	payload := make([]byte, 150)
	payload[0] = byte(PayloadTypeAttestMeta)

	result, err := ParseTokenBridgePayload(payload)
	require.NoError(t, err)
	assert.Nil(t, result, "attestation payloads should return nil")
}

func TestParseTokenBridgePayload_Empty(t *testing.T) {
	result, err := ParseTokenBridgePayload([]byte{})
	require.NoError(t, err)
	assert.Nil(t, result)

	result, err = ParseTokenBridgePayload(nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestParseTokenBridgePayload_TooShort(t *testing.T) {
	// Type 1 but truncated — should error
	payload := make([]byte, 50)
	payload[0] = byte(PayloadTypeTransfer)

	_, err := ParseTokenBridgePayload(payload)
	assert.Error(t, err)
}

func TestParseTokenBridgePayload_UnknownType(t *testing.T) {
	payload := make([]byte, 150)
	payload[0] = 99 // unknown type

	result, err := ParseTokenBridgePayload(payload)
	require.NoError(t, err)
	assert.Nil(t, result, "unknown payload types should return nil")
}

func TestParseTokenBridgePayload_UnknownChain(t *testing.T) {
	var amount, tokenAddr, toAddr, fee [32]byte
	// Use chain IDs not in WormholeChainIDToEVM
	payload := buildTransferPayload(amount, tokenAddr, 999, toAddr, 888, fee)

	result, err := ParseTokenBridgePayload(payload)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, uint64(0), result.TokenChainEVM, "unknown chain should have EVM ID 0")
	assert.Equal(t, uint64(0), result.ToChainEVM, "unknown chain should have EVM ID 0")
}
