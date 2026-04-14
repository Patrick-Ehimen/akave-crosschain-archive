package archiver

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ArchiveStore handles database operations for the archival pipeline.
type ArchiveStore struct {
	pool *pgxpool.Pool
}

// NewArchiveStore creates a new ArchiveStore.
func NewArchiveStore(pool *pgxpool.Pool) *ArchiveStore {
	return &ArchiveStore{pool: pool}
}

// FetchRecordsForArchival queries messages within a timestamp range for a
// specific protocol and chain, joining all related tables. Returns flattened
// ParquetRecords ready for serialization.
func (s *ArchiveStore) FetchRecordsForArchival(
	ctx context.Context,
	protocol string,
	chainID uint64,
	fromTimestamp, toTimestamp int64,
) ([]ParquetRecord, error) {
	query := `
		SELECT
			m.message_id, m.protocol, m.type, m.status, m.created_at, m.updated_at,
			s.chain_id, s.tx_hash, s.block_number, s.timestamp, s.sender, s.log_index,
			d.chain_id, d.tx_hash, d.block_number, d.timestamp, d.receiver, d.log_index,
			p.token, p.amount, p.data, p.nonce,
			md.fee, md.relayer, md.gas_used, md.latency_seconds
		FROM messages m
		JOIN message_sources s ON m.message_id = s.message_id
		LEFT JOIN message_destinations d ON m.message_id = d.message_id
		LEFT JOIN message_payloads p ON m.message_id = p.message_id
		LEFT JOIN message_metadata md ON m.message_id = md.message_id
		WHERE m.protocol = $1
		  AND s.chain_id = $2
		  AND s.timestamp >= $3
		  AND s.timestamp < $4
		ORDER BY s.timestamp, s.block_number, s.log_index
	`

	rows, err := s.pool.Query(ctx, query, protocol, chainID, fromTimestamp, toTimestamp)
	if err != nil {
		return nil, fmt.Errorf("querying records for archival: %w", err)
	}
	defer rows.Close()

	var records []ParquetRecord
	for rows.Next() {
		var r ParquetRecord
		var dstChainID, dstBlockNumber *int64
		var dstTimestamp, metaLatency *int64
		var dstTxHash, dstReceiver *string
		var dstLogIndex *int32
		var payloadToken, payloadAmount, payloadData *string
		var payloadNonce *int64
		var metaFee, metaRelayer *string
		var metaGasUsed *int64

		err := rows.Scan(
			&r.MessageID, &r.Protocol, &r.Type, &r.Status, &r.CreatedAt, &r.UpdatedAt,
			&r.SrcChainID, &r.SrcTxHash, &r.SrcBlockNumber, &r.SrcTimestamp,
			&r.SrcSender, &r.SrcLogIndex,
			&dstChainID, &dstTxHash, &dstBlockNumber, &dstTimestamp,
			&dstReceiver, &dstLogIndex,
			&payloadToken, &payloadAmount, &payloadData, &payloadNonce,
			&metaFee, &metaRelayer, &metaGasUsed, &metaLatency,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning record: %w", err)
		}

		// Convert nullable int64 to *uint64 for destination fields
		if dstChainID != nil {
			v := uint64(*dstChainID)
			r.DstChainID = &v
		}
		if dstBlockNumber != nil {
			v := uint64(*dstBlockNumber)
			r.DstBlockNumber = &v
		}
		r.DstTimestamp = dstTimestamp
		r.DstTxHash = dstTxHash
		r.DstReceiver = dstReceiver
		r.DstLogIndex = dstLogIndex

		// Payload fields
		r.PayloadToken = payloadToken
		r.PayloadAmount = payloadAmount
		r.PayloadData = payloadData
		if payloadNonce != nil {
			v := uint64(*payloadNonce)
			r.PayloadNonce = &v
		}

		// Metadata fields
		r.MetaFee = metaFee
		r.MetaRelayer = metaRelayer
		if metaGasUsed != nil {
			v := uint64(*metaGasUsed)
			r.MetaGasUsed = &v
		}
		r.MetaLatencySeconds = metaLatency

		records = append(records, r)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating rows: %w", err)
	}

	return records, nil
}

// ArchivalRecord stores metadata about an archived file.
type ArchivalRecord struct {
	Protocol     string
	ChainID      uint64
	YearMonth    string
	RowCount     int64
	MinTimestamp int64
	MaxTimestamp int64
	FileSize     int64
	ObjectKey    string
	Checksum     string // SHA-256 hex digest of the Parquet bytes
	ArchivedAt   time.Time
}

// UpsertArchivalCursor records that a (protocol, chain, year-month) has been archived.
func (s *ArchiveStore) UpsertArchivalCursor(ctx context.Context, rec *ArchivalRecord) error {
	query := `
		INSERT INTO archival_cursors (protocol, chain_id, year_month, row_count,
			min_timestamp, max_timestamp, file_size, object_key, checksum, archived_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (protocol, chain_id, year_month) DO UPDATE SET
			row_count = EXCLUDED.row_count,
			min_timestamp = EXCLUDED.min_timestamp,
			max_timestamp = EXCLUDED.max_timestamp,
			file_size = EXCLUDED.file_size,
			object_key = EXCLUDED.object_key,
			checksum = EXCLUDED.checksum,
			archived_at = NOW()
	`
	_, err := s.pool.Exec(ctx, query,
		rec.Protocol, rec.ChainID, rec.YearMonth, rec.RowCount,
		rec.MinTimestamp, rec.MaxTimestamp, rec.FileSize, rec.ObjectKey, rec.Checksum,
	)
	if err != nil {
		return fmt.Errorf("upserting archival cursor: %w", err)
	}
	return nil
}

// ListArchivalCursors returns all archival records, used to build the manifest.
func (s *ArchiveStore) ListArchivalCursors(ctx context.Context) ([]ArchivalRecord, error) {
	query := `
		SELECT protocol, chain_id, year_month, row_count,
		       min_timestamp, max_timestamp, file_size, object_key, checksum, archived_at
		FROM archival_cursors
		ORDER BY protocol, chain_id, year_month
	`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing archival cursors: %w", err)
	}
	defer rows.Close()

	var records []ArchivalRecord
	for rows.Next() {
		var r ArchivalRecord
		err := rows.Scan(
			&r.Protocol, &r.ChainID, &r.YearMonth, &r.RowCount,
			&r.MinTimestamp, &r.MaxTimestamp, &r.FileSize,
			&r.ObjectKey, &r.Checksum, &r.ArchivedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning archival cursor: %w", err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
