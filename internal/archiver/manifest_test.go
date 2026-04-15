package archiver

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuildManifest(t *testing.T) {
	records := []ArchivalRecord{
		{
			Protocol:     "layerzero_v2",
			ChainID:      1,
			YearMonth:    "2025-01",
			RowCount:     150,
			MinTimestamp: 1704067200,
			MaxTimestamp: 1706745599,
			FileSize:     32768,
			ObjectKey:    "protocols/layerzero_v2/ethereum/2025-01.parquet",
			ArchivedAt:   time.Now(),
		},
		{
			Protocol:     "layerzero_v2",
			ChainID:      42161,
			YearMonth:    "2025-01",
			RowCount:     300,
			MinTimestamp: 1704067200,
			MaxTimestamp: 1706745599,
			FileSize:     65536,
			ObjectKey:    "protocols/layerzero_v2/arbitrum/2025-01.parquet",
			ArchivedAt:   time.Now(),
		},
	}

	chainNames := map[uint64]string{
		1:     "ethereum",
		42161: "arbitrum",
	}

	manifest := BuildManifest(records, chainNames)

	if manifest.Version != 1 {
		t.Errorf("expected version 1, got %d", manifest.Version)
	}
	if manifest.TotalFiles != 2 {
		t.Errorf("expected 2 files, got %d", manifest.TotalFiles)
	}
	if manifest.TotalRows != 450 {
		t.Errorf("expected 450 total rows, got %d", manifest.TotalRows)
	}
	if manifest.Files[0].ChainName != "ethereum" {
		t.Errorf("expected chain name ethereum, got %s", manifest.Files[0].ChainName)
	}

	// Verify JSON round-trip
	buf, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest failed: %v", err)
	}

	var parsed Manifest
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("failed to parse manifest JSON: %v", err)
	}
	if parsed.TotalFiles != 2 {
		t.Errorf("parsed total_files: expected 2, got %d", parsed.TotalFiles)
	}
}

func TestBuildManifest_UnknownChain(t *testing.T) {
	records := []ArchivalRecord{
		{
			Protocol:  "layerzero_v2",
			ChainID:   999,
			YearMonth: "2025-01",
			RowCount:  10,
			ObjectKey: "protocols/layerzero_v2/chain_999/2025-01.parquet",
		},
	}

	manifest := BuildManifest(records, map[uint64]string{})

	if manifest.Files[0].ChainName != "chain_999" {
		t.Errorf("expected fallback chain name chain_999, got %s", manifest.Files[0].ChainName)
	}
}

func TestBuildManifest_Empty(t *testing.T) {
	manifest := BuildManifest(nil, map[uint64]string{})

	if manifest.TotalFiles != 0 {
		t.Errorf("expected 0 files, got %d", manifest.TotalFiles)
	}
	if manifest.TotalRows != 0 {
		t.Errorf("expected 0 total rows, got %d", manifest.TotalRows)
	}
}
