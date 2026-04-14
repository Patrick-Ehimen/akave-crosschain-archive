package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/archiver"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
)

// HistoricalStore provides access to archived file metadata from the database.
type HistoricalStore interface {
	ListArchivalCursors(ctx context.Context) ([]archiver.ArchivalRecord, error)
}

// GetHistoricalMessagesHandler serves archived Parquet data from Akave O3 for
// time ranges that have been rotated out of hot storage.
//
// GET /historical/messages?protocol=layerzero&chain_id=1&year_month=2024-01
//
// The handler downloads the Parquet file, verifies its SHA-256 checksum, and
// returns the raw records as JSON.
func GetHistoricalMessagesHandler(store HistoricalStore, o3 *akave.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		protocol := r.URL.Query().Get("protocol")
		chainIDStr := r.URL.Query().Get("chain_id")
		yearMonth := r.URL.Query().Get("year_month")

		if protocol == "" || chainIDStr == "" || yearMonth == "" {
			http.Error(w, `{"error":"protocol, chain_id and year_month are required"}`, http.StatusBadRequest)
			return
		}

		chainID, err := strconv.ParseUint(chainIDStr, 10, 64)
		if err != nil {
			http.Error(w, `{"error":"invalid chain_id"}`, http.StatusBadRequest)
			return
		}

		// Look up the archival record to get the object key and expected checksum.
		cursors, err := store.ListArchivalCursors(r.Context())
		if err != nil {
			http.Error(w, `{"error":"failed to query archival index"}`, http.StatusInternalServerError)
			return
		}

		var rec *archiver.ArchivalRecord
		for i := range cursors {
			c := &cursors[i]
			if c.Protocol == protocol && c.ChainID == chainID && c.YearMonth == yearMonth {
				rec = c
				break
			}
		}
		if rec == nil {
			http.Error(w, `{"error":"no archived data found for the requested period"}`, http.StatusNotFound)
			return
		}

		// Download from O3.
		obj, err := o3.Download(r.Context(), rec.ObjectKey)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to download archive: %s"}`, err), http.StatusBadGateway)
			return
		}
		defer obj.Close()

		data, err := io.ReadAll(obj)
		if err != nil {
			http.Error(w, `{"error":"failed to read archive data"}`, http.StatusInternalServerError)
			return
		}

		// Verify checksum if stored.
		if rec.Checksum != "" {
			h := sha256.New()
			h.Write(data)
			got := fmt.Sprintf("%x", h.Sum(nil))
			if got != rec.Checksum {
				http.Error(w, fmt.Sprintf(`{"error":"checksum mismatch: archive integrity check failed"}`), http.StatusInternalServerError)
				return
			}
		}

		// Deserialize Parquet to records.
		records, err := archiver.ReadParquet(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			http.Error(w, `{"error":"failed to deserialize archive"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Archive-Object", rec.ObjectKey)
		w.Header().Set("X-Archive-Checksum", rec.Checksum)
		w.Header().Set("X-Archive-Row-Count", strconv.FormatInt(rec.RowCount, 10))
		json.NewEncoder(w).Encode(map[string]any{
			"object_key":  rec.ObjectKey,
			"year_month":  rec.YearMonth,
			"protocol":    rec.Protocol,
			"chain_id":    rec.ChainID,
			"checksum":    rec.Checksum,
			"row_count":   len(records),
			"records":     records,
		})
	}
}

// GetHistoricalIndexHandler lists all available archived periods.
//
// GET /historical/index
func GetHistoricalIndexHandler(store HistoricalStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cursors, err := store.ListArchivalCursors(r.Context())
		if err != nil {
			http.Error(w, `{"error":"failed to query archival index"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"total": len(cursors),
			"files": cursors,
		})
	}
}

// ensure chi import is used for future route param access
var _ = chi.URLParam
