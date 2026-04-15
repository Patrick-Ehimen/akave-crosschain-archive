package archiver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/metrics"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
)

// Archiver handles periodic serialization of indexed events to Parquet
// and upload to Akave O3 storage.
type Archiver struct {
	store      *ArchiveStore
	o3         *akave.Client
	chainNames map[uint64]string
	protocols  []string
	chainIDs   []uint64
	log        zerolog.Logger
}

// New creates a new Archiver.
func New(
	store *ArchiveStore,
	o3 *akave.Client,
	chainNames map[uint64]string,
	protocols []string,
	chainIDs []uint64,
	log zerolog.Logger,
) *Archiver {
	return &Archiver{
		store:      store,
		o3:         o3,
		chainNames: chainNames,
		protocols:  protocols,
		chainIDs:   chainIDs,
		log:        log.With().Str("component", "archiver").Logger(),
	}
}

// Run starts the archival loop with the given interval. It blocks until
// the context is cancelled. Runs one archival cycle immediately on start,
// then on each tick.
func (a *Archiver) Run(ctx context.Context, interval time.Duration) error {
	a.log.Info().Dur("interval", interval).Msg("Starting archival loop")

	// Run once immediately
	if err := a.runCycle(ctx); err != nil {
		a.log.Error().Err(err).Msg("Initial archival cycle failed")
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.log.Info().Msg("Archival loop stopping")
			return ctx.Err()
		case <-ticker.C:
			if err := a.runCycle(ctx); err != nil {
				a.log.Error().Err(err).Msg("Archival cycle failed")
			}
		}
	}
}

// runCycle performs one full archival sweep across all (protocol, chain) pairs.
func (a *Archiver) runCycle(ctx context.Context) error {
	a.log.Info().Msg("Starting archival cycle")

	// Archive all completed months up to the start of the current month.
	// The current month is excluded because it may still receive events.
	now := time.Now().UTC()
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)

	archived := 0

	for _, protocol := range a.protocols {
		for _, chainID := range a.chainIDs {
			n, err := a.archiveChainProtocol(ctx, protocol, chainID, currentMonthStart)
			if err != nil {
				a.log.Error().
					Err(err).
					Str("protocol", protocol).
					Uint64("chain_id", chainID).
					Msg("Failed to archive chain/protocol pair")
				continue
			}
			archived += n
		}
	}

	// Update manifest if any files were archived
	if archived > 0 {
		if err := a.updateManifest(ctx); err != nil {
			return fmt.Errorf("updating manifest: %w", err)
		}
	}

	a.log.Info().Int("files_archived", archived).Msg("Archival cycle complete")
	return nil
}

// archiveChainProtocol archives all complete months for a single
// (protocol, chainID) pair. Returns the number of files archived.
func (a *Archiver) archiveChainProtocol(
	ctx context.Context,
	protocol string,
	chainID uint64,
	beforeMonth time.Time,
) (int, error) {
	// Scan from a reasonable lower bound through the month before current.
	scanStart := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	count := 0

	for monthStart := scanStart; monthStart.Before(beforeMonth); monthStart = monthStart.AddDate(0, 1, 0) {
		select {
		case <-ctx.Done():
			return count, ctx.Err()
		default:
		}

		yearMonth := monthStart.Format("2006-01")
		monthEnd := monthStart.AddDate(0, 1, 0)

		fromTS := monthStart.Unix()
		toTS := monthEnd.Unix()

		records, err := a.store.FetchRecordsForArchival(ctx, protocol, chainID, fromTS, toTS)
		if err != nil {
			return count, fmt.Errorf("fetching records for %s chain %d %s: %w",
				protocol, chainID, yearMonth, err)
		}

		if len(records) == 0 {
			continue
		}

		chainName := a.chainNames[chainID]
		if chainName == "" {
			chainName = fmt.Sprintf("chain_%d", chainID)
		}

		objectKey := fmt.Sprintf("protocols/%s/%s/%s.parquet",
			protocol, chainName, yearMonth)

		// Serialize to Parquet with Snappy compression
		buf, err := WriteParquet(records)
		if err != nil {
			return count, fmt.Errorf("writing parquet for %s: %w", objectKey, err)
		}

		// Compute SHA-256 checksum before uploading
		h := sha256.New()
		if _, err := io.Copy(h, bytes.NewReader(buf.Bytes())); err != nil {
			return count, fmt.Errorf("computing checksum for %s: %w", objectKey, err)
		}
		checksum := fmt.Sprintf("%x", h.Sum(nil))

		// Upload to O3
		if err := a.o3.Upload(ctx, objectKey, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
			return count, fmt.Errorf("uploading %s: %w", objectKey, err)
		}

		// Verify round-trip: download and re-check checksum
		if err := a.verifyUpload(ctx, objectKey, checksum, buf.Len()); err != nil {
			a.log.Warn().Err(err).Str("object_key", objectKey).Msg("Checksum verification failed after upload")
		}

		// Compute min/max timestamps from records
		minTS, maxTS := records[0].SrcTimestamp, records[0].SrcTimestamp
		for _, r := range records[1:] {
			if r.SrcTimestamp < minTS {
				minTS = r.SrcTimestamp
			}
			if r.SrcTimestamp > maxTS {
				maxTS = r.SrcTimestamp
			}
		}

		// Record in archival_cursors
		if err := a.store.UpsertArchivalCursor(ctx, &ArchivalRecord{
			Protocol:     protocol,
			ChainID:      chainID,
			YearMonth:    yearMonth,
			RowCount:     int64(len(records)),
			MinTimestamp: minTS,
			MaxTimestamp: maxTS,
			FileSize:     int64(buf.Len()),
			ObjectKey:    objectKey,
			Checksum:     checksum,
		}); err != nil {
			return count, fmt.Errorf("recording archival cursor for %s: %w", objectKey, err)
		}

		chainIDStr := strconv.FormatUint(chainID, 10)
		metrics.ArchivedFilesTotal.WithLabelValues(protocol, chainIDStr).Inc()

		a.log.Info().
			Str("object_key", objectKey).
			Int("row_count", len(records)).
			Int("file_size", buf.Len()).
			Str("checksum", checksum).
			Str("year_month", yearMonth).
			Msg("Archived parquet file")

		count++
	}

	return count, nil
}

// verifyUpload downloads the object from O3 and confirms its SHA-256 checksum matches.
func (a *Archiver) verifyUpload(ctx context.Context, objectKey, expectedChecksum string, expectedSize int) error {
	obj, err := a.o3.Download(ctx, objectKey)
	if err != nil {
		return fmt.Errorf("downloading for verification: %w", err)
	}
	defer obj.Close()

	h := sha256.New()
	n, err := io.Copy(h, obj)
	if err != nil {
		return fmt.Errorf("reading for checksum: %w", err)
	}
	if int(n) != expectedSize {
		return fmt.Errorf("size mismatch: got %d want %d", n, expectedSize)
	}
	got := fmt.Sprintf("%x", h.Sum(nil))
	if got != expectedChecksum {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, expectedChecksum)
	}
	return nil
}

// updateManifest builds and uploads the manifest file to O3.
func (a *Archiver) updateManifest(ctx context.Context) error {
	cursors, err := a.store.ListArchivalCursors(ctx)
	if err != nil {
		return fmt.Errorf("listing archival cursors: %w", err)
	}

	manifest := BuildManifest(cursors, a.chainNames)

	buf, err := MarshalManifest(manifest)
	if err != nil {
		return err
	}

	if err := a.o3.Upload(ctx, ManifestKey, bytes.NewReader(buf.Bytes()), int64(buf.Len())); err != nil {
		return fmt.Errorf("uploading manifest: %w", err)
	}

	a.log.Info().
		Int("total_files", manifest.TotalFiles).
		Int64("total_rows", manifest.TotalRows).
		Msg("Updated manifest")

	return nil
}
