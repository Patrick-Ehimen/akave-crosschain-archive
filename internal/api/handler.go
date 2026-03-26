package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// DBPinger abstracts the Ping method from pgxpool.Pool for testability.
type DBPinger interface {
	Ping(ctx context.Context) error
}

// CursorInfo represents one row from the indexer_cursors table.
type CursorInfo struct {
	ChainID   int64     `json:"chain_id"`
	Protocol  string    `json:"protocol"`
	LastBlock int64     `json:"last_block"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IndexerStateQuerier retrieves the current indexer cursor state.
type IndexerStateQuerier interface {
	QueryCursors(ctx context.Context) ([]CursorInfo, error)
}

type healthResponse struct {
	Status   string        `json:"status"`
	Database string        `json:"database"`
	Indexer  *indexerState  `json:"indexer,omitempty"`
	Error    string        `json:"error,omitempty"`
}

type indexerState struct {
	Cursors []CursorInfo `json:"cursors"`
}

// HealthHandler returns an http.HandlerFunc that checks database connectivity
// and indexer state.
// On success it returns 200 with status, database, and indexer cursor info.
// On DB failure it returns 503 with {"status":"degraded","database":"disconnected","error":"..."}.
func HealthHandler(db DBPinger, idx IndexerStateQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp := healthResponse{
			Status:   "ok",
			Database: "connected",
		}

		if err := db.Ping(ctx); err != nil {
			resp.Status = "degraded"
			resp.Database = "disconnected"
			resp.Error = err.Error()
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(resp)
			return
		}

		cursors, err := idx.QueryCursors(ctx)
		if err != nil {
			resp.Indexer = &indexerState{Cursors: nil}
		} else {
			resp.Indexer = &indexerState{Cursors: cursors}
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	}
}
