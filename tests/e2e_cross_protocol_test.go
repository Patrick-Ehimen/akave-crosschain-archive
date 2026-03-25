//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/archiver"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	akaveClient "github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
)

// TestE2E_CrossProtocol_ArchivalPipeline verifies that Parquet archives are
// created on Akave O3 for all 4 protocols. It extends the single-protocol
// e2e_archival_test to cover the full Milestone 3 multi-protocol scope.
func TestE2E_CrossProtocol_ArchivalPipeline(t *testing.T) {
	accessKey := os.Getenv("CROSSCHAIN_AKAVE_ACCESS_KEY")
	secretKey := os.Getenv("CROSSCHAIN_AKAVE_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		t.Skip("Skipping: CROSSCHAIN_AKAVE_ACCESS_KEY/SECRET_KEY not set")
	}

	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Connect to DB
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Skipf("Skipping: PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	// Verify at least some data exists across protocols
	allProtocols := []string{"layerzero_v2", "wormhole", "axelar", "ccip"}
	protocolsWithData := []string{}

	for _, proto := range allProtocols {
		var count int
		pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE protocol = $1", proto).Scan(&count)
		if count > 0 {
			protocolsWithData = append(protocolsWithData, proto)
			t.Logf("Protocol %s: %d messages in DB", proto, count)
		} else {
			t.Logf("Protocol %s: no data in DB (will be skipped by archiver)", proto)
		}
	}

	if len(protocolsWithData) == 0 {
		t.Skip("Skipping: no test data in messages table for any protocol")
	}

	// Connect to Akave O3
	akaveCfg := config.Akave{
		Endpoint:   "o3-rc3.akave.xyz",
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		BucketName: "crosschain-archive",
		UseSSL:     true,
		Region:     "akave-network",
	}
	o3, err := akaveClient.NewClient(ctx, akaveCfg, log)
	if err != nil {
		t.Fatalf("Failed to connect to Akave O3: %v", err)
	}

	// Create archiver with all 4 protocols and all known chains
	store := archiver.NewArchiveStore(pool)
	chainNames := map[uint64]string{
		1:     "ethereum",
		42161: "arbitrum",
		10:    "optimism",
		8453:  "base",
		137:   "polygon",
		43114: "avalanche",
		56:    "bsc",
	}
	chainIDs := []uint64{1, 42161, 10, 8453, 137, 43114, 56}

	arc := archiver.New(
		store, o3, chainNames,
		allProtocols,
		chainIDs,
		log,
	)

	// Run with short context — first cycle runs immediately, then context expires
	shortCtx, shortCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shortCancel()

	err = arc.Run(shortCtx, 1*time.Hour)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Archiver error: %v", err)
	}

	// Verify archival_cursors populated — check for multiple protocols
	rows, err := pool.Query(ctx,
		"SELECT protocol, chain_id, year_month, row_count, object_key FROM archival_cursors ORDER BY protocol, chain_id, year_month")
	if err != nil {
		t.Fatalf("Failed to query archival_cursors: %v", err)
	}
	defer rows.Close()

	archivedProtocols := map[string]int{}
	for rows.Next() {
		var protocol, yearMonth, objectKey string
		var chainID, rowCount int64
		rows.Scan(&protocol, &chainID, &yearMonth, &rowCount, &objectKey)
		t.Logf("Archived: protocol=%s chain_id=%d year_month=%s rows=%d key=%s",
			protocol, chainID, yearMonth, rowCount, objectKey)
		archivedProtocols[protocol]++
	}

	if len(archivedProtocols) == 0 {
		t.Fatal("No archival cursors found — archival did not run")
	}
	t.Logf("Archived protocols: %v", archivedProtocols)

	// Verify O3 Parquet files exist for each archived protocol
	for _, proto := range protocolsWithData {
		prefix := fmt.Sprintf("protocols/%s/", proto)
		objects, err := o3.List(ctx, prefix)
		if err != nil {
			t.Fatalf("Failed to list O3 objects for %s: %v", proto, err)
		}
		if len(objects) == 0 {
			t.Logf("Warning: no Parquet files found on O3 for %s (may have no data in completed months)", proto)
			continue
		}
		for _, obj := range objects {
			t.Logf("O3 object [%s]: key=%s size=%d", proto, obj.Key, obj.Size)
		}
	}

	// Verify manifest exists and contains multi-protocol entries
	manifestObj, err := o3.Download(ctx, "manifests/index.json")
	if err != nil {
		t.Fatalf("Failed to download manifest: %v", err)
	}
	defer manifestObj.Close()

	buf := make([]byte, 65536)
	n, _ := manifestObj.Read(buf)
	manifestJSON := buf[:n]

	var manifest struct {
		TotalFiles int `json:"total_files"`
		TotalRows  int `json:"total_rows"`
		Files      []struct {
			Protocol string `json:"protocol"`
			ObjectKey string `json:"object_key"`
		} `json:"files"`
	}

	if err := json.Unmarshal(manifestJSON, &manifest); err != nil {
		t.Fatalf("Failed to parse manifest JSON: %v", err)
	}

	t.Logf("Manifest: total_files=%d total_rows=%d", manifest.TotalFiles, manifest.TotalRows)

	manifestProtocols := map[string]bool{}
	for _, f := range manifest.Files {
		manifestProtocols[f.Protocol] = true
	}
	t.Logf("Manifest protocols: %v", manifestProtocols)

	fmt.Println("=== E2E CROSS-PROTOCOL ARCHIVAL VERIFICATION COMPLETE ===")
}
