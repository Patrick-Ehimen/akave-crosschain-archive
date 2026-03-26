package api

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PgCursorQuerier queries indexer_cursors using a pgx connection pool.
type PgCursorQuerier struct {
	pool *pgxpool.Pool
}

// NewPgCursorQuerier creates a PgCursorQuerier.
func NewPgCursorQuerier(pool *pgxpool.Pool) *PgCursorQuerier {
	return &PgCursorQuerier{pool: pool}
}

// QueryCursors returns all rows from indexer_cursors ordered by chain and protocol.
func (q *PgCursorQuerier) QueryCursors(ctx context.Context) ([]CursorInfo, error) {
	rows, err := q.pool.Query(ctx,
		`SELECT chain_id, protocol, last_block, updated_at FROM indexer_cursors ORDER BY chain_id, protocol`)
	if err != nil {
		return nil, fmt.Errorf("querying indexer cursors: %w", err)
	}
	defer rows.Close()

	var cursors []CursorInfo
	for rows.Next() {
		var c CursorInfo
		if err := rows.Scan(&c.ChainID, &c.Protocol, &c.LastBlock, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning cursor row: %w", err)
		}
		cursors = append(cursors, c)
	}
	return cursors, rows.Err()
}
