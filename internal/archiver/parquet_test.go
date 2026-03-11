package archiver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockArchiveStore struct {
	objects map[string][]byte
}

func newMockArchiveStore() *mockArchiveStore {
	return &mockArchiveStore{
		objects: make(map[string][]byte),
	}
}

func (m *mockArchiveStore) Upload(_ context.Context, key string, reader io.Reader, _ int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *mockArchiveStore) DownloadBytes(_ context.Context, key string) ([]byte, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("object not found: %s", key)
	}
	return bytes.Clone(data), nil
}

func TestEventToParquet(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc",
		LogIndex:    3,
		Timestamp:   1700000000,
		EventType:   "PacketSent",
		Data: map[string]string{
			"sender":       "0x1111",
			"receiver":     "0x2222",
			"src_chain_id": "1",
			"dst_chain_id": "56",
			"nonce":        "42",
			"guid":         "0xguid",
			"amount_sent":  "1000",
			"message":      "0xdeadbeef",
		},
	}

	pe := EventToParquet(event)

	assert.Equal(t, "layerzero_v2", pe.Protocol)
	assert.Equal(t, uint64(1), pe.ChainID)
	assert.Equal(t, uint64(19000000), pe.BlockNumber)
	assert.Equal(t, "0xabc", pe.TxHash)
	assert.Equal(t, uint(3), pe.LogIndex)
	assert.Equal(t, int64(1700000000), pe.Timestamp)
	assert.Equal(t, "PacketSent", pe.EventType)
	assert.Equal(t, "0x1111", pe.Sender)
	assert.Equal(t, "0x2222", pe.Receiver)
	assert.Equal(t, "1", pe.SrcChainID)
	assert.Equal(t, "56", pe.DstChainID)
	assert.Equal(t, "42", pe.Nonce)
	assert.Equal(t, "0xguid", pe.GUID)
	assert.Equal(t, "1000", pe.Amount)
	assert.Equal(t, "0xdeadbeef", pe.Message)
}

func TestSerializeEvents(t *testing.T) {
	events := []*decoder.RawEvent{
		{
			Protocol:    "layerzero_v2",
			ChainID:     1,
			BlockNumber: 19000000,
			TxHash:      "0xabc",
			LogIndex:    0,
			Timestamp:   1700000000,
			EventType:   "PacketSent",
			Data: map[string]string{
				"sender": "0x1111",
				"guid":   "0xguid1",
			},
		},
		{
			Protocol:    "layerzero_v2",
			ChainID:     1,
			BlockNumber: 19000001,
			TxHash:      "0xdef",
			LogIndex:    1,
			Timestamp:   1700000012,
			EventType:   "PacketSent",
			Data: map[string]string{
				"sender": "0x2222",
				"guid":   "0xguid2",
			},
		},
	}

	data, err := SerializeEvents(events)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// Verify it's valid JSON Lines (each line is a JSON object)
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	assert.Len(t, lines, 2)

	var pe1, pe2 ParquetEvent
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &pe1))
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &pe2))

	assert.Equal(t, "0xguid1", pe1.GUID)
	assert.Equal(t, "0xguid2", pe2.GUID)
	assert.Equal(t, uint64(19000000), pe1.BlockNumber)
	assert.Equal(t, uint64(19000001), pe2.BlockNumber)
}

func TestSerializeEvents_Empty(t *testing.T) {
	_, err := SerializeEvents(nil)
	require.Error(t, err)

	_, err = SerializeEvents([]*decoder.RawEvent{})
	require.Error(t, err)
}

func TestArchiveKey(t *testing.T) {
	tests := []struct {
		name     string
		protocol string
		chainID  uint64
		time     time.Time
		want     string
	}{
		{
			name:     "normal",
			protocol: "layerzero_v2",
			chainID:  1,
			time:     time.Date(2024, 11, 15, 0, 0, 0, 0, time.UTC),
			want:     "protocols/layerzero_v2/1/2024-11.jsonl",
		},
		{
			name:     "single digit month",
			protocol: "layerzero_v2",
			chainID:  42161,
			time:     time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC),
			want:     "protocols/layerzero_v2/42161/2025-03.jsonl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ArchiveKey(tt.protocol, tt.chainID, tt.time)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSerializeEvents_ValidJSON(t *testing.T) {
	event := &decoder.RawEvent{
		Protocol:    "layerzero_v2",
		ChainID:     1,
		BlockNumber: 19000000,
		TxHash:      "0xabc",
		LogIndex:    0,
		Timestamp:   1700000000,
		EventType:   "PacketSent",
		Data:        map[string]string{},
	}

	data, err := SerializeEvents([]*decoder.RawEvent{event})
	require.NoError(t, err)

	// Should be parseable back
	var pe ParquetEvent
	err = json.Unmarshal(data, &pe)
	require.NoError(t, err)
	assert.Equal(t, "layerzero_v2", pe.Protocol)
}

func TestArchiveBatch_MergesExistingArchiveAndManifest(t *testing.T) {
	store := newMockArchiveStore()
	arch := New(store, zerolog.Nop())

	ctx := context.Background()
	firstBatch := []*decoder.RawEvent{
		{
			Protocol:    "layerzero_v2",
			ChainID:     1,
			BlockNumber: 19000000,
			TxHash:      "0xabc",
			LogIndex:    0,
			Timestamp:   1700000000,
			EventType:   "PacketSent",
			Data: map[string]string{
				"guid": "0xguid1",
			},
		},
	}

	secondBatch := []*decoder.RawEvent{
		{
			Protocol:    "layerzero_v2",
			ChainID:     1,
			BlockNumber: 19000000,
			TxHash:      "0xabc",
			LogIndex:    0,
			Timestamp:   1700000000,
			EventType:   "PacketSent",
			Data: map[string]string{
				"guid": "0xguid1",
			},
		},
		{
			Protocol:    "layerzero_v2",
			ChainID:     1,
			BlockNumber: 19000001,
			TxHash:      "0xdef",
			LogIndex:    1,
			Timestamp:   1700000120,
			EventType:   "PacketReceived",
			Data: map[string]string{
				"guid": "0xguid2",
			},
		},
	}

	require.NoError(t, arch.ArchiveBatch(ctx, firstBatch))
	require.NoError(t, arch.ArchiveBatch(ctx, secondBatch))

	key := ArchiveKey("layerzero_v2", 1, time.Unix(1700000000, 0).UTC())
	data, err := store.DownloadBytes(ctx, key)
	require.NoError(t, err)

	events, err := deserializeParquetEvents(data)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "0xguid1", events[0].GUID)
	assert.Equal(t, "0xguid2", events[1].GUID)

	manifestData, err := store.DownloadBytes(ctx, "manifests/index.json")
	require.NoError(t, err)

	var manifest Manifest
	require.NoError(t, json.Unmarshal(manifestData, &manifest))
	require.Len(t, manifest.Files, 1)
	assert.Equal(t, key, manifest.Files[0].Key)
	assert.Equal(t, 2, manifest.Files[0].RowCount)
	assert.Equal(t, int64(1700000000), manifest.Files[0].TimeRange.From)
	assert.Equal(t, int64(1700000120), manifest.Files[0].TimeRange.To)
}
