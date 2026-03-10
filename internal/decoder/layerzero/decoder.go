package layerzero

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

const (
	ProtocolName = "layerzero_v2"
)

// EndpointToChainID maps LayerZero V2 endpoint IDs to EVM chain IDs.
var EndpointToChainID = map[uint32]uint64{
	30101: 1,      // Ethereum
	30102: 56,     // BSC
	30106: 43114,  // Avalanche
	30109: 137,    // Polygon
	30110: 42161,  // Arbitrum
	30111: 10,     // Optimism
	30112: 250,    // Fantom
	30184: 8453,   // Base
	30183: 59144,  // Linea
	30181: 5000,   // Mantle
	30214: 534352, // Scroll
}

// EndpointAddresses maps EVM chain IDs to their specific LayerZero V2 Endpoint contract address.
var EndpointAddresses = map[uint64]common.Address{
	// Mainnets
	1:      common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Ethereum
	56:     common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // BSC
	43114:  common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Avalanche
	137:    common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Polygon
	42161:  common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Arbitrum
	10:     common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Optimism
	250:    common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Fantom
	8453:   common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Base
	59144:  common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Linea
	5000:   common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Mantle
	534352: common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c"), // Scroll

	// Testnets
	11155111: common.HexToAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"), // Sepolia
	421614:   common.HexToAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"), // Arbitrum Sepolia
	84532:    common.HexToAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"), // Base Sepolia
	43113:    common.HexToAddress("0x6EDCE65403992e310A62460808c4b910D972f10f"), // Fuji (Avalanche testnet)
}

// ChainIDToEndpoint maps EVM chain IDs to LayerZero V2 endpoint IDs.
var ChainIDToEndpoint = map[uint64]uint32{}

func init() {
	for eid, cid := range EndpointToChainID {
		ChainIDToEndpoint[cid] = eid
	}
}

const lzABI = `[
	{
		"anonymous": false,
		"inputs": [
			{"indexed": false, "internalType": "bytes", "name": "encodedPayload", "type": "bytes"},
			{"indexed": false, "internalType": "bytes", "name": "options", "type": "bytes"},
			{"indexed": false, "internalType": "address", "name": "sendLibrary", "type": "address"}
		],
		"name": "PacketSent",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"components": [
					{"internalType": "uint32", "name": "srcEid", "type": "uint32"},
					{"internalType": "bytes32", "name": "sender", "type": "bytes32"},
					{"internalType": "uint64", "name": "nonce", "type": "uint64"}
				],
				"indexed": false,
				"internalType": "struct Origin",
				"name": "origin",
				"type": "tuple"
			},
			{"indexed": false, "internalType": "address", "name": "receiver", "type": "address"}
		],
		"name": "PacketDelivered",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"components": [
					{"internalType": "uint32", "name": "srcEid", "type": "uint32"},
					{"internalType": "bytes32", "name": "sender", "type": "bytes32"},
					{"internalType": "uint64", "name": "nonce", "type": "uint64"}
				],
				"indexed": false,
				"internalType": "struct Origin",
				"name": "origin",
				"type": "tuple"
			},
			{"indexed": false, "internalType": "address", "name": "receiver", "type": "address"}
		],
		"name": "PacketReceived",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "guid", "type": "bytes32"},
			{"indexed": false, "internalType": "uint32", "name": "dstEid", "type": "uint32"},
			{"indexed": true, "internalType": "address", "name": "fromAddress", "type": "address"},
			{"indexed": false, "internalType": "uint256", "name": "amountSentLD", "type": "uint256"},
			{"indexed": false, "internalType": "uint256", "name": "amountReceivedLD", "type": "uint256"}
		],
		"name": "OFTSent",
		"type": "event"
	}
]`

type LayerZeroDecoder struct {
	parsedABI abi.ABI
}

func NewLayerZeroDecoder() *LayerZeroDecoder {
	parsed, err := abi.JSON(strings.NewReader(lzABI))
	if err != nil {
		panic(fmt.Errorf("failed to parse LayerZero ABI: %v", err))
	}
	return &LayerZeroDecoder{
		parsedABI: parsed,
	}
}

func (d *LayerZeroDecoder) Protocol() string {
	return ProtocolName
}

func (d *LayerZeroDecoder) ContractAddresses(chainID uint64) []common.Address {
	if addr, ok := EndpointAddresses[chainID]; ok {
		return []common.Address{addr}
	}
	// Fallback to the standard EVM V2 endpoint if not explicitly mapped
	return []common.Address{common.HexToAddress("0x1a44076050125825900e736c501f859c50fE728c")}
}

func (d *LayerZeroDecoder) EventTopics() []common.Hash {
	return []common.Hash{
		d.parsedABI.Events["PacketSent"].ID,
		d.parsedABI.Events["PacketDelivered"].ID,
		d.parsedABI.Events["PacketReceived"].ID,
		d.parsedABI.Events["OFTSent"].ID,
	}
}

func (d *LayerZeroDecoder) Decode(log ethtypes.Log, chainID uint64) (*decoder.RawEvent, error) {
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

	// We treat PacketDelivered as PacketReceived to match the exact issue wording if necessary.
	if rawEvent.EventType == "PacketDelivered" {
		rawEvent.EventType = "PacketReceived"
	}

	switch eventName {
	case "PacketSent":
		unpacked, err := event.Inputs.Unpack(log.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to unpack PacketSent: %v", err)
		}
		if len(unpacked) < 1 {
			return nil, fmt.Errorf("unexpected unpacked size for PacketSent")
		}

		encodedPayload, ok := unpacked[0].([]byte)
		if !ok {
			return nil, fmt.Errorf("encodedPayload is not []byte")
		}

		if err := decodeEncodedPayload(encodedPayload, rawEvent.Data); err != nil {
			return nil, fmt.Errorf("failed to decode encodedPayload: %v", err)
		}

	case "PacketDelivered", "PacketReceived":
		unpacked, err := event.Inputs.Unpack(log.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to unpack PacketReceived: %v", err)
		}

		// Unpack origin struct and receiver
		if len(unpacked) >= 2 {
			originStruct := unpacked[0].(struct {
				SrcEid uint32    `json:"srcEid"`
				Sender [32]uint8 `json:"sender"`
				Nonce  uint64    `json:"nonce"`
			})
			receiver := unpacked[1].(common.Address)

			rawEvent.Data["src_eid"] = fmt.Sprintf("%d", originStruct.SrcEid)
			rawEvent.Data["src_chain_id"] = fmt.Sprintf("%d", EndpointToChainID[originStruct.SrcEid])
			rawEvent.Data["sender"] = common.BytesToAddress(originStruct.Sender[:]).Hex()
			rawEvent.Data["nonce"] = fmt.Sprintf("%d", originStruct.Nonce)
			rawEvent.Data["receiver"] = receiver.Hex()
		}

	case "OFTSent":
		// OFTSent has indexed arguments: guid, fromAddress
		// Unpack non-indexed: dstEid, amountSentLD, amountReceivedLD
		if len(log.Topics) >= 3 {
			rawEvent.Data["guid"] = log.Topics[1].Hex()
			rawEvent.Data["from_address"] = common.BytesToAddress(log.Topics[2].Bytes()).Hex()
		}

		unpacked, err := event.Inputs.NonIndexed().Unpack(log.Data)
		if err != nil {
			return nil, fmt.Errorf("failed to unpack OFTSent: %v", err)
		}
		if len(unpacked) >= 3 {
			dstEid := unpacked[0].(uint32)
			amountSentLD := unpacked[1].(*big.Int)
			amountReceivedLD := unpacked[2].(*big.Int)

			rawEvent.Data["dst_eid"] = fmt.Sprintf("%d", dstEid)
			rawEvent.Data["dst_chain_id"] = fmt.Sprintf("%d", EndpointToChainID[dstEid])
			if amountSentLD != nil {
				rawEvent.Data["amount_sent"] = amountSentLD.String()
			}
			if amountReceivedLD != nil {
				rawEvent.Data["amount_received"] = amountReceivedLD.String()
			}
		}
	}

	return rawEvent, nil
}

func decodeEncodedPayload(payload []byte, data map[string]string) error {
	if len(payload) < 113 {
		return fmt.Errorf("payload too short, expected at least 113 bytes, got %d", len(payload))
	}

	// version (1 byte)
	data["version"] = fmt.Sprintf("%d", payload[0])

	// nonce (8 bytes)
	nonce := binary.BigEndian.Uint64(payload[1:9])
	data["nonce"] = fmt.Sprintf("%d", nonce)

	// srcEid (4 bytes)
	srcEid := binary.BigEndian.Uint32(payload[9:13])
	data["src_eid"] = fmt.Sprintf("%d", srcEid)
	if chainID, ok := EndpointToChainID[srcEid]; ok {
		data["src_chain_id"] = fmt.Sprintf("%d", chainID)
	}

	// sender (32 bytes) -> last 20 bytes usually address
	senderBytes := payload[13:45]
	data["sender"] = common.BytesToAddress(senderBytes[12:]).Hex()

	// dstEid (4 bytes)
	dstEid := binary.BigEndian.Uint32(payload[45:49])
	data["dst_eid"] = fmt.Sprintf("%d", dstEid)
	if chainID, ok := EndpointToChainID[dstEid]; ok {
		data["dst_chain_id"] = fmt.Sprintf("%d", chainID)
	}

	// receiver (32 bytes) -> last 20 bytes usually address
	receiverBytes := payload[49:81]
	data["receiver"] = common.BytesToAddress(receiverBytes[12:]).Hex()

	// guid (32 bytes)
	guid := payload[81:113]
	data["guid"] = "0x" + hex.EncodeToString(guid)

	// message (remaining bytes)
	if len(payload) > 113 {
		data["message"] = "0x" + hex.EncodeToString(payload[113:])
	} else {
		data["message"] = "0x"
	}

	return nil
}
