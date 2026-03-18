package archiver

import "time"

// ParquetRecord is the flattened representation of a cross-chain message
// for Parquet serialization. It joins data from messages, message_sources,
// message_destinations, message_payloads, and message_metadata tables.
type ParquetRecord struct {
	// Core message fields
	MessageID string    `parquet:"message_id"`
	Protocol  string    `parquet:"protocol"`
	Type      string    `parquet:"type"`
	Status    string    `parquet:"status"`
	CreatedAt time.Time `parquet:"created_at,timestamp"`
	UpdatedAt time.Time `parquet:"updated_at,timestamp"`

	// Source fields (always present)
	SrcChainID     uint64 `parquet:"src_chain_id"`
	SrcTxHash      string `parquet:"src_tx_hash"`
	SrcBlockNumber uint64 `parquet:"src_block_number"`
	SrcTimestamp   int64  `parquet:"src_timestamp"`
	SrcSender      string `parquet:"src_sender"`
	SrcLogIndex    int32  `parquet:"src_log_index"`

	// Destination fields (optional — nil when message is still pending)
	DstChainID     *uint64 `parquet:"dst_chain_id,optional"`
	DstTxHash      *string `parquet:"dst_tx_hash,optional"`
	DstBlockNumber *uint64 `parquet:"dst_block_number,optional"`
	DstTimestamp   *int64  `parquet:"dst_timestamp,optional"`
	DstReceiver    *string `parquet:"dst_receiver,optional"`
	DstLogIndex    *int32  `parquet:"dst_log_index,optional"`

	// Payload fields (optional)
	PayloadToken  *string `parquet:"payload_token,optional"`
	PayloadAmount *string `parquet:"payload_amount,optional"`
	PayloadData   *string `parquet:"payload_data,optional"`
	PayloadNonce  *uint64 `parquet:"payload_nonce,optional"`

	// Metadata fields (optional)
	MetaFee            *string `parquet:"meta_fee,optional"`
	MetaRelayer        *string `parquet:"meta_relayer,optional"`
	MetaGasUsed        *uint64 `parquet:"meta_gas_used,optional"`
	MetaLatencySeconds *int64  `parquet:"meta_latency_seconds,optional"`
}
