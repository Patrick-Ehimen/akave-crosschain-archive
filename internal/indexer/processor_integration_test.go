package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/archiver"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/correlator"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/layerzero"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

const layerZeroTestABI = `[
	{
		"anonymous": false,
		"inputs": [
			{"indexed": false, "internalType": "bytes", "name": "encodedPayload", "type": "bytes"},
			{"indexed": false, "internalType": "bytes", "name": "options", "type": "bytes"},
			{"indexed": false, "internalType": "address", "name": "sendLibrary", "type": "address"}
		],
		"name": "PacketSent",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{
				"components": [
					{"internalType": "uint32", "name": "srcEid", "type": "uint32"},
					{"internalType": "bytes32", "name": "sender", "type": "bytes32"},
					{"internalType": "uint64", "name": "nonce", "type": "uint64"}
				],
				"indexed": false,
				"internalType": "struct Origin",
				"name": "origin",
				"type": "tuple"
			},
			{"indexed": false, "internalType": "address", "name": "receiver", "type": "address"}
		],
		"name": "PacketReceived",
		"type": "event"
	}
]`

type fakeChainClient struct {
	latestBlock uint64
	logs        []ethtypes.Log
	timestamps  map[uint64]int64
	fetchCalls  int
}

func (f *fakeChainClient) LatestConfirmedBlock(context.Context) (uint64, error) {
	return f.latestBlock, nil
}

func (f *fakeChainClient) FetchLogs(_ context.Context, fromBlock, toBlock uint64, addresses []common.Address, topics [][]common.Hash) ([]ethtypes.Log, error) {
	f.fetchCalls++

	addressFilter := make(map[common.Address]struct{}, len(addresses))
	for _, address := range addresses {
		addressFilter[address] = struct{}{}
	}

	topicFilter := make(map[common.Hash]struct{})
	if len(topics) > 0 {
		for _, topic := range topics[0] {
			topicFilter[topic] = struct{}{}
		}
	}

	filtered := make([]ethtypes.Log, 0, len(f.logs))
	for _, log := range f.logs {
		if log.BlockNumber < fromBlock || log.BlockNumber > toBlock {
			continue
		}
		if len(addressFilter) > 0 {
			if _, ok := addressFilter[log.Address]; !ok {
				continue
			}
		}
		if len(topicFilter) > 0 && len(log.Topics) > 0 {
			if _, ok := topicFilter[log.Topics[0]]; !ok {
				continue
			}
		}
		filtered = append(filtered, log)
	}

	return filtered, nil
}

func (f *fakeChainClient) BlockTimestamp(_ context.Context, blockNumber uint64) (int64, error) {
	timestamp, ok := f.timestamps[blockNumber]
	if !ok {
		return 0, fmt.Errorf("missing timestamp for block %d", blockNumber)
	}
	return timestamp, nil
}

type memoryArchiveStore struct {
	objects map[string][]byte
}

func newMemoryArchiveStore() *memoryArchiveStore {
	return &memoryArchiveStore{
		objects: make(map[string][]byte),
	}
}

func (m *memoryArchiveStore) Upload(_ context.Context, key string, reader io.Reader, _ int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *memoryArchiveStore) DownloadBytes(_ context.Context, key string) ([]byte, error) {
	data, ok := m.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", akave.ErrObjectNotFound, key)
	}
	return bytes.Clone(data), nil
}

func TestProcessChain_LayerZeroCorrelationArchiveAndCursorPersistence(t *testing.T) {
	ctx := context.Background()
	pool, cleanup := setupProcessorTestDB(t)
	defer cleanup()

	repo := postgres.NewMessageRepository(pool)
	cursors := NewPostgresCursorStore(pool)
	corr := correlator.New(repo, zerolog.Nop())
	archiveStore := newMemoryArchiveStore()
	arch := archiver.New(archiveStore, zerolog.Nop())
	dec := layerzero.NewLayerZeroDecoder()

	hooks := ProcessorHooks{
		OnEvent:    corr.ProcessEvent,
		AfterBatch: arch.ArchiveBatch,
	}

	sourceGUID := "0x3333333333333333333333333333333333333333333333333333333333333333"
	sourceClient := &fakeChainClient{
		latestBlock: 10,
		logs: []ethtypes.Log{
			buildPacketSentLog(t, 1, 10, 0, "0xaaa111"),
		},
		timestamps: map[uint64]int64{
			10: 1700000000,
		},
	}

	runProcessorUntilCursor(t, 1, dec, sourceClient, cursors, hooks, 10)

	msg, err := repo.GetMessage(ctx, sourceGUID)
	require.NoError(t, err)
	assert.Equal(t, types.StatusPending, msg.Status)
	assert.Equal(t, int64(1700000000), msg.Source.Timestamp)

	sourceArchiveKey := archiver.ArchiveKey(dec.Protocol(), 1, time.Unix(1700000000, 0).UTC())
	sourceArchive := readArchivedEvents(t, archiveStore, sourceArchiveKey)
	require.Len(t, sourceArchive, 1)
	assert.Equal(t, decoder.EventPacketSent, sourceArchive[0]["event_type"])

	destinationClient := &fakeChainClient{
		latestBlock: 20,
		logs: []ethtypes.Log{
			buildPacketReceivedLog(t, 56, 20, 1, "0xbbb222"),
		},
		timestamps: map[uint64]int64{
			20: 1700000120,
		},
	}

	runProcessorUntilCursor(t, 56, dec, destinationClient, cursors, hooks, 20)

	msg, err = repo.GetMessage(ctx, sourceGUID)
	require.NoError(t, err)
	assert.Equal(t, types.StatusExecuted, msg.Status)
	require.NotNil(t, msg.Destination)
	assert.Equal(t, uint64(56), msg.Destination.ChainID)
	assert.Equal(t, common.HexToHash("0xbbb222").Hex(), msg.Destination.TxHash)
	require.NotNil(t, msg.Metadata)
	assert.Equal(t, int64(120), msg.Metadata.LatencySeconds)

	destinationArchiveKey := archiver.ArchiveKey(dec.Protocol(), 56, time.Unix(1700000120, 0).UTC())
	destinationArchive := readArchivedEvents(t, archiveStore, destinationArchiveKey)
	require.Len(t, destinationArchive, 1)
	assert.Equal(t, decoder.EventPacketReceived, destinationArchive[0]["event_type"])

	manifestBytes, err := archiveStore.DownloadBytes(ctx, archiver.ManifestKey(dec.Protocol()))
	require.NoError(t, err)

	var manifest archiver.Manifest
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	require.Len(t, manifest.Files, 2)

	sourceFetches := sourceClient.fetchCalls
	sourceArchiveBeforeRestart, err := archiveStore.DownloadBytes(ctx, sourceArchiveKey)
	require.NoError(t, err)

	restartCtx, cancel := context.WithCancel(ctx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- processChainInternal(
			restartCtx,
			1,
			dec.Protocol(),
			dec,
			sourceClient,
			cursors,
			100,
			5*time.Millisecond,
			zerolog.Nop(),
			hooks,
		)
	}()

	time.Sleep(25 * time.Millisecond)
	cancel()

	require.ErrorIs(t, <-errCh, context.Canceled)
	assert.Equal(t, sourceFetches, sourceClient.fetchCalls)

	sourceArchiveAfterRestart, err := archiveStore.DownloadBytes(ctx, sourceArchiveKey)
	require.NoError(t, err)
	assert.Equal(t, sourceArchiveBeforeRestart, sourceArchiveAfterRestart)

	messages, err := repo.ListByProtocol(ctx, dec.Protocol(), 10)
	require.NoError(t, err)
	require.Len(t, messages, 1)
}

func setupProcessorTestDB(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	ctx := context.Background()

	pool, err := postgres.NewPool(ctx, "postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable")
	if err != nil {
		t.Skip("Skipping test: PostgreSQL not available:", err)
	}

	err = postgres.RunMigrations(
		"postgres://postgres:password@localhost:5432/crosschain_test?sslmode=disable",
		"file://../../migrations",
	)
	if err != nil {
		t.Skip("Skipping test: migrations failed:", err)
	}

	cleanup := func() {
		pool.Exec(ctx, "DELETE FROM indexer_cursors")
		pool.Exec(ctx, "DELETE FROM message_metadata")
		pool.Exec(ctx, "DELETE FROM message_payloads")
		pool.Exec(ctx, "DELETE FROM message_destinations")
		pool.Exec(ctx, "DELETE FROM message_sources")
		pool.Exec(ctx, "DELETE FROM messages")
		pool.Close()
	}

	return pool, cleanup
}

func runProcessorUntilCursor(
	t *testing.T,
	chainID uint64,
	dec decoder.Decoder,
	client chainReader,
	cursors CursorStore,
	hooks ProcessorHooks,
	wantCursor uint64,
) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- processChainInternal(
			ctx,
			chainID,
			dec.Protocol(),
			dec,
			client,
			cursors,
			100,
			5*time.Millisecond,
			zerolog.Nop(),
			hooks,
		)
	}()

	require.Eventually(t, func() bool {
		cursor, err := cursors.LoadCursor(context.Background(), chainID, dec.Protocol())
		return err == nil && cursor == wantCursor
	}, time.Second, 10*time.Millisecond)

	cancel()
	require.ErrorIs(t, <-errCh, context.Canceled)
}

func readArchivedEvents(t *testing.T, store *memoryArchiveStore, key string) []map[string]any {
	t.Helper()

	data, err := store.DownloadBytes(context.Background(), key)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	events := make([]map[string]any, 0, len(lines))

	for _, line := range lines {
		var event map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &event))
		events = append(events, event)
	}

	return events
}

func buildPacketSentLog(t *testing.T, chainID uint64, blockNumber uint64, logIndex uint, txHashSuffix string) ethtypes.Log {
	t.Helper()

	parsedABI := parseLayerZeroTestABI(t)
	event := parsedABI.Events[decoder.EventPacketSent]
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	payload := makeEncodedPayload()

	data, err := event.Inputs.Pack(payload, []byte("options"), sender)
	require.NoError(t, err)

	return ethtypes.Log{
		Address:     layerzero.EndpointAddresses[chainID],
		BlockNumber: blockNumber,
		TxHash:      common.HexToHash(txHashSuffix),
		Index:       logIndex,
		Topics:      []common.Hash{event.ID},
		Data:        data,
	}
}

func buildPacketReceivedLog(t *testing.T, chainID uint64, blockNumber uint64, logIndex uint, txHashSuffix string) ethtypes.Log {
	t.Helper()

	parsedABI := parseLayerZeroTestABI(t)
	event := parsedABI.Events[decoder.EventPacketReceived]
	receiver := common.HexToAddress("0x2222222222222222222222222222222222222222")
	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")

	var senderBytes [32]byte
	copy(senderBytes[:], common.LeftPadBytes(sender.Bytes(), 32))

	origin := struct {
		SrcEid uint32
		Sender [32]byte
		Nonce  uint64
	}{
		SrcEid: 30101,
		Sender: senderBytes,
		Nonce:  123,
	}

	data, err := event.Inputs.Pack(origin, receiver)
	require.NoError(t, err)

	return ethtypes.Log{
		Address:     layerzero.EndpointAddresses[chainID],
		BlockNumber: blockNumber,
		TxHash:      common.HexToHash(txHashSuffix),
		Index:       logIndex,
		Topics:      []common.Hash{event.ID},
		Data:        data,
	}
}

func parseLayerZeroTestABI(t *testing.T) abi.ABI {
	t.Helper()

	parsedABI, err := abi.JSON(strings.NewReader(layerZeroTestABI))
	require.NoError(t, err)
	return parsedABI
}

func makeEncodedPayload() []byte {
	payload := make([]byte, 113)
	payload[0] = 1
	new(big.Int).SetUint64(123).FillBytes(payload[1:9])
	new(big.Int).SetUint64(30101).FillBytes(payload[9:13])

	sender := common.HexToAddress("0x1111111111111111111111111111111111111111")
	copy(payload[13:45], common.LeftPadBytes(sender.Bytes(), 32))

	new(big.Int).SetUint64(30102).FillBytes(payload[45:49])

	receiver := common.HexToAddress("0x2222222222222222222222222222222222222222")
	copy(payload[49:81], common.LeftPadBytes(receiver.Bytes(), 32))

	guid := common.HexToHash("0x3333333333333333333333333333333333333333333333333333333333333333")
	copy(payload[81:113], guid.Bytes())

	return payload
}
