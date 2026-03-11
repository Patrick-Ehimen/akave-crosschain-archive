package archiver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
)

// ParquetEvent is the flattened structure serialized to Parquet/JSON lines for archival.
// Note: Once parquet-go is added as a dependency, this will use Parquet struct tags.
// For now, we use JSON Lines format which is equally valid for archival and readable from O3.
type ParquetEvent struct {
	Protocol    string `json:"protocol"`
	ChainID     uint64 `json:"chain_id"`
	BlockNumber uint64 `json:"block_number"`
	TxHash      string `json:"tx_hash"`
	LogIndex    uint   `json:"log_index"`
	Timestamp   int64  `json:"timestamp"`
	EventType   string `json:"event_type"`
	Sender      string `json:"sender,omitempty"`
	Receiver    string `json:"receiver,omitempty"`
	SrcChainID  string `json:"src_chain_id,omitempty"`
	DstChainID  string `json:"dst_chain_id,omitempty"`
	Nonce       string `json:"nonce,omitempty"`
	GUID        string `json:"guid,omitempty"`
	Amount      string `json:"amount,omitempty"`
	Message     string `json:"message,omitempty"`
}

// ManifestEntry describes a single archived file in the manifest.
type ManifestEntry struct {
	Key       string `json:"key"`
	RowCount  int    `json:"row_count"`
	TimeRange struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"time_range"`
	ArchivedAt time.Time `json:"archived_at"`
}

// Manifest is the index file listing all archived files.
type Manifest struct {
	Protocol  string          `json:"protocol"`
	Files     []ManifestEntry `json:"files"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type archiveStore interface {
	Upload(ctx context.Context, key string, reader io.Reader, size int64) error
	DownloadBytes(ctx context.Context, key string) ([]byte, error)
}

// Archiver handles serialization of events and upload to Akave O3.
type Archiver struct {
	o3  archiveStore
	log zerolog.Logger
}

// New creates a new Archiver with the provided O3 client and logger.
func New(o3 archiveStore, log zerolog.Logger) *Archiver {
	return &Archiver{
		o3:  o3,
		log: log.With().Str("component", "archiver").Logger(),
	}
}

// EventToParquet converts a RawEvent to a flattened ParquetEvent for archival.
func EventToParquet(event *decoder.RawEvent) *ParquetEvent {
	return &ParquetEvent{
		Protocol:    event.Protocol,
		ChainID:     event.ChainID,
		BlockNumber: event.BlockNumber,
		TxHash:      event.TxHash,
		LogIndex:    event.LogIndex,
		Timestamp:   event.Timestamp,
		EventType:   event.EventType,
		Sender:      event.Data["sender"],
		Receiver:    event.Data["receiver"],
		SrcChainID:  event.Data["src_chain_id"],
		DstChainID:  event.Data["dst_chain_id"],
		Nonce:       event.Data["nonce"],
		GUID:        event.Data["guid"],
		Amount:      event.Data["amount_sent"],
		Message:     event.Data["message"],
	}
}

// SerializeEvents serializes a batch of RawEvents to JSON Lines format.
// Each line is a complete JSON object representing one event.
// This format is compatible with Parquet conversion tools and readable as-is.
func SerializeEvents(events []*decoder.RawEvent) ([]byte, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("no events to serialize")
	}

	parquetEvents := make([]*ParquetEvent, 0, len(events))
	for _, event := range events {
		parquetEvents = append(parquetEvents, EventToParquet(event))
	}

	return serializeParquetEvents(parquetEvents)
}

func serializeParquetEvents(events []*ParquetEvent) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)

	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return nil, fmt.Errorf("encoding event: %w", err)
		}
	}

	return buf.Bytes(), nil
}

func deserializeParquetEvents(data []byte) ([]*ParquetEvent, error) {
	if len(data) == 0 {
		return nil, nil
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	events := make([]*ParquetEvent, 0)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		var event ParquetEvent
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("decoding archived event: %w", err)
		}
		events = append(events, &event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading archived events: %w", err)
	}

	return events, nil
}

// ArchiveKey generates the O3 object key for a batch of events.
// Format: protocols/{protocol}/{chain}/{year}-{month}.jsonl
func ArchiveKey(protocol string, chainID uint64, t time.Time) string {
	return fmt.Sprintf("protocols/%s/%d/%d-%02d.jsonl", protocol, chainID, t.Year(), t.Month())
}

func ManifestKey(protocol string) string {
	return fmt.Sprintf("manifests/%s/index.json", protocol)
}

// ArchiveBatch serializes and uploads a batch of events to Akave O3.
func (a *Archiver) ArchiveBatch(ctx context.Context, events []*decoder.RawEvent) error {
	if len(events) == 0 {
		return nil
	}

	// Use the first event's metadata for keying.
	first := events[0]
	t := time.Unix(first.Timestamp, 0).UTC()
	key := ArchiveKey(first.Protocol, first.ChainID, t)

	existingData, err := a.o3.DownloadBytes(ctx, key)
	if err != nil {
		if !errors.Is(err, akave.ErrObjectNotFound) {
			return fmt.Errorf("downloading existing archive %s: %w", key, err)
		}
		existingData = nil
	}

	existingEvents, err := deserializeParquetEvents(existingData)
	if err != nil {
		return fmt.Errorf("decoding existing archive %s: %w", key, err)
	}

	batchEvents := make([]*ParquetEvent, 0, len(events))
	for _, event := range events {
		batchEvents = append(batchEvents, EventToParquet(event))
	}

	mergedEvents := mergeParquetEvents(existingEvents, batchEvents)
	data, err := serializeParquetEvents(mergedEvents)
	if err != nil {
		return fmt.Errorf("serializing events: %w", err)
	}

	reader := bytes.NewReader(data)
	if err := a.o3.Upload(ctx, key, reader, int64(len(data))); err != nil {
		return fmt.Errorf("uploading to O3: %w", err)
	}

	entry := buildManifestEntry(key, mergedEvents)
	if err := a.UpdateManifest(ctx, first.Protocol, entry); err != nil {
		return fmt.Errorf("updating manifest: %w", err)
	}

	a.log.Info().
		Str("key", key).
		Int("events", len(mergedEvents)).
		Int("bytes", len(data)).
		Msg("Archived events to O3")

	return nil
}

// UpdateManifest reads the current manifest, adds a new entry, and uploads it back.
func (a *Archiver) UpdateManifest(ctx context.Context, protocol string, entry ManifestEntry) error {
	manifestKey := ManifestKey(protocol)

	// Try to read existing manifest.
	var manifest Manifest
	data, err := a.o3.DownloadBytes(ctx, manifestKey)
	if err != nil {
		if !errors.Is(err, akave.ErrObjectNotFound) {
			return fmt.Errorf("downloading manifest %s: %w", manifestKey, err)
		}
	} else if len(data) > 0 {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("unmarshaling manifest: %w", err)
		}
	}

	manifest.Protocol = protocol
	updated := false
	for i := range manifest.Files {
		if manifest.Files[i].Key == entry.Key {
			manifest.Files[i] = entry
			updated = true
			break
		}
	}
	if !updated {
		manifest.Files = append(manifest.Files, entry)
	}
	manifest.UpdatedAt = time.Now().UTC()

	marshaledData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	reader := bytes.NewReader(marshaledData)
	if err := a.o3.Upload(ctx, manifestKey, reader, int64(len(marshaledData))); err != nil {
		return fmt.Errorf("uploading manifest: %w", err)
	}

	a.log.Info().Int("total_files", len(manifest.Files)).Msg("Updated manifest")

	return nil
}

func buildManifestEntry(key string, events []*ParquetEvent) ManifestEntry {
	entry := ManifestEntry{
		Key:        key,
		RowCount:   len(events),
		ArchivedAt: time.Now().UTC(),
	}

	if len(events) == 0 {
		return entry
	}

	entry.TimeRange.From = events[0].Timestamp
	entry.TimeRange.To = events[0].Timestamp

	for _, event := range events[1:] {
		if event.Timestamp < entry.TimeRange.From {
			entry.TimeRange.From = event.Timestamp
		}
		if event.Timestamp > entry.TimeRange.To {
			entry.TimeRange.To = event.Timestamp
		}
	}

	return entry
}

func mergeParquetEvents(existing, incoming []*ParquetEvent) []*ParquetEvent {
	merged := make([]*ParquetEvent, 0, len(existing)+len(incoming))
	seen := make(map[string]struct{}, len(existing)+len(incoming))

	for _, event := range append(existing, incoming...) {
		if event == nil {
			continue
		}

		id := parquetEventID(event)
		if _, ok := seen[id]; ok {
			continue
		}

		seen[id] = struct{}{}
		merged = append(merged, event)
	}

	return merged
}

func parquetEventID(event *ParquetEvent) string {
	return fmt.Sprintf("%s|%d|%d|%s|%d", event.EventType, event.ChainID, event.BlockNumber, event.TxHash, event.LogIndex)
}
