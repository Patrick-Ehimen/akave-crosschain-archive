package decoder

import (
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// RawEvent represents a decoded protocol-specific event before normalization.
type RawEvent struct {
	Protocol    string            `json:"protocol"`
	ChainID     uint64            `json:"chain_id"`
	BlockNumber uint64            `json:"block_number"`
	TxHash      string            `json:"tx_hash"`
	LogIndex    uint              `json:"log_index"`
	Timestamp   int64             `json:"timestamp"`
	EventType   string            `json:"event_type"`
	Data        map[string]string `json:"data"`
}

// Decoder is the interface that all protocol decoders must implement.
type Decoder interface {
	// Protocol returns the protocol name (e.g., "layerzero_v2", "wormhole").
	Protocol() string

	// ContractAddresses returns the contract addresses to monitor on a given chain.
	ContractAddresses(chainID uint64) []common.Address

	// EventTopics returns the event topic hashes this decoder handles.
	EventTopics() []common.Hash

	// Decode parses a raw Ethereum log into a protocol-specific RawEvent.
	Decode(log ethtypes.Log, chainID uint64) (*RawEvent, error)
}
