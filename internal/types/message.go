package types

import "time"

// MessageStatus represents the lifecycle state of a cross-chain message.
type MessageStatus string

const (
	StatusPending  MessageStatus = "pending"
	StatusExecuted MessageStatus = "executed"
	StatusFailed   MessageStatus = "failed"
)

// MessageType categorizes the cross-chain message.
type MessageType string

const (
	TypeTokenTransfer MessageType = "token_transfer"
	TypeMessage       MessageType = "message"
	TypeContractCall  MessageType = "contract_call"
)

// Message is the unified cross-chain message representation.
type Message struct {
	MessageID   string        `json:"message_id"`
	Protocol    string        `json:"protocol"`
	Type        MessageType   `json:"type"`
	Status      MessageStatus `json:"status"`
	Source      Source        `json:"source"`
	Destination *Destination  `json:"destination,omitempty"`
	Payload     *Payload      `json:"payload,omitempty"`
	Metadata    *Metadata     `json:"metadata,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// Source holds details about the originating chain transaction.
type Source struct {
	ChainID     uint64 `json:"chain_id"`
	TxHash      string `json:"tx_hash"`
	BlockNumber uint64 `json:"block_number"`
	Timestamp   int64  `json:"timestamp"`
	Sender      string `json:"sender"`
	LogIndex    uint   `json:"log_index"`
}

// Destination holds details about the receiving chain transaction.
type Destination struct {
	ChainID     uint64 `json:"chain_id"`
	TxHash      string `json:"tx_hash"`
	BlockNumber uint64 `json:"block_number"`
	Timestamp   int64  `json:"timestamp"`
	Receiver    string `json:"receiver"`
	LogIndex    uint   `json:"log_index"`
}

// Payload holds the transfer or message data.
type Payload struct {
	Token  string `json:"token,omitempty"`
	Amount string `json:"amount,omitempty"`
	Data   string `json:"data,omitempty"`
	Nonce  uint64 `json:"nonce,omitempty"`
}

// Metadata holds execution details.
type Metadata struct {
	Fee            string `json:"fee,omitempty"`
	Relayer        string `json:"relayer,omitempty"`
	GasUsed        uint64 `json:"gas_used,omitempty"`
	LatencySeconds int64  `json:"latency_seconds,omitempty"`
}
