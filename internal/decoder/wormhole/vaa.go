package wormhole

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// TokenBridgePayloadType identifies the type of token bridge transfer.
// Reference: https://github.com/wormhole-foundation/wormhole/blob/main/whitepapers/0003_token_bridge.md
type TokenBridgePayloadType uint8

const (
	// PayloadTypeTransfer is a simple token transfer (type 1).
	PayloadTypeTransfer TokenBridgePayloadType = 1
	// PayloadTypeAttestMeta is a token attestation (type 2) — not indexed.
	PayloadTypeAttestMeta TokenBridgePayloadType = 2
	// PayloadTypeTransferWithMessage is a transfer with arbitrary message (type 3).
	PayloadTypeTransferWithMessage TokenBridgePayloadType = 3
)

// TokenTransferPayload represents a decoded Wormhole token bridge transfer.
// This is the payload inside the VAA for payload type 1 and type 3.
//
// Wire format (all big-endian):
//
//	Offset  Size  Field
//	0       1     payloadID      (1 = Transfer, 3 = TransferWithMessage)
//	1       32    amount         (uint256, in token decimals)
//	33      32    tokenAddress   (source chain token address, zero-padded to 32 bytes)
//	65      2     tokenChain     (Wormhole chain ID of the token's origin)
//	67      32    toAddress      (recipient address on destination chain, zero-padded)
//	99      2     toChain        (Wormhole chain ID of the destination)
//	101     2     fee            (relayer fee, uint256 — only present in type 1)
//	                              For type 3, bytes 101+ contain the arbitrary message.
//
// Reference: wormhole/sdk/js/src/token_bridge/transfer.ts
const (
	transferPayloadMinLen = 101
	payloadIDOffset       = 0
	amountOffset          = 1
	tokenAddressOffset    = 33
	tokenChainOffset      = 65
	toAddressOffset       = 67
	toChainOffset         = 99
	feeOffset             = 101
)

// TokenTransferPayload holds the decoded fields from a Wormhole token bridge VAA payload.
type TokenTransferPayload struct {
	// PayloadType is 1 (Transfer) or 3 (TransferWithMessage).
	PayloadType TokenBridgePayloadType

	// Amount is the transferred token amount as a hex string (uint256 big-endian).
	Amount string

	// TokenAddress is the source token contract address (checksummed EVM address).
	TokenAddress string

	// TokenChain is the Wormhole chain ID where the token originates.
	TokenChain uint16

	// TokenChainEVM is the EVM chain ID mapped from TokenChain (0 if unknown).
	TokenChainEVM uint64

	// ToAddress is the recipient address on the destination chain.
	ToAddress string

	// ToChain is the Wormhole destination chain ID.
	ToChain uint16

	// ToChainEVM is the EVM chain ID mapped from ToChain (0 if unknown).
	ToChainEVM uint64

	// Fee is the relayer fee (only present in type 1 transfers).
	Fee string

	// ExtraPayload is the arbitrary message bytes (only present in type 3 transfers).
	ExtraPayload []byte
}

// ParseTokenBridgePayload attempts to decode a Wormhole token bridge payload.
// Returns nil, nil for non-transfer payloads (attestation, unknown types).
// Returns an error only if the payload looks like a transfer but is malformed.
func ParseTokenBridgePayload(payload []byte) (*TokenTransferPayload, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	payloadType := TokenBridgePayloadType(payload[payloadIDOffset])

	// Only decode transfer types; skip attestation and unknown types silently.
	if payloadType != PayloadTypeTransfer && payloadType != PayloadTypeTransferWithMessage {
		return nil, nil
	}

	if len(payload) < transferPayloadMinLen {
		return nil, fmt.Errorf("wormhole token bridge payload too short: got %d bytes, need at least %d",
			len(payload), transferPayloadMinLen)
	}

	t := &TokenTransferPayload{
		PayloadType: payloadType,
	}

	// amount: bytes 1-32 (uint256 big-endian) — format as hex for storage
	// We don't use math/big here to avoid the dependency in this file;
	// the raw 32-byte hex is stored and can be parsed by consumers.
	amountBytes := payload[amountOffset : amountOffset+32]
	t.Amount = fmt.Sprintf("0x%x", amountBytes)

	// tokenAddress: bytes 33-64 — last 20 bytes are the EVM address
	tokenAddrBytes := payload[tokenAddressOffset : tokenAddressOffset+32]
	t.TokenAddress = common.BytesToAddress(tokenAddrBytes[12:]).Hex()

	// tokenChain: bytes 65-66
	t.TokenChain = binary.BigEndian.Uint16(payload[tokenChainOffset : tokenChainOffset+2])
	if evmID, ok := WormholeChainIDToEVM[t.TokenChain]; ok {
		t.TokenChainEVM = evmID
	}

	// toAddress: bytes 67-98 — last 20 bytes are the EVM address
	toAddrBytes := payload[toAddressOffset : toAddressOffset+32]
	t.ToAddress = common.BytesToAddress(toAddrBytes[12:]).Hex()

	// toChain: bytes 99-100
	t.ToChain = binary.BigEndian.Uint16(payload[toChainOffset : toChainOffset+2])
	if evmID, ok := WormholeChainIDToEVM[t.ToChain]; ok {
		t.ToChainEVM = evmID
	}

	switch payloadType {
	case PayloadTypeTransfer:
		// fee: bytes 101-132 (uint256)
		if len(payload) >= feeOffset+32 {
			feeBytes := payload[feeOffset : feeOffset+32]
			t.Fee = fmt.Sprintf("0x%x", feeBytes)
		}

	case PayloadTypeTransferWithMessage:
		// bytes 101+ are the arbitrary message
		if len(payload) > feeOffset {
			t.ExtraPayload = payload[feeOffset:]
		}
	}

	return t, nil
}