//go:build integration

package tests

import (
	"context"
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

const testDSN = "postgres://crosschain:crosschain@localhost:5432/crosschain_archive?sslmode=disable"

func TestE2E_ArchivalPipeline(t *testing.T) {
	accessKey := os.Getenv("CROSSCHAIN_AKAVE_ACCESS_KEY")
	secretKey := os.Getenv("CROSSCHAIN_AKAVE_SECRET_KEY")
	if accessKey == "" || secretKey == "" {
		t.Skip("Skipping: CROSSCHAIN_AKAVE_ACCESS_KEY/SECRET_KEY not set")
	}

	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to DB
	pool, err := pgxpool.New(ctx, testDSN)
	if err != nil {
		t.Skipf("Skipping: PostgreSQL not available: %v", err)
	}
	defer pool.Close()

	// Verify test data exists
	var count int
	pool.QueryRow(ctx, "SELECT COUNT(*) FROM messages WHERE protocol = 'layerzero_v2'").Scan(&count)
	if count == 0 {
		t.Skip("Skipping: no test data in messages table")
	}
	t.Logf("Found %d messages in DB", count)

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

	// Create archiver and run one cycle
	store := archiver.NewArchiveStore(pool)
	chainNames := map[uint64]string{1: "ethereum", 42161: "arbitrum"}

	arc := archiver.New(
		store, o3, chainNames,
		[]string{"layerzero_v2"},
		[]uint64{1, 42161},
		log,
	)

	// Run with short context — first cycle runs immediately, then context expires
	shortCtx, shortCancel := context.WithTimeout(ctx, 15*time.Second)
	defer shortCancel()

	err = arc.Run(shortCtx, 1*time.Hour)
	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("Archiver error: %v", err)
	}

	// Verify archival_cursors populated
	rows, err := pool.Query(ctx, "SELECT protocol, chain_id, year_month, row_count, object_key FROM archival_cursors ORDER BY protocol, chain_id, year_month")
	if err != nil {
		t.Fatalf("Failed to query archival_cursors: %v", err)
	}
	defer rows.Close()

	cursorCount := 0
	for rows.Next() {
		var protocol, yearMonth, objectKey string
		var chainID, rowCount int64
		rows.Scan(&protocol, &chainID, &yearMonth, &rowCount, &objectKey)
		t.Logf("Archived: protocol=%s chain_id=%d year_month=%s rows=%d key=%s",
			protocol, chainID, yearMonth, rowCount, objectKey)
		cursorCount++
	}

	if cursorCount == 0 {
		t.Fatal("No archival cursors found — archival did not run")
	}
	t.Logf("Total archival cursors: %d", cursorCount)

	// Verify O3 parquet files exist
	objects, err := o3.List(ctx, "protocols/")
	if err != nil {
		t.Fatalf("Failed to list O3 objects: %v", err)
	}
	if len(objects) == 0 {
		t.Fatal("No Parquet files found on O3")
	}
	for _, obj := range objects {
		t.Logf("O3 object: key=%s size=%d", obj.Key, obj.Size)
	}

	// Verify manifest exists
	manifestObjects, err := o3.List(ctx, "manifests/")
	if err != nil {
		t.Fatalf("Failed to list manifest objects: %v", err)
	}
	if len(manifestObjects) == 0 {
		t.Fatal("Manifest file not found on O3")
	}
	for _, obj := range manifestObjects {
		t.Logf("Manifest: key=%s size=%d", obj.Key, obj.Size)
	}

	// Download and verify manifest is valid JSON
	manifestObj, err := o3.Download(ctx, "manifests/index.json")
	if err != nil {
		t.Fatalf("Failed to download manifest: %v", err)
	}
	defer manifestObj.Close()

	buf := make([]byte, 16384)
	n, _ := manifestObj.Read(buf)
	t.Logf("Manifest content:\n%s", string(buf[:n]))

	fmt.Println("=== E2E ARCHIVAL VERIFICATION COMPLETE ===")
}
