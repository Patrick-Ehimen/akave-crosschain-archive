package indexer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CursorStore manages indexing cursors (last processed block) per chain and protocol.
type CursorStore interface {
	// LoadCursor returns the last processed block number for a given chain and protocol.
	// Returns 0 if no cursor exists (first run).
	LoadCursor(ctx context.Context, chainID uint64, protocol string) (uint64, error)

	// UpdateCursor atomically updates the last processed block for a chain and protocol.
	UpdateCursor(ctx context.Context, chainID uint64, protocol string, blockNumber uint64) error
}

// PostgresCursorStore implements CursorStore using PostgreSQL.
type PostgresCursorStore struct {
	pool *pgxpool.Pool
}

// NewPostgresCursorStore creates a new PostgreSQL-backed cursor store.
func NewPostgresCursorStore(pool *pgxpool.Pool) *PostgresCursorStore {
	return &PostgresCursorStore{
		pool: pool,
	}
}

// LoadCursor retrieves the last processed block number for a chain+protocol.
// Returns 0 if no cursor exists (indicating first run or new protocol).
func (s *PostgresCursorStore) LoadCursor(ctx context.Context, chainID uint64, protocol string) (uint64, error) {
	var lastBlock uint64

	query := `SELECT last_block FROM indexer_cursors WHERE chain_id = $1 AND protocol = $2`

	err := s.pool.QueryRow(ctx, query, chainID, protocol).Scan(&lastBlock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No cursor exists yet, return 0 (start from beginning)
			return 0, nil
		}
		return 0, fmt.Errorf("loading cursor for chain %d protocol %s: %w", chainID, protocol, err)
	}

	return lastBlock, nil
}

// UpdateCursor atomically updates or inserts the cursor for a chain+protocol.
// Uses PostgreSQL's ON CONFLICT to handle upserts atomically.
func (s *PostgresCursorStore) UpdateCursor(ctx context.Context, chainID uint64, protocol string, blockNumber uint64) error {
	query := `
		INSERT INTO indexer_cursors (chain_id, protocol, last_block, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (chain_id, protocol)
		DO UPDATE SET last_block = $3, updated_at = NOW()
	`

	_, err := s.pool.Exec(ctx, query, chainID, protocol, blockNumber)
	if err != nil {
		return fmt.Errorf("updating cursor for chain %d protocol %s to block %d: %w", chainID, protocol, blockNumber, err)
	}

	return nil
}
