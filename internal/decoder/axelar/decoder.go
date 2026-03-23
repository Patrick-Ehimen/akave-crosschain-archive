package axelar

import (
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

const (
	// ProtocolName is the identifier for Axelar protocol in the decoder registry.
	ProtocolName = "axelar"
)

// ChainNameToID maps Axelar chain name strings to EVM chain IDs.
// Axelar uses human-readable chain names (e.g. "ethereum", "Avalanche") in its
// GMP events, which must be translated to standard EVM chain IDs.
var ChainNameToID = map[string]uint64{
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
	"linea":     59144,
	"Linea":     59144,
	"Moonbeam":  1284,
	"moonbeam":  1284,
	"Filecoin":  314,
	"filecoin":  314,
	"Mantle":    5000,
	"mantle":    5000,
	"scroll":    534352,
	"Scroll":    534352,
}

// ChainIDToName maps EVM chain IDs to Axelar chain name strings.
// Built automatically from ChainNameToID in init().
var ChainIDToName = map[uint64]string{}

func init() {
	for name, id := range ChainNameToID {
		// Use lowercase as canonical form.
		if _, exists := ChainIDToName[id]; !exists {
			ChainIDToName[id] = strings.ToLower(name)
		}
	}
}

// GatewayAddresses maps EVM chain IDs to their Axelar Gateway contract addresses.
// These are the gateway contracts that emit ContractCall, ContractCallApproved,
// and Executed events.
var GatewayAddresses = map[uint64]common.Address{
	1:     common.HexToAddress("0x4F4495243837681061C4743b74B3eEdf548D56A5"), // Ethereum
	43114: common.HexToAddress("0x5029C0EFf6C34351a0CEc334542cDb22c7928f78"), // Avalanche
	137:   common.HexToAddress("0x6f015F16De9fC8791b234eF68D486d2bF203FBA8"), // Polygon
	56:    common.HexToAddress("0x304acf330bbE08d1e512eefaa92F6a57871fD895"), // BSC
	42161: common.HexToAddress("0xe432150cce91c13a887f7D836923d5597adD8E31"), // Arbitrum
	10:    common.HexToAddress("0xe432150cce91c13a887f7D836923d5597adD8E31"), // Optimism
	8453:  common.HexToAddress("0xe432150cce91c13a887f7D836923d5597adD8E31"), // Base
	250:   common.HexToAddress("0x304acf330bbE08d1e512eefaa92F6a57871fD895"), // Fantom
}

// axelarABI defines the ABI for Axelar Gateway events relevant to GMP.
const axelarABI = `[
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "address", "name": "sender", "type": "address"},
			{"indexed": false, "internalType": "string", "name": "destinationChain", "type": "string"},
			{"indexed": false, "internalType": "string", "name": "destinationContractAddress", "type": "string"},
			{"indexed": true, "internalType": "bytes32", "name": "payloadHash", "type": "bytes32"},
			{"indexed": false, "internalType": "bytes", "name": "payload", "type": "bytes"}
		],
		"name": "ContractCall",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "commandId", "type": "bytes32"},
			{"indexed": false, "internalType": "string", "name": "sourceChain", "type": "string"},
			{"indexed": false, "internalType": "string", "name": "sourceAddress", "type": "string"},
			{"indexed": true, "internalType": "address", "name": "contractAddress", "type": "address"},
			{"indexed": false, "internalType": "bytes32", "name": "payloadHash", "type": "bytes32"},
			{"indexed": true, "internalType": "bytes32", "name": "sourceTxHash", "type": "bytes32"},
			{"indexed": false, "internalType": "uint256", "name": "sourceEventIndex", "type": "uint256"}
		],
		"name": "ContractCallApproved",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "commandId", "type": "bytes32"}
		],
		"name": "Executed",
		"type": "event"
	}
]`

// AxelarDecoder implements the Decoder interface for Axelar GMP protocol events.
type AxelarDecoder struct {
	parsedABI abi.ABI
}

// NewAxelarDecoder creates a new Axelar GMP protocol decoder.
func NewAxelarDecoder() *AxelarDecoder {
	parsed, err := abi.JSON(strings.NewReader(axelarABI))
	if err != nil {
		panic(fmt.Errorf("failed to parse Axelar ABI: %v", err))
	}
	return &AxelarDecoder{
		parsedABI: parsed,
	}
}

func (d *AxelarDecoder) Protocol() string {
	return ProtocolName
}

func (d *AxelarDecoder) ContractAddresses(chainID uint64) []common.Address {
	if addr, ok := GatewayAddresses[chainID]; ok {
		return []common.Address{addr}
	}
	return nil
}

func (d *AxelarDecoder) EventTopics() []common.Hash {
	return []common.Hash{
		d.parsedABI.Events["ContractCall"].ID,
		d.parsedABI.Events["ContractCallApproved"].ID,
		d.parsedABI.Events["Executed"].ID,
	}
}

func (d *AxelarDecoder) Decode(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
	if len(log.Topics) == 0 {
		return nil, fmt.Errorf("no topics in log")
	}

	eventID := log.Topics[0]
	var eventName string
	var event abi.Event

	for name, ev := range d.parsedABI.Events {
		if ev.ID == eventID {
			eventName = name
			event = ev
			break
		}
	}

	if eventName == "" {
		return nil, fmt.Errorf("unknown event topic: %s", eventID.Hex())
	}

	rawEvent := &decoder.RawEvent{
		Protocol:    ProtocolName,
		ChainID:     chainID,
		BlockNumber: log.BlockNumber,
		TxHash:      log.TxHash.Hex(),
		LogIndex:    log.Index,
		EventType:   eventName,
		Data:        make(map[string]string),
	}

	switch eventName {
	case "ContractCall":
		if err := d.decodeContractCall(log, event, rawEvent); err != nil {
			return nil, err
		}
	case "ContractCallApproved":
		if err := d.decodeContractCallApproved(log, event, rawEvent); err != nil {
			return nil, err
		}
	case "Executed":
		d.decodeExecuted(log, rawEvent)
	}

	return rawEvent, nil
}

// decodeContractCall extracts fields from a ContractCall event.
// ContractCall(address indexed sender, string destinationChain,
//
//	string destinationContractAddress, bytes32 indexed payloadHash, bytes payload)
func (d *AxelarDecoder) decodeContractCall(log ethtypes.Log, event abi.Event, rawEvent *decoder.RawEvent) error {
	// Indexed: sender (topics[1]), payloadHash (topics[2])
	if len(log.Topics) >= 3 {
		rawEvent.Data["sender"] = common.BytesToAddress(log.Topics[1].Bytes()).Hex()
		rawEvent.Data["payload_hash"] = log.Topics[2].Hex()
	}

	// Non-indexed: destinationChain, destinationContractAddress, payload
	unpacked, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return fmt.Errorf("failed to unpack ContractCall: %v", err)
	}
	if len(unpacked) < 3 {
		return fmt.Errorf("unexpected unpacked size for ContractCall: got %d, want at least 3", len(unpacked))
	}

	destChain, ok := unpacked[0].(string)
	if !ok {
		return fmt.Errorf("destinationChain is not string")
	}
	rawEvent.Data["destination_chain"] = destChain

	if chainID, ok := ChainNameToID[destChain]; ok {
		rawEvent.Data["dst_chain_id"] = fmt.Sprintf("%d", chainID)
	} else {
		rawEvent.Data["dst_chain_id"] = "unknown"
	}

	destAddr, ok := unpacked[1].(string)
	if !ok {
		return fmt.Errorf("destinationContractAddress is not string")
	}
	rawEvent.Data["destination_contract_address"] = destAddr

	payload, ok := unpacked[2].([]byte)
	if !ok {
		return fmt.Errorf("payload is not []byte")
	}
	rawEvent.Data["payload"] = common.Bytes2Hex(payload)

	return nil
}

// decodeContractCallApproved extracts fields from a ContractCallApproved event.
// ContractCallApproved(bytes32 indexed commandId, string sourceChain,
//
//	string sourceAddress, address indexed contractAddress,
//	bytes32 payloadHash, bytes32 indexed sourceTxHash, uint256 sourceEventIndex)
func (d *AxelarDecoder) decodeContractCallApproved(log ethtypes.Log, event abi.Event, rawEvent *decoder.RawEvent) error {
	// Indexed: commandId (topics[1]), contractAddress (topics[2]), sourceTxHash (topics[3])
	if len(log.Topics) >= 4 {
		rawEvent.Data["command_id"] = log.Topics[1].Hex()
		rawEvent.Data["contract_address"] = common.BytesToAddress(log.Topics[2].Bytes()).Hex()
		rawEvent.Data["source_tx_hash"] = log.Topics[3].Hex()
	}

	// Non-indexed: sourceChain, sourceAddress, payloadHash, sourceEventIndex
	unpacked, err := event.Inputs.NonIndexed().Unpack(log.Data)
	if err != nil {
		return fmt.Errorf("failed to unpack ContractCallApproved: %v", err)
	}
	if len(unpacked) < 4 {
		return fmt.Errorf("unexpected unpacked size for ContractCallApproved: got %d, want at least 4", len(unpacked))
	}

	srcChain, ok := unpacked[0].(string)
	if !ok {
		return fmt.Errorf("sourceChain is not string")
	}
	rawEvent.Data["source_chain"] = srcChain

	if chainID, ok := ChainNameToID[srcChain]; ok {
		rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", chainID)
	} else {
		rawEvent.Data["src_chain_id"] = "unknown"
	}

	srcAddr, ok := unpacked[1].(string)
	if !ok {
		return fmt.Errorf("sourceAddress is not string")
	}
	rawEvent.Data["source_address"] = srcAddr

	payloadHash, ok := unpacked[2].([32]byte)
	if !ok {
		return fmt.Errorf("payloadHash is not [32]byte")
	}
	rawEvent.Data["payload_hash"] = common.BytesToHash(payloadHash[:]).Hex()

	srcEventIndex, ok := unpacked[3].(*big.Int)
	if !ok {
		return fmt.Errorf("sourceEventIndex is not *big.Int")
	}
	rawEvent.Data["source_event_index"] = srcEventIndex.String()

	return nil
}

// decodeExecuted extracts the commandId from an Executed event.
// Executed(bytes32 indexed commandId)
func (d *AxelarDecoder) decodeExecuted(log ethtypes.Log, rawEvent *decoder.RawEvent) {
	if len(log.Topics) >= 2 {
		rawEvent.Data["command_id"] = log.Topics[1].Hex()
	}
}
