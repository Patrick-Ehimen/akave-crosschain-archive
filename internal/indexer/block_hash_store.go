package indexer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BlockHashRecord stores a block's hash and parent hash for reorg detection.
type BlockHashRecord struct {
	ChainID     uint64
	BlockNumber uint64
	BlockHash   string
	ParentHash  string
}

// BlockHashStore manages block hash records for reorg detection.
type BlockHashStore interface {
	// SaveBlockHash persists the hash and parent hash for a block.
	SaveBlockHash(ctx context.Context, r BlockHashRecord) error

	// GetBlockHash retrieves the stored hash for a block number.
	// Returns ("", nil) if no record exists yet.
	GetBlockHash(ctx context.Context, chainID, blockNumber uint64) (blockHash, parentHash string, err error)

	// PruneBlockHashes deletes hash records older than keepBelowBlock to bound table size.
	PruneBlockHashes(ctx context.Context, chainID, keepBelowBlock uint64) error

	// DeleteBlockHashesFrom removes all records at or above fromBlock (used on reorg rewind).
	DeleteBlockHashesFrom(ctx context.Context, chainID, fromBlock uint64) error
}

// PostgresBlockHashStore implements BlockHashStore using PostgreSQL.
type PostgresBlockHashStore struct {
	pool *pgxpool.Pool
}

// NewPostgresBlockHashStore creates a new PostgreSQL-backed block hash store.
func NewPostgresBlockHashStore(pool *pgxpool.Pool) *PostgresBlockHashStore {
	return &PostgresBlockHashStore{pool: pool}
}

func (s *PostgresBlockHashStore) SaveBlockHash(ctx context.Context, r BlockHashRecord) error {
	const q = `
		INSERT INTO block_hashes (chain_id, block_number, block_hash, parent_hash)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chain_id, block_number) DO UPDATE
		  SET block_hash = EXCLUDED.block_hash,
		      parent_hash = EXCLUDED.parent_hash,
		      recorded_at = NOW()
	`
	_, err := s.pool.Exec(ctx, q, r.ChainID, r.BlockNumber, r.BlockHash, r.ParentHash)
	if err != nil {
		return fmt.Errorf("saving block hash for chain %d block %d: %w", r.ChainID, r.BlockNumber, err)
	}
	return nil
}

func (s *PostgresBlockHashStore) GetBlockHash(ctx context.Context, chainID, blockNumber uint64) (string, string, error) {
	const q = `SELECT block_hash, parent_hash FROM block_hashes WHERE chain_id = $1 AND block_number = $2`
	var blockHash, parentHash string
	err := s.pool.QueryRow(ctx, q, chainID, blockNumber).Scan(&blockHash, &parentHash)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil
		}
		return "", "", fmt.Errorf("getting block hash for chain %d block %d: %w", chainID, blockNumber, err)
	}
	return blockHash, parentHash, nil
}

func (s *PostgresBlockHashStore) PruneBlockHashes(ctx context.Context, chainID, keepBelowBlock uint64) error {
	const q = `DELETE FROM block_hashes WHERE chain_id = $1 AND block_number < $2`
	_, err := s.pool.Exec(ctx, q, chainID, keepBelowBlock)
	if err != nil {
		return fmt.Errorf("pruning block hashes for chain %d below %d: %w", chainID, keepBelowBlock, err)
	}
	return nil
}

func (s *PostgresBlockHashStore) DeleteBlockHashesFrom(ctx context.Context, chainID, fromBlock uint64) error {
	const q = `DELETE FROM block_hashes WHERE chain_id = $1 AND block_number >= $2`
	_, err := s.pool.Exec(ctx, q, chainID, fromBlock)
	if err != nil {
		return fmt.Errorf("deleting block hashes for chain %d from block %d: %w", chainID, fromBlock, err)
	}
	return nil
}
