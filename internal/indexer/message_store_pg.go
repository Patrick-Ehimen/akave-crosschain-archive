package indexer

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// PostgresMessageStore implements MessageStore using PostgreSQL.
type PostgresMessageStore struct {
	pool *pgxpool.Pool
}

// NewPostgresMessageStore creates a new PostgresMessageStore.
func NewPostgresMessageStore(pool *pgxpool.Pool) *PostgresMessageStore {
	return &PostgresMessageStore{pool: pool}
}

// UpsertMessage inserts a new message or updates an existing one across all tables.
func (s *PostgresMessageStore) UpsertMessage(ctx context.Context, msg *types.Message) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Upsert messages table
	_, err = tx.Exec(ctx, `
		INSERT INTO messages (message_id, protocol, type, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (message_id) DO UPDATE SET
			type = CASE WHEN EXCLUDED.type != 'message' THEN EXCLUDED.type ELSE messages.type END,
			updated_at = EXCLUDED.updated_at`,
		msg.MessageID, msg.Protocol, string(msg.Type), string(msg.Status),
		msg.CreatedAt, msg.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upserting message: %w", err)
	}

	// Upsert message_sources table
	_, err = tx.Exec(ctx, `
		INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (message_id) DO NOTHING`,
		msg.MessageID, msg.Source.ChainID, msg.Source.TxHash,
		msg.Source.BlockNumber, msg.Source.Timestamp, msg.Source.Sender, msg.Source.LogIndex,
	)
	if err != nil {
		return fmt.Errorf("upserting message source: %w", err)
	}

	// Upsert message_payloads table
	if msg.Payload != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_payloads (message_id, token, amount, data, nonce)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (message_id) DO UPDATE SET
				token = COALESCE(NULLIF(EXCLUDED.token, ''), message_payloads.token),
				amount = COALESCE(NULLIF(EXCLUDED.amount, ''), message_payloads.amount),
				data = COALESCE(NULLIF(EXCLUDED.data, ''), message_payloads.data)`,
			msg.MessageID, msg.Payload.Token, msg.Payload.Amount,
			msg.Payload.Data, msg.Payload.Nonce,
		)
		if err != nil {
			return fmt.Errorf("upserting message payload: %w", err)
		}
	}

	// Upsert message_metadata table
	if msg.Metadata != nil {
		_, err = tx.Exec(ctx, `
			INSERT INTO message_metadata (message_id, fee, relayer, gas_used, latency_seconds)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (message_id) DO NOTHING`,
			msg.MessageID, msg.Metadata.Fee, msg.Metadata.Relayer,
			msg.Metadata.GasUsed, msg.Metadata.LatencySeconds,
		)
		if err != nil {
			return fmt.Errorf("upserting message metadata: %w", err)
		}
	}

	return tx.Commit(ctx)
}

// FindByCorrelationKey looks up an existing pending message by composite key.
func (s *PostgresMessageStore) FindByCorrelationKey(ctx context.Context, key *normalizer.CorrelationKey) (*types.Message, error) {
	row := s.pool.QueryRow(ctx, `
		SELECT m.message_id, m.protocol, m.type, m.status, m.created_at, m.updated_at,
		       s.chain_id, s.tx_hash, s.block_number, s.timestamp, s.sender, s.log_index,
		       p.token, p.amount, p.data, p.nonce
		FROM messages m
		JOIN message_sources s ON m.message_id = s.message_id
		JOIN message_payloads p ON m.message_id = p.message_id
		WHERE m.protocol = $1
		  AND p.nonce = $2
		  AND s.sender = $3
		  AND s.chain_id = $4
		  AND m.status = 'pending'
		LIMIT 1`,
		key.Protocol, key.Nonce, key.Sender, key.SrcChainID,
	)

	msg := &types.Message{
		Payload: &types.Payload{},
	}
	var msgType, msgStatus string
	var token, amount, data *string

	err := row.Scan(
		&msg.MessageID, &msg.Protocol, &msgType, &msgStatus, &msg.CreatedAt, &msg.UpdatedAt,
		&msg.Source.ChainID, &msg.Source.TxHash, &msg.Source.BlockNumber,
		&msg.Source.Timestamp, &msg.Source.Sender, &msg.Source.LogIndex,
		&token, &amount, &data, &msg.Payload.Nonce,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying by correlation key: %w", err)
	}

	msg.Type = types.MessageType(msgType)
	msg.Status = types.MessageStatus(msgStatus)
	if token != nil {
		msg.Payload.Token = *token
	}
	if amount != nil {
		msg.Payload.Amount = *amount
	}
	if data != nil {
		msg.Payload.Data = *data
	}

	return msg, nil
}

// UpdateDestination sets the destination details and updates the message status.
func (s *PostgresMessageStore) UpdateDestination(ctx context.Context, messageID string, dest *types.Destination, status types.MessageStatus) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

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
			log_index = EXCLUDED.log_index`,
		messageID, dest.ChainID, dest.TxHash, dest.BlockNumber,
		dest.Timestamp, dest.Receiver, dest.LogIndex,
	)
	if err != nil {
		return fmt.Errorf("upserting destination: %w", err)
	}

	// Update message status
	_, err = tx.Exec(ctx, `
		UPDATE messages SET status = $2, updated_at = NOW() WHERE message_id = $1`,
		messageID, string(status),
	)
	if err != nil {
		return fmt.Errorf("updating message status: %w", err)
	}

	// Compute and store latency when both timestamps are available
	if dest.Timestamp > 0 {
		_, err = tx.Exec(ctx, `
			UPDATE message_metadata SET latency_seconds = ($2 - s.timestamp)
			FROM message_sources s
			WHERE message_metadata.message_id = $1
			  AND s.message_id = $1
			  AND s.timestamp > 0`,
			messageID, dest.Timestamp,
		)
		if err != nil {
			return fmt.Errorf("updating latency: %w", err)
		}
	}

	return tx.Commit(ctx)
}
