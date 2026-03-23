package wormhole

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

const ProtocolName = "wormhole"

// WormholeChainIDToEVM maps Wormhole's internal chain IDs to standard EVM chain IDs.
// Wormhole uses its own numbering (e.g. 2 = Ethereum, 4 = BSC) which must be
// translated to standard EVM chain IDs for the unified schema.
// Reference: https://docs.wormhole.com/wormhole/reference/constants
var WormholeChainIDToEVM = map[uint16]uint64{
	2:  1,      // Ethereum
	4:  56,     // BSC
	5:  137,    // Polygon
	6:  43114,  // Avalanche
	10: 250,    // Fantom
	13: 8217,   // Klaytn
	14: 42220,  // Celo
	16: 1284,   // Moonbeam
	23: 42161,  // Arbitrum
	24: 10,     // Optimism
	30: 8453,   // Base
	35: 534352, // Scroll
}

// EVMToWormholeChainID is the reverse mapping, built in init().
var EVMToWormholeChainID = map[uint64]uint16{}

func init() {
	for wh, evm := range WormholeChainIDToEVM {
		EVMToWormholeChainID[evm] = wh
	}
}

// CoreBridgeAddresses maps EVM chain IDs to the Wormhole Core Bridge contract address.
// The Core Bridge emits LogMessagePublished on the source chain.
// Reference: https://docs.wormhole.com/wormhole/reference/contract-addresses
var CoreBridgeAddresses = map[uint64]common.Address{
	1:      common.HexToAddress("0x98f3c9e6E3fAce36bAAd05FE09d375Ef1464288B"), // Ethereum
	56:     common.HexToAddress("0x98f3c9e6E3fAce36bAAd05FE09d375Ef1464288B"), // BSC
	137:    common.HexToAddress("0x7A4B5a56256163F07b2C80A7cA55aBE66c4ec4d7"), // Polygon
	43114:  common.HexToAddress("0x54a8e5f9c4CbA08F9943965859F6c34eAF03E26c"), // Avalanche
	42161:  common.HexToAddress("0xa5f208e072434bC67592E4C49C1B991BA79BCA46"), // Arbitrum
	10:     common.HexToAddress("0xEe91C335eab126dF5fDB3797EA9d6aD93aeC9722"), // Optimism
	8453:   common.HexToAddress("0xbebdb6C8ddC678FfA9f8748f85C815C556Dd8ac6"), // Base
	250:    common.HexToAddress("0x126783A6Cb203a3E35344528B26ca3a0489a1485"), // Fantom
}

// TokenBridgeAddresses maps EVM chain IDs to the Wormhole Token Bridge contract.
// The Token Bridge emits TransferRedeemed on the destination chain.
var TokenBridgeAddresses = map[uint64]common.Address{
	1:      common.HexToAddress("0x3ee18B2214AFF97000D974cf647E7C347E8fa585"), // Ethereum
	56:     common.HexToAddress("0xB6F6D86a8f9879A9c87f643768d9efc38c1Da6E7"), // BSC
	137:    common.HexToAddress("0x5a58505a96D1dbf8dF91cB21B54419FC36e93fdE"), // Polygon
	43114:  common.HexToAddress("0x0e082F06FF657D94310cB8cE8B0D9a04541d8052"), // Avalanche
	42161:  common.HexToAddress("0x0b2402144Bb366A632D14B83F244D2e0e21bD39c"), // Arbitrum
	10:     common.HexToAddress("0x1D68124e65faFC907325e3EDbF8c4d84499DAa8b"), // Optimism
	8453:   common.HexToAddress("0x8d2de8d2f73F1F4cAB472AC9A881C9b123C79627"), // Base
	250:    common.HexToAddress("0x7C9Fc5741288cDFdD83CeB07f3ea7e22618D79D2"), // Fantom
}

// wormholeABI defines the two events we care about:
//   - LogMessagePublished: emitted by Core Bridge on source chain
//   - TransferRedeemed:    emitted by Token Bridge on destination chain
const wormholeABI = `[
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true,  "internalType": "address", "name": "sender",           "type": "address"},
			{"indexed": false, "internalType": "uint64",  "name": "sequence",         "type": "uint64"},
			{"indexed": false, "internalType": "uint32",  "name": "nonce",            "type": "uint32"},
			{"indexed": false, "internalType": "bytes",   "name": "payload",          "type": "bytes"},
			{"indexed": false, "internalType": "uint8",   "name": "consistencyLevel", "type": "uint8"}
		],
		"name": "LogMessagePublished",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true,  "internalType": "uint16",  "name": "emitterChainId", "type": "uint16"},
			{"indexed": true,  "internalType": "bytes32", "name": "emitterAddress", "type": "bytes32"},
			{"indexed": true,  "internalType": "uint64",  "name": "sequence",       "type": "uint64"}
		],
		"name": "TransferRedeemed",
		"type": "event"
	}
]`

// WormholeDecoder implements the decoder.Decoder interface for Wormhole protocol.
// It handles LogMessagePublished (source) and TransferRedeemed (destination) events.
type WormholeDecoder struct {
	parsedABI abi.ABI
}

// NewWormholeDecoder creates and returns a new WormholeDecoder.
func NewWormholeDecoder() *WormholeDecoder {
	parsed, err := abi.JSON(strings.NewReader(wormholeABI))
	if err != nil {
		// ABI is embedded and validated — panic signals a programming error.
		panic(fmt.Errorf("failed to parse Wormhole ABI: %v", err))
	}
	return &WormholeDecoder{parsedABI: parsed}
}

// Protocol returns the protocol identifier.
func (d *WormholeDecoder) Protocol() string {
	return ProtocolName
}

// ContractAddresses returns both Core Bridge and Token Bridge addresses for a chain.
// Both contracts emit events we need to index.
func (d *WormholeDecoder) ContractAddresses(chainID uint64) []common.Address {
	var addrs []common.Address
	if addr, ok := CoreBridgeAddresses[chainID]; ok {
		addrs = append(addrs, addr)
	}
	if addr, ok := TokenBridgeAddresses[chainID]; ok {
		addrs = append(addrs, addr)
	}
	return addrs
}

// EventTopics returns the topic hashes for LogMessagePublished and TransferRedeemed.
func (d *WormholeDecoder) EventTopics() []common.Hash {
	return []common.Hash{
		d.parsedABI.Events["LogMessagePublished"].ID,
		d.parsedABI.Events["TransferRedeemed"].ID,
	}
}

// Decode parses a raw Ethereum log into a Wormhole-specific RawEvent.
func (d *WormholeDecoder) Decode(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
	if len(log.Topics) == 0 {
		return nil, fmt.Errorf("no topics in log")
	}

	eventID := log.Topics[0]

	rawEvent := &decoder.RawEvent{
		Protocol:    ProtocolName,
		ChainID:     chainID,
		BlockNumber: log.BlockNumber,
		TxHash:      log.TxHash.Hex(),
		LogIndex:    log.Index,
		Data:        make(map[string]string),
	}

	switch eventID {
	case d.parsedABI.Events["LogMessagePublished"].ID:
		return d.decodeLogMessagePublished(log, rawEvent, chainID)
	case d.parsedABI.Events["TransferRedeemed"].ID:
		return d.decodeTransferRedeemed(log, rawEvent, chainID)
	default:
		return nil, fmt.Errorf("unknown event topic: %s", eventID.Hex())
	}
}

// decodeLogMessagePublished handles the source-chain event.
//
// LogMessagePublished structure:
//   - Topics[0]: event signature
//   - Topics[1]: sender address (indexed)
//   - Data:      sequence (uint64), nonce (uint32), payload (bytes), consistencyLevel (uint8)
//
// The correlation key for Wormhole is (emitterChain, emitterAddress, sequence).
// emitterChain = the Wormhole chain ID of the source chain.
// emitterAddress = the sender address (zero-padded to bytes32).
// sequence = monotonically increasing per sender.
func (d *WormholeDecoder) decodeLogMessagePublished(log ethtypes.Log, rawEvent *decoder.RawEvent, chainID uint64) (*decoder.RawEvent, error) {
	rawEvent.EventType = "LogMessagePublished"

	// sender is indexed — Topics[1]
	if len(log.Topics) < 2 {
		return nil, fmt.Errorf("LogMessagePublished: missing sender topic")
	}
	sender := common.BytesToAddress(log.Topics[1].Bytes())
	rawEvent.Data["sender"] = sender.Hex()
	// emitter_address is the sender zero-padded to bytes32 (Wormhole convention)
	rawEvent.Data["emitter_address"] = log.Topics[1].Hex()

	// Unpack non-indexed fields from Data
	event := d.parsedABI.Events["LogMessagePublished"]
	unpacked, err := event.Inputs.Unpack(log.Data)
	if err != nil {
		return nil, fmt.Errorf("LogMessagePublished: failed to unpack: %w", err)
	}

	type logMsgData struct {
		Sequence         uint64
		Nonce            uint32
		Payload          []byte
		ConsistencyLevel uint8
	}
	var parsed logMsgData
	if err := event.Inputs.Copy(&parsed, unpacked); err != nil {
		return nil, fmt.Errorf("LogMessagePublished: failed to copy fields: %w", err)
	}

	rawEvent.Data["sequence"] = fmt.Sprintf("%d", parsed.Sequence)
	rawEvent.Data["nonce"] = fmt.Sprintf("%d", parsed.Nonce)
	rawEvent.Data["consistency_level"] = fmt.Sprintf("%d", parsed.ConsistencyLevel)

	if len(parsed.Payload) > 0 {
		rawEvent.Data["payload"] = fmt.Sprintf("0x%x", parsed.Payload)

		// Attempt to decode the token bridge VAA payload for enriched fields.
		// This is best-effort — if the payload is not a token transfer we skip silently.
		transfer, err := ParseTokenBridgePayload(parsed.Payload)
		if err != nil {
			// Malformed payload — log as warning but don't fail decoding
			rawEvent.Data["payload_parse_error"] = err.Error()
		} else if transfer != nil {
			// Enrich with decoded token transfer fields
			rawEvent.Data["token_address"] = transfer.TokenAddress
			rawEvent.Data["token_chain"] = fmt.Sprintf("%d", transfer.TokenChain)
			if transfer.TokenChainEVM > 0 {
				rawEvent.Data["token_chain_evm"] = fmt.Sprintf("%d", transfer.TokenChainEVM)
			}
			rawEvent.Data["to_address"] = transfer.ToAddress
			rawEvent.Data["to_chain"] = fmt.Sprintf("%d", transfer.ToChain)
			if transfer.ToChainEVM > 0 {
				rawEvent.Data["to_chain_evm"] = fmt.Sprintf("%d", transfer.ToChainEVM)
			}
			rawEvent.Data["token_amount"] = transfer.Amount
			if transfer.Fee != "" {
				rawEvent.Data["relayer_fee"] = transfer.Fee
			}
			if len(transfer.ExtraPayload) > 0 {
				rawEvent.Data["extra_payload"] = fmt.Sprintf("0x%x", transfer.ExtraPayload)
			}
			rawEvent.Data["payload_type"] = fmt.Sprintf("%d", transfer.PayloadType)
		}
	} else {
		rawEvent.Data["payload"] = "0x"
	}

	// Wormhole chain ID for this source chain
	if whChainID, ok := EVMToWormholeChainID[chainID]; ok {
		rawEvent.Data["emitter_chain"] = fmt.Sprintf("%d", whChainID)
	} else {
		rawEvent.Data["emitter_chain"] = "unknown"
	}

	// message_id: canonical Wormhole VAA ID = "emitterChain/emitterAddress/sequence"
	rawEvent.Data["message_id"] = fmt.Sprintf("%s/%s/%s",
		rawEvent.Data["emitter_chain"],
		rawEvent.Data["emitter_address"],
		rawEvent.Data["sequence"],
	)

	return rawEvent, nil
}

// decodeTransferRedeemed handles the destination-chain event.
//
// TransferRedeemed structure (all fields are indexed):
//   - Topics[0]: event signature
//   - Topics[1]: emitterChainId (uint16, indexed)
//   - Topics[2]: emitterAddress (bytes32, indexed)
//   - Topics[3]: sequence (uint64, indexed)
//
// All three indexed fields together form the correlation key that matches
// back to the original LogMessagePublished event.
func (d *WormholeDecoder) decodeTransferRedeemed(log ethtypes.Log, rawEvent *decoder.RawEvent, chainID uint64) (*decoder.RawEvent, error) {
	rawEvent.EventType = "TransferRedeemed"

	if len(log.Topics) < 4 {
		return nil, fmt.Errorf("TransferRedeemed: expected 4 topics, got %d", len(log.Topics))
	}

	// emitterChainId: Topics[1] — uint16 stored right-aligned in 32 bytes
	emitterChainID := uint16(log.Topics[1].Big().Uint64())
	rawEvent.Data["emitter_chain"] = fmt.Sprintf("%d", emitterChainID)

	// map Wormhole chain ID → EVM chain ID for the source chain
	if evmChainID, ok := WormholeChainIDToEVM[emitterChainID]; ok {
		rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", evmChainID)
	} else {
		rawEvent.Data["src_chain_id"] = "unknown"
	}

	// emitterAddress: Topics[2] — bytes32
	rawEvent.Data["emitter_address"] = log.Topics[2].Hex()
	// also expose as a human-readable EVM address (last 20 bytes)
	rawEvent.Data["sender"] = common.BytesToAddress(log.Topics[2].Bytes()).Hex()

	// sequence: Topics[3] — uint64 stored right-aligned in 32 bytes
	sequence := log.Topics[3].Big().Uint64()
	rawEvent.Data["sequence"] = fmt.Sprintf("%d", sequence)

	// reconstruct the canonical message_id for correlation lookup
	rawEvent.Data["message_id"] = fmt.Sprintf("%d/%s/%d",
		emitterChainID,
		log.Topics[2].Hex(),
		sequence,
	)

	// receiver: the transaction sender is the redeemer — not directly in the event,
	// but we record the contract address as the destination receiver placeholder.
	// The actual receiver is in the VAA payload (not decoded here).
	rawEvent.Data["receiver"] = log.Address.Hex()

	return rawEvent, nil
}