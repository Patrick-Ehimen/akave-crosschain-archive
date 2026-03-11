package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// MessageRepository provides CRUD operations for cross-chain messages.
type MessageRepository struct {
	pool *pgxpool.Pool
}

// NewMessageRepository creates a new message repository.
func NewMessageRepository(pool *pgxpool.Pool) *MessageRepository {
	return &MessageRepository{pool: pool}
}

// UpsertMessage inserts or updates a message and its related records
// (source, destination, payload, metadata) in a single transaction.
func (r *MessageRepository) UpsertMessage(ctx context.Context, msg *types.Message) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Upsert messages table
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (message_id) DO UPDATE SET
			status = EXCLUDED.status,
			updated_at = EXCLUDED.updated_at
	`, msg.MessageID, msg.Protocol, string(msg.Type), string(msg.Status), msg.CreatedAt, msg.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upserting message: %w", err)
	}

	// Upsert source
	if msg.Source.TxHash != "" {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (message_id) DO UPDATE SET
				chain_id = EXCLUDED.chain_id,
				tx_hash = EXCLUDED.tx_hash,
				block_number = EXCLUDED.block_number,
				timestamp = EXCLUDED.timestamp,
				sender = EXCLUDED.sender,
				log_index = EXCLUDED.log_index
		`, msg.MessageID, msg.Source.ChainID, msg.Source.TxHash, msg.Source.BlockNumber,
			msg.Source.Timestamp, msg.Source.Sender, msg.Source.LogIndex)
		if err != nil {
			return fmt.Errorf("upserting source: %w", err)
		}
	}

	// Upsert destination if present
	if msg.Destination != nil && msg.Destination.TxHash != "" {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (message_id) DO UPDATE SET
				chain_id = EXCLUDED.chain_id,
				tx_hash = EXCLUDED.tx_hash,
				block_number = EXCLUDED.block_number,
				timestamp = EXCLUDED.timestamp,
				receiver = EXCLUDED.receiver,
				log_index = EXCLUDED.log_index
		`, msg.MessageID, msg.Destination.ChainID, msg.Destination.TxHash, msg.Destination.BlockNumber,
			msg.Destination.Timestamp, msg.Destination.Receiver, msg.Destination.LogIndex)
		if err != nil {
			return fmt.Errorf("upserting destination: %w", err)
		}
	}

	// Upsert payload if present
	if msg.Payload != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_payloads (message_id, token, amount, data, nonce)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (message_id) DO UPDATE SET
				token = EXCLUDED.token,
				amount = EXCLUDED.amount,
				data = EXCLUDED.data,
				nonce = EXCLUDED.nonce
		`, msg.MessageID, msg.Payload.Token, msg.Payload.Amount, msg.Payload.Data, msg.Payload.Nonce)
		if err != nil {
			return fmt.Errorf("upserting payload: %w", err)
		}
	}

	// Upsert metadata if present
	if msg.Metadata != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_metadata (message_id, fee, relayer, gas_used, latency_seconds)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (message_id) DO UPDATE SET
				fee = EXCLUDED.fee,
				relayer = EXCLUDED.relayer,
				gas_used = EXCLUDED.gas_used,
				latency_seconds = EXCLUDED.latency_seconds
		`, msg.MessageID, msg.Metadata.Fee, msg.Metadata.Relayer, msg.Metadata.GasUsed, msg.Metadata.LatencySeconds)
		if err != nil {
			return fmt.Errorf("upserting metadata: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// GetMessage retrieves a message by its ID, including source, destination, payload, and metadata.
func (r *MessageRepository) GetMessage(ctx context.Context, messageID string) (*types.Message, error) {
	msg := &types.Message{}

	// Get core message
	err := r.pool.QueryRow(ctx, `
		SELECT message_id, protocol, type, status, created_at, updated_at
		FROM messages WHERE message_id = $1
	`, messageID).Scan(
		&msg.MessageID, &msg.Protocol, &msg.Type, &msg.Status,
		&msg.CreatedAt, &msg.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("querying message: %w", err)
	}

	// Get source
	var src types.Source
	err = r.pool.QueryRow(ctx, `
		SELECT chain_id, tx_hash, block_number, timestamp, sender, log_index
		FROM message_sources WHERE message_id = $1
	`, messageID).Scan(
		&src.ChainID, &src.TxHash, &src.BlockNumber,
		&src.Timestamp, &src.Sender, &src.LogIndex,
	)
	switch {
	case err == nil:
		msg.Source = src
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("querying source for message %s: %w", messageID, err)
	}

	// Get destination
	var dst types.Destination
	err = r.pool.QueryRow(ctx, `
		SELECT chain_id, tx_hash, block_number, timestamp, receiver, log_index
		FROM message_destinations WHERE message_id = $1
	`, messageID).Scan(
		&dst.ChainID, &dst.TxHash, &dst.BlockNumber,
		&dst.Timestamp, &dst.Receiver, &dst.LogIndex,
	)
	switch {
	case err == nil:
		msg.Destination = &dst
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("querying destination for message %s: %w", messageID, err)
	}

	// Get payload
	var payload types.Payload
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(token, ''), COALESCE(amount, ''), COALESCE(data, ''), COALESCE(nonce, 0)
		FROM message_payloads WHERE message_id = $1
	`, messageID).Scan(
		&payload.Token, &payload.Amount, &payload.Data, &payload.Nonce,
	)
	switch {
	case err == nil:
		msg.Payload = &payload
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("querying payload for message %s: %w", messageID, err)
	}

	// Get metadata
	var meta types.Metadata
	err = r.pool.QueryRow(ctx, `
		SELECT COALESCE(fee, ''), COALESCE(relayer, ''), COALESCE(gas_used, 0), COALESCE(latency_seconds, 0)
		FROM message_metadata WHERE message_id = $1
	`, messageID).Scan(
		&meta.Fee, &meta.Relayer, &meta.GasUsed, &meta.LatencySeconds,
	)
	switch {
	case err == nil:
		msg.Metadata = &meta
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, fmt.Errorf("querying metadata for message %s: %w", messageID, err)
	}

	return msg, nil
}

// UpdateStatus updates the status of a message.
func (r *MessageRepository) UpdateStatus(ctx context.Context, messageID string, status types.MessageStatus) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE messages SET status = $1, updated_at = NOW() WHERE message_id = $2
	`, string(status), messageID)
	if err != nil {
		return fmt.Errorf("updating status: %w", err)
	}
	return nil
}

// UpdateDestination sets the destination details for a message and updates its status.
func (r *MessageRepository) UpdateDestination(ctx context.Context, messageID string, dst *types.Destination, latencySeconds int64) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Update status to executed
	_, err = tx.Exec(ctx, `
		UPDATE messages SET status = $1, updated_at = NOW() WHERE message_id = $2
	`, string(types.StatusExecuted), messageID)
	if err != nil {
		return fmt.Errorf("updating message status: %w", err)
	}

	// Upsert destination
	_, err = tx.Exec(ctx, `
		INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (message_id) DO UPDATE SET
			chain_id = EXCLUDED.chain_id,
			tx_hash = EXCLUDED.tx_hash,
			block_number = EXCLUDED.block_number,
			timestamp = EXCLUDED.timestamp,
			receiver = EXCLUDED.receiver,
			log_index = EXCLUDED.log_index
	`, messageID, dst.ChainID, dst.TxHash, dst.BlockNumber,
		dst.Timestamp, dst.Receiver, dst.LogIndex)
	if err != nil {
		return fmt.Errorf("upserting destination: %w", err)
	}

	// Upsert latency into metadata
	if latencySeconds > 0 {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_metadata (message_id, latency_seconds)
			VALUES ($1, $2)
			ON CONFLICT (message_id) DO UPDATE SET latency_seconds = EXCLUDED.latency_seconds
		`, messageID, latencySeconds)
		if err != nil {
			return fmt.Errorf("upserting latency: %w", err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// ListByProtocol returns all messages for a given protocol.
func (r *MessageRepository) ListByProtocol(ctx context.Context, protocol string, limit int) ([]*types.Message, error) {
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.pool.Query(ctx, `
		SELECT message_id, protocol, type, status, created_at, updated_at
		FROM messages WHERE protocol = $1
		ORDER BY created_at DESC LIMIT $2
	`, protocol, limit)
	if err != nil {
		return nil, fmt.Errorf("querying messages: %w", err)
	}
	defer rows.Close()

	var messages []*types.Message
	for rows.Next() {
		msg := &types.Message{}
		if err := rows.Scan(&msg.MessageID, &msg.Protocol, &msg.Type, &msg.Status, &msg.CreatedAt, &msg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning message: %w", err)
		}
		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

// FindPendingByNonce finds a pending message by source chain, protocol, sender, and nonce.
// This is used by the correlator to match PacketReceived events to their PacketSent counterparts.
func (r *MessageRepository) FindPendingByNonce(ctx context.Context, protocol string, srcChainID uint64, sender string, nonce uint64) (*types.Message, error) {
	var msgID string

	err := r.pool.QueryRow(ctx, `
		SELECT m.message_id
		FROM messages m
		JOIN message_sources ms ON m.message_id = ms.message_id
		JOIN message_payloads mp ON m.message_id = mp.message_id
		WHERE m.protocol = $1
		  AND m.status = 'pending'
		  AND ms.chain_id = $2
		  AND ms.sender = $3
		  AND mp.nonce = $4
		LIMIT 1
	`, protocol, srcChainID, sender, nonce).Scan(&msgID)
	if err != nil {
		return nil, fmt.Errorf("finding pending message: %w", err)
	}

	return r.GetMessage(ctx, msgID)
}

// DeleteMessage removes a message and all its related records (cascading).
func (r *MessageRepository) DeleteMessage(ctx context.Context, messageID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM messages WHERE message_id = $1`, messageID)
	if err != nil {
		return fmt.Errorf("deleting message: %w", err)
	}
	return nil
}
