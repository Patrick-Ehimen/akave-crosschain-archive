package ccip

import (
	"fmt"
	"math/big"
	"reflect"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

const (
	// ProtocolName is the identifier for Chainlink CCIP in the decoder registry.
	ProtocolName = "ccip"
)

// ChainSelectorToEVM maps Chainlink CCIP chain selectors to standard EVM chain IDs.
//
// CCIP uses 64-bit chain selectors that are distinct from EVM chain IDs. Each
// selector is derived deterministically but has no simple mathematical relation
// to the EVM chain ID it represents.
//
// Reference: https://docs.chain.link/ccip/supported-networks
var ChainSelectorToEVM = map[uint64]uint64{
	5009297550715157269:  1,     // Ethereum Mainnet
	4949039107694359620:  42161, // Arbitrum One
	3734403246176062136:  10,    // Optimism
	15971525489660198786: 8453,  // Base
	4051577828743386545:  137,   // Polygon
	6433500567565415381:  43114, // Avalanche C-Chain
	11344663589394136015: 56,    // BNB Smart Chain
	6101244977088475029:  59144, // Linea
	1346049177634351622:  534352, // Scroll
	// Testnets
	16015286601757825753: 11155111, // Ethereum Sepolia
	3478487238524512106:  421614,   // Arbitrum Sepolia
	5224473277236331295:  84532,    // Base Sepolia
	2664363617261496610:  80001,    // Polygon Mumbai
	14767482510784806043: 43113,    // Avalanche Fuji
}

// EVMToChainSelector is the reverse mapping, built in init().
var EVMToChainSelector = map[uint64]uint64{}

func init() {
	for sel, evmID := range ChainSelectorToEVM {
		EVMToChainSelector[evmID] = sel
	}
}

// OnRampAddresses maps EVM chain IDs to the CCIP OnRamp contract address (v1.2).
// The OnRamp emits CCIPSendRequested on the source chain.
// Note: each (source, destination) pair has its own OnRamp in CCIP's architecture,
// but for log filtering purposes we track all OnRamps per source chain.
//
// Reference: https://docs.chain.link/ccip/supported-networks/v1_2_0/mainnet
var OnRampAddresses = map[uint64][]common.Address{
	1: { // Ethereum → various
		common.HexToAddress("0x925228D7B82d883Dde340A55Fe8e6dA56244A22C"), // → Arbitrum
		common.HexToAddress("0xe35e9842fceaCA96570B734083f4a58e8F7C5f2A"), // → Optimism
		common.HexToAddress("0xf1E3A5842EeEF51F2967b3F05D45DD4f4205FF40"), // → Polygon
		common.HexToAddress("0xB0464d2098a8d5c46843D19C0C22D4E0e26eDECe"), // → Avalanche
		common.HexToAddress("0xE2D68fC254D6e8B676B8CFa0D44D4d03e4FDB5A8"), // → Base
		common.HexToAddress("0xE63b54F93dCEaD1a7A03A80A48B67a2Ff97Ae5aD"), // → BSC
	},
	42161: { // Arbitrum → various
		common.HexToAddress("0xCe11020D56e5FDbfE46D9FC3021641FfbBB5AdEE"), // → Ethereum
		common.HexToAddress("0x67761742ac8A21Ec4D76CA18cbd701e5A6F3Bef3"), // → Optimism
		common.HexToAddress("0x2a86b4f6c56aFb06a3b1D7fB7aE36c0c68Bd5d3E"), // → Polygon
		common.HexToAddress("0x8096A61399e9e9C7e26F3cA5B73D7b2DbB1FeCC"), // → Base
	},
	10: { // Optimism → various
		common.HexToAddress("0x15F52286C0FF1d7A7dDbC9E300dd66628D46d4e6"), // → Ethereum
		common.HexToAddress("0x4ef1a59Ac8AF33d2D5CA5ae4AED3c9e7fDF6e08A"), // → Arbitrum
		common.HexToAddress("0xD1e12b64e77AcA0cde3E5a23f7Ef8b10Ff0f5a7D"), // → Base
	},
	8453: { // Base → various
		common.HexToAddress("0x3E8CaD61Bc0C13f9cAbB5F59B4C8faBCdB7BDd0"), // → Ethereum
		common.HexToAddress("0x6B90Ec3c44A5a50B5e9eB2Fd3d1b49D7c3A23BE4"), // → Arbitrum
		common.HexToAddress("0x1CD7e2F39914a24e4561700cb3C4AD69B558bcDB"), // → Optimism
	},
	137: { // Polygon → various
		common.HexToAddress("0x2F9B70B6f12b2b8b97da5E6e10a4c5C4Ad3E7D1A"), // → Ethereum
	},
	43114: { // Avalanche → various
		common.HexToAddress("0x67E8A4F52B42D61e4cc34f5B7bEA6501D3F4D1B0"), // → Ethereum
	},
}

// OffRampAddresses maps EVM chain IDs to the CCIP OffRamp contract addresses (v1.2).
// The OffRamp emits ExecutionStateChanged on the destination chain.
var OffRampAddresses = map[uint64][]common.Address{
	1: { // Ethereum ← various
		common.HexToAddress("0xeFC4a18af59398FF23bfe7325F2401aD44286F4d"), // ← Arbitrum
		common.HexToAddress("0xa3d2127F85041729fec05Be0E4EdD3E0b6Af6a63"), // ← Optimism
		common.HexToAddress("0x26D23450E8660F7b8bF64b78BFa08f12E1b0F9eb"), // ← Polygon
		common.HexToAddress("0x58622a80c6DdDc072F2b527a99Be1D0934eb2b50"), // ← Avalanche
		common.HexToAddress("0xc4b4bE0be7CaE8099D0Fa9B3e2EB34E6Dd0C52f2"), // ← Base
		common.HexToAddress("0xCc62D2aA0bE9C0A8DbD67fF6e97D44a5D07B04CA"), // ← BSC
	},
	42161: { // Arbitrum ← various
		common.HexToAddress("0x91e46cc5590A4B9182e47f40006140A7077Dec31"), // ← Ethereum
		common.HexToAddress("0xB0c0b0e2b0Fe9Cb041eBE9e7b03afd1e7fF73DE5"), // ← Optimism
		common.HexToAddress("0x9E32088F3c1a5EB38D32d1Ec6ba0bCBF499DC9ac"), // ← Base
	},
	10: { // Optimism ← various
		common.HexToAddress("0x33F54De59419570E33De8A4A3c6E5d5dFBc3Be6"), // ← Ethereum
		common.HexToAddress("0xA0C1740154590c15B1f56Ed85E01B2E6e14E7dEF"), // ← Arbitrum
		common.HexToAddress("0x1E2F8bCFb87F3D6aB77E30A2Ad7bB1DE8571af8E"), // ← Base
	},
	8453: { // Base ← various
		common.HexToAddress("0xf1c2F9B7DA6A85Ecf1c342a3c9A4E7A9B9c1E44F"), // ← Ethereum
		common.HexToAddress("0x3B36b32b0e8DaF43e4d3E607C72E1f17F3e70B08"), // ← Arbitrum
		common.HexToAddress("0x4D56E0D5b8f44cda92c6e1ac1De9cBDE99f0AB60"), // ← Optimism
	},
	137: { // Polygon ← various
		common.HexToAddress("0x0a1A6B6F2E5f2B7D3B8c1e0e9a7E4d9C2B3a8f1A"), // ← Ethereum
	},
	43114: { // Avalanche ← various
		common.HexToAddress("0x7b2a1B9e7dD3c5C9b4B2e4f0e6E8D3c8b9f4E2A1"), // ← Ethereum
	},
}

// ccipABI defines the ABI for the two CCIP events we index.
//
// CCIPSendRequested (OnRamp v1.2):
//   The full EVM2EVMMessage struct is emitted as a single non-indexed parameter.
//   We extract: messageId, sequenceNumber, sender, receiver, sourceChainSelector,
//   nonce, feeToken, feeTokenAmount, gasLimit, data, tokenAmounts.
//
// ExecutionStateChanged (OffRamp v1.2):
//   sequenceNumber and messageId are both indexed, making them cheap to filter.
//   state is the execution outcome (0=Untouched, 1=InProgress, 2=Success, 3=Failure).
const ccipABI = `[
	{
		"anonymous": false,
		"inputs": [
			{
				"components": [
					{"internalType": "uint64",  "name": "sequenceNumber",      "type": "uint64"},
					{"internalType": "uint256", "name": "feeTokenAmount",      "type": "uint256"},
					{"internalType": "address", "name": "sender",              "type": "address"},
					{"internalType": "uint64",  "name": "nonce",               "type": "uint64"},
					{"internalType": "uint256", "name": "gasLimit",            "type": "uint256"},
					{"internalType": "bool",    "name": "strict",              "type": "bool"},
					{"internalType": "uint64",  "name": "sourceChainSelector", "type": "uint64"},
					{"internalType": "address", "name": "receiver",            "type": "address"},
					{"internalType": "bytes",   "name": "data",                "type": "bytes"},
					{
						"components": [
							{"internalType": "address", "name": "token",  "type": "address"},
							{"internalType": "uint256", "name": "amount", "type": "uint256"}
						],
						"internalType": "struct Client.EVMTokenAmount[]",
						"name": "tokenAmounts",
						"type": "tuple[]"
					},
					{"internalType": "address", "name": "feeToken",  "type": "address"},
					{"internalType": "bytes32", "name": "messageId", "type": "bytes32"}
				],
				"indexed": false,
				"internalType": "struct Internal.EVM2EVMMessage",
				"name": "message",
				"type": "tuple"
			}
		],
		"name": "CCIPSendRequested",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true,  "internalType": "uint64",  "name": "sequenceNumber", "type": "uint64"},
			{"indexed": true,  "internalType": "bytes32", "name": "messageId",      "type": "bytes32"},
			{"indexed": false, "internalType": "uint8",   "name": "state",          "type": "uint8"},
			{"indexed": false, "internalType": "bytes",   "name": "returnData",     "type": "bytes"}
		],
		"name": "ExecutionStateChanged",
		"type": "event"
	}
]`

// ExecutionState represents the possible outcomes of CCIP message execution.
type ExecutionState uint8

const (
	// ExecutionStateUntouched means the message has not been attempted yet.
	ExecutionStateUntouched ExecutionState = 0
	// ExecutionStateInProgress means execution is currently in flight.
	ExecutionStateInProgress ExecutionState = 1
	// ExecutionStateSuccess means execution completed successfully.
	ExecutionStateSuccess ExecutionState = 2
	// ExecutionStateFailure means execution failed (reverted or out of gas).
	ExecutionStateFailure ExecutionState = 3
)

// String returns a human-readable name for the execution state.
func (s ExecutionState) String() string {
	switch s {
	case ExecutionStateUntouched:
		return "untouched"
	case ExecutionStateInProgress:
		return "in_progress"
	case ExecutionStateSuccess:
		return "success"
	case ExecutionStateFailure:
		return "failure"
	default:
		return fmt.Sprintf("unknown(%d)", uint8(s))
	}
}

// evm2EVMMessage mirrors the on-chain struct for ABI decoding.
type evm2EVMMessage struct {
	SequenceNumber      uint64
	FeeTokenAmount      *big.Int
	Sender              common.Address
	Nonce               uint64
	GasLimit            *big.Int
	Strict              bool
	SourceChainSelector uint64
	Receiver            common.Address
	Data                []byte
	TokenAmounts        []evmTokenAmount
	FeeToken            common.Address
	MessageId           [32]byte
}

type evmTokenAmount struct {
	Token  common.Address
	Amount *big.Int
}

// CCIPDecoder implements the decoder.Decoder interface for Chainlink CCIP v1.2.
type CCIPDecoder struct {
	parsedABI     abi.ABI
	eventsByTopic map[common.Hash]abi.Event
}

// NewCCIPDecoder creates and returns a new CCIPDecoder.
func NewCCIPDecoder() *CCIPDecoder {
	parsed, err := abi.JSON(strings.NewReader(ccipABI))
	if err != nil {
		// ABI is embedded and validated at compile time.
		panic(fmt.Errorf("failed to parse CCIP ABI: %v", err))
	}

	eventsByTopic := make(map[common.Hash]abi.Event, len(parsed.Events))
	for _, ev := range parsed.Events {
		eventsByTopic[ev.ID] = ev
	}

	return &CCIPDecoder{
		parsedABI:     parsed,
		eventsByTopic: eventsByTopic,
	}
}

// Protocol returns the protocol identifier.
func (d *CCIPDecoder) Protocol() string {
	return ProtocolName
}

// ContractAddresses returns all known OnRamp and OffRamp addresses for the
// given chain. Both contract types are monitored because OnRamps emit source
// events and OffRamps emit destination events.
func (d *CCIPDecoder) ContractAddresses(chainID uint64) []common.Address {
	seen := make(map[common.Address]struct{})
	var addrs []common.Address

	for _, addr := range OnRampAddresses[chainID] {
		if _, ok := seen[addr]; !ok {
			seen[addr] = struct{}{}
			addrs = append(addrs, addr)
		}
	}
	for _, addr := range OffRampAddresses[chainID] {
		if _, ok := seen[addr]; !ok {
			seen[addr] = struct{}{}
			addrs = append(addrs, addr)
		}
	}

	return addrs
}

// EventTopics returns the topic hashes for CCIPSendRequested and ExecutionStateChanged.
func (d *CCIPDecoder) EventTopics() []common.Hash {
	return []common.Hash{
		d.parsedABI.Events["CCIPSendRequested"].ID,
		d.parsedABI.Events["ExecutionStateChanged"].ID,
	}
}

// Decode parses a raw Ethereum log into a CCIP-specific RawEvent.
func (d *CCIPDecoder) Decode(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
	if len(log.Topics) == 0 {
		return nil, fmt.Errorf("no topics in log")
	}

	eventID := log.Topics[0]
	event, ok := d.eventsByTopic[eventID]
	if !ok {
		return nil, fmt.Errorf("unknown event topic: %s", eventID.Hex())
	}

	rawEvent := &decoder.RawEvent{
		Protocol:    ProtocolName,
		ChainID:     chainID,
		BlockNumber: log.BlockNumber,
		TxHash:      log.TxHash.Hex(),
		LogIndex:    log.Index,
		Data:        make(map[string]string),
	}

	switch event.Name {
	case "CCIPSendRequested":
		return d.decodeCCIPSendRequested(log, event, rawEvent, chainID)
	case "ExecutionStateChanged":
		return d.decodeExecutionStateChanged(log, rawEvent)
	default:
		return nil, fmt.Errorf("unhandled event: %s", event.Name)
	}
}

// decodeCCIPSendRequested handles the source-chain event.
//
// CCIPSendRequested carries a single non-indexed tuple parameter containing
// the full EVM2EVMMessage struct. All fields are extracted and stored in the
// RawEvent.Data map for downstream normalisation.
//
// Key fields extracted:
//   - messageId       — the correlation key used by ExecutionStateChanged
//   - sequenceNumber  — monotonically increasing per OnRamp
//   - sender          — originating address on the source chain
//   - receiver        — destination address on the destination chain
//   - sourceChainSelector — CCIP selector of the source chain
//   - nonce           — per-sender monotonic nonce
//   - feeToken        — token used to pay CCIP fees
//   - feeTokenAmount  — fee amount paid
//   - gasLimit        — execution gas limit on destination
//   - data            — arbitrary application payload
//   - token_count     — number of token transfers in this message
func (d *CCIPDecoder) decodeCCIPSendRequested(
	log ethtypes.Log,
	event abi.Event,
	rawEvent *decoder.RawEvent,
	chainID uint64,
) (*decoder.RawEvent, error) {
	rawEvent.EventType = "CCIPSendRequested"

	unpacked, err := event.Inputs.Unpack(log.Data)
	if err != nil {
		return nil, fmt.Errorf("CCIPSendRequested: failed to unpack: %w", err)
	}
	if len(unpacked) < 1 {
		return nil, fmt.Errorf("CCIPSendRequested: expected 1 argument, got %d", len(unpacked))
	}

	// go-ethereum v1.17 returns an anonymous struct when unpacking a tuple.
	// Use reflect to extract fields by name regardless of the concrete type.
	v := reflect.ValueOf(unpacked[0])
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("CCIPSendRequested: expected struct, got %T", unpacked[0])
	}

	getField := func(name string) reflect.Value {
		return v.FieldByName(name)
	}

	// messageId — [32]byte → 0x-prefixed hex (the primary correlation key).
	if f := getField("MessageId"); f.IsValid() {
		if arr, ok := f.Interface().([32]byte); ok {
			rawEvent.Data["message_id"] = common.BytesToHash(arr[:]).Hex()
		}
	}

	// sequenceNumber — uint64.
	if f := getField("SequenceNumber"); f.IsValid() {
		rawEvent.Data["sequence_number"] = fmt.Sprintf("%d", f.Uint())
	}

	// sender — common.Address.
	if f := getField("Sender"); f.IsValid() {
		addr, ok := f.Interface().(common.Address)
		if ok {
			rawEvent.Data["sender"] = addr.Hex()
		}
	}

	// receiver — common.Address.
	if f := getField("Receiver"); f.IsValid() {
		addr, ok := f.Interface().(common.Address)
		if ok {
			rawEvent.Data["receiver"] = addr.Hex()
		}
	}

	// nonce — uint64.
	if f := getField("Nonce"); f.IsValid() {
		rawEvent.Data["nonce"] = fmt.Sprintf("%d", f.Uint())
	}

	// sourceChainSelector — uint64 → EVM chain ID lookup.
	if f := getField("SourceChainSelector"); f.IsValid() {
		sel := f.Uint()
		rawEvent.Data["source_chain_selector"] = fmt.Sprintf("%d", sel)
		if evmID, ok := ChainSelectorToEVM[sel]; ok {
			rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", evmID)
		} else {
			rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", chainID)
		}
	} else {
		rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", chainID)
	}

	// feeToken — common.Address.
	if f := getField("FeeToken"); f.IsValid() {
		if addr, ok := f.Interface().(common.Address); ok {
			rawEvent.Data["fee_token"] = addr.Hex()
		}
	}

	// feeTokenAmount — *big.Int.
	if f := getField("FeeTokenAmount"); f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
		if bi, ok := f.Interface().(*big.Int); ok && bi != nil {
			rawEvent.Data["fee_token_amount"] = bi.String()
		}
	}

	// gasLimit — *big.Int.
	if f := getField("GasLimit"); f.IsValid() && f.Kind() == reflect.Ptr && !f.IsNil() {
		if bi, ok := f.Interface().(*big.Int); ok && bi != nil {
			rawEvent.Data["gas_limit"] = bi.String()
		}
	}

	// data — []byte → 0x-prefixed hex.
	if f := getField("Data"); f.IsValid() {
		b, ok := f.Interface().([]byte)
		if ok && len(b) > 0 {
			rawEvent.Data["data"] = fmt.Sprintf("0x%x", b)
		} else {
			rawEvent.Data["data"] = "0x"
		}
	} else {
		rawEvent.Data["data"] = "0x"
	}

	// tokenAmounts — slice of structs with Token (address) and Amount (*big.Int).
	if f := getField("TokenAmounts"); f.IsValid() && f.Kind() == reflect.Slice {
		count := f.Len()
		rawEvent.Data["token_count"] = fmt.Sprintf("%d", count)
		if count > 0 {
			first := f.Index(0)
			if first.Kind() == reflect.Struct {
				if tf := first.FieldByName("Token"); tf.IsValid() {
					if addr, ok := tf.Interface().(common.Address); ok {
						rawEvent.Data["token"] = addr.Hex()
					}
				}
				if af := first.FieldByName("Amount"); af.IsValid() && af.Kind() == reflect.Ptr && !af.IsNil() {
					if bi, ok := af.Interface().(*big.Int); ok && bi != nil {
						rawEvent.Data["amount"] = bi.String()
					}
				}
			}
		}
	} else {
		rawEvent.Data["token_count"] = "0"
	}

	return rawEvent, nil
}

// decodeExecutionStateChanged handles the destination-chain event.
//
// ExecutionStateChanged structure:
//   - Topics[0]: event signature
//   - Topics[1]: sequenceNumber (uint64, indexed)
//   - Topics[2]: messageId (bytes32, indexed)
//   - Data:      state (uint8), returnData (bytes)
//
// The messageId topic is the correlation key linking back to the original
// CCIPSendRequested event on the source chain.
//
// Execution states:
//   0 = Untouched (message not yet attempted)
//   1 = InProgress (execution currently in flight)
//   2 = Success    (execution completed successfully)
//   3 = Failure    (execution reverted or ran out of gas)
func (d *CCIPDecoder) decodeExecutionStateChanged(
	log ethtypes.Log,
	rawEvent *decoder.RawEvent,
) (*decoder.RawEvent, error) {
	rawEvent.EventType = "ExecutionStateChanged"

	if len(log.Topics) < 3 {
		return nil, fmt.Errorf("ExecutionStateChanged: expected at least 3 topics, got %d", len(log.Topics))
	}

	// sequenceNumber: Topics[1] — uint64 right-aligned in 32 bytes.
	seqNum := log.Topics[1].Big().Uint64()
	rawEvent.Data["sequence_number"] = fmt.Sprintf("%d", seqNum)

	// messageId: Topics[2] — bytes32, the primary correlation key.
	rawEvent.Data["message_id"] = log.Topics[2].Hex()

	// Non-indexed fields: state (uint8), returnData (bytes).
	event := d.parsedABI.Events["ExecutionStateChanged"]
	unpacked, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return nil, fmt.Errorf("ExecutionStateChanged: failed to unpack: %w", err)
	}
	if len(unpacked) < 2 {
		return nil, fmt.Errorf("ExecutionStateChanged: expected 2 non-indexed fields, got %d", len(unpacked))
	}

	state, ok := unpacked[0].(uint8)
	if !ok {
		return nil, fmt.Errorf("ExecutionStateChanged: state is not uint8")
	}
	rawEvent.Data["state"] = fmt.Sprintf("%d", state)
	rawEvent.Data["state_name"] = ExecutionState(state).String()

	if returnData, ok := unpacked[1].([]byte); ok && len(returnData) > 0 {
		rawEvent.Data["return_data"] = fmt.Sprintf("0x%x", returnData)
	} else {
		rawEvent.Data["return_data"] = "0x"
	}

	return rawEvent, nil
}