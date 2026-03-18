package archiver

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"
)

// ManifestKey is the O3 object key for the manifest file.
const ManifestKey = "manifests/index.json"

// Manifest represents the index of all archived Parquet files.
type Manifest struct {
	Version    int             `json:"version"`
	UpdatedAt  time.Time       `json:"updated_at"`
	TotalFiles int             `json:"total_files"`
	TotalRows  int64           `json:"total_rows"`
	Files      []ManifestEntry `json:"files"`
}

// ManifestEntry describes a single archived Parquet file.
type ManifestEntry struct {
	ObjectKey   string    `json:"object_key"`
	Protocol    string    `json:"protocol"`
	ChainID     uint64    `json:"chain_id"`
	ChainName   string    `json:"chain_name"`
	YearMonth   string    `json:"year_month"`
	RowCount    int64     `json:"row_count"`
	FileSize    int64     `json:"file_size"`
	MinTimestamp int64    `json:"min_timestamp"`
	MaxTimestamp int64    `json:"max_timestamp"`
	ArchivedAt  time.Time `json:"archived_at"`
}

// BuildManifest creates a Manifest from archival records and a chain name resolver.
func BuildManifest(records []ArchivalRecord, chainNames map[uint64]string) *Manifest {
	entries := make([]ManifestEntry, 0, len(records))
	var totalRows int64

	for _, r := range records {
		chainName := chainNames[r.ChainID]
		if chainName == "" {
			chainName = fmt.Sprintf("chain_%d", r.ChainID)
		}
		entries = append(entries, ManifestEntry{
			ObjectKey:   r.ObjectKey,
			Protocol:    r.Protocol,
			ChainID:     r.ChainID,
			ChainName:   chainName,
			YearMonth:   r.YearMonth,
			RowCount:    r.RowCount,
			FileSize:    r.FileSize,
			MinTimestamp: r.MinTimestamp,
			MaxTimestamp: r.MaxTimestamp,
			ArchivedAt:  r.ArchivedAt,
		})
		totalRows += r.RowCount
	}

	return &Manifest{
		Version:    1,
		UpdatedAt:  time.Now(),
		TotalFiles: len(entries),
		TotalRows:  totalRows,
		Files:      entries,
	}
}

// MarshalManifest serializes a Manifest to pretty-printed JSON bytes.
func MarshalManifest(m *Manifest) (*bytes.Buffer, error) {
	buf := new(bytes.Buffer)
	enc := json.NewEncoder(buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return nil, fmt.Errorf("marshaling manifest: %w", err)
	}
	return buf, nil
}
