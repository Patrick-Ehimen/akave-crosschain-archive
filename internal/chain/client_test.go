package chain

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog"
)

// mockEthClient implements EthClient for testing.
type mockEthClient struct {
	blockNumber    uint64
	blockNumberErr error
	filterLogs     []types.Log
	filterLogsErr  error
	filterCalls    int
	closed         bool
}

func (m *mockEthClient) BlockNumber(_ context.Context) (uint64, error) {
	return m.blockNumber, m.blockNumberErr
}

func (m *mockEthClient) FilterLogs(_ context.Context, _ ethereum.FilterQuery) ([]types.Log, error) {
	m.filterCalls++
	return m.filterLogs, m.filterLogsErr
}

func (m *mockEthClient) Close() {
	m.closed = true
}

func newTestClient(mock *mockEthClient, confirmationDepth, maxBlockRange uint64) *Client {
	log := zerolog.Nop()
	return newClientFromEth(mock, 1, "test", confirmationDepth, maxBlockRange, 1000, log)
}

func TestLatestConfirmedBlock(t *testing.T) {
	tests := []struct {
		name              string
		blockNumber       uint64
		confirmationDepth uint64
		want              uint64
	}{
		{"normal", 1000, 12, 988},
		{"zero depth", 1000, 0, 1000},
		{"depth exceeds block", 5, 12, 0},
		{"depth equals block", 12, 12, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockEthClient{blockNumber: tt.blockNumber}
			c := newTestClient(mock, tt.confirmationDepth, 1000)

			got, err := c.LatestConfirmedBlock(context.Background())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestLatestConfirmedBlock_RPCError(t *testing.T) {
	mock := &mockEthClient{blockNumberErr: fmt.Errorf("rpc down")}
	c := newTestClient(mock, 12, 1000)

	_, err := c.LatestConfirmedBlock(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchLogs_SingleChunk(t *testing.T) {
	expectedLogs := []types.Log{{BlockNumber: 100}, {BlockNumber: 200}}
	mock := &mockEthClient{filterLogs: expectedLogs}
	c := newTestClient(mock, 0, 1000)

	logs, err := c.FetchLogs(context.Background(), 0, 999, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != len(expectedLogs) {
		t.Errorf("got %d logs, want %d", len(logs), len(expectedLogs))
	}
	if mock.filterCalls != 1 {
		t.Errorf("expected 1 FilterLogs call, got %d", mock.filterCalls)
	}
}

func TestFetchLogs_MultipleChunks(t *testing.T) {
	mock := &mockEthClient{filterLogs: []types.Log{{BlockNumber: 1}}}
	c := newTestClient(mock, 0, 100)

	// Range 0-499 with maxBlockRange=100 should produce 5 chunks:
	// [0-99], [100-199], [200-299], [300-399], [400-499]
	logs, err := c.FetchLogs(context.Background(), 0, 499, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.filterCalls != 5 {
		t.Errorf("expected 5 FilterLogs calls, got %d", mock.filterCalls)
	}
	// Each chunk returns 1 log, total should be 5.
	if len(logs) != 5 {
		t.Errorf("got %d logs, want 5", len(logs))
	}
}

func TestFetchLogs_InvalidRange(t *testing.T) {
	mock := &mockEthClient{}
	c := newTestClient(mock, 0, 1000)

	_, err := c.FetchLogs(context.Background(), 500, 100, nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid range, got nil")
	}
}

func TestFetchLogs_WithAddressesAndTopics(t *testing.T) {
	mock := &mockEthClient{filterLogs: []types.Log{{BlockNumber: 50}}}
	c := newTestClient(mock, 0, 1000)

	addrs := []common.Address{common.HexToAddress("0x1234")}
	topics := [][]common.Hash{{common.HexToHash("0xabcd")}}

	logs, err := c.FetchLogs(context.Background(), 0, 100, addrs, topics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("got %d logs, want 1", len(logs))
	}
}

func TestFetchLogs_RPCError(t *testing.T) {
	mock := &mockEthClient{filterLogsErr: fmt.Errorf("rpc error")}
	c := newTestClient(mock, 0, 1000)

	_, err := c.FetchLogs(context.Background(), 0, 100, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchLogs_ContextCancelled(t *testing.T) {
	mock := &mockEthClient{filterLogsErr: fmt.Errorf("rpc error")}
	c := newTestClient(mock, 0, 1000)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.FetchLogs(ctx, 0, 100, nil, nil)
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

func TestClient_Close(t *testing.T) {
	mock := &mockEthClient{}
	c := newTestClient(mock, 0, 1000)

	c.Close()
	if !mock.closed {
		t.Error("expected ethclient to be closed")
	}
}

func TestManagerGetClient_NotFound(t *testing.T) {
	log := zerolog.Nop()
	m := &Manager{
		clients: make(map[uint64]*Client),
		log:     log,
	}

	_, err := m.GetClient(999)
	if err == nil {
		t.Fatal("expected error for unknown chain, got nil")
	}
}

func TestManagerGetClient_Found(t *testing.T) {
	mock := &mockEthClient{}
	c := newTestClient(mock, 0, 1000)
	log := zerolog.Nop()

	m := &Manager{
		clients: map[uint64]*Client{1: c},
		log:     log,
	}

	got, err := m.GetClient(1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != c {
		t.Error("returned client does not match")
	}
}

func TestManagerAllClients(t *testing.T) {
	mock1 := &mockEthClient{}
	mock2 := &mockEthClient{}
	c1 := newTestClient(mock1, 0, 1000)
	c2 := newTestClient(mock2, 0, 1000)
	log := zerolog.Nop()

	m := &Manager{
		clients: map[uint64]*Client{1: c1, 42161: c2},
		log:     log,
	}

	all := m.AllClients()
	if len(all) != 2 {
		t.Errorf("got %d clients, want 2", len(all))
	}
}

func TestManagerClose(t *testing.T) {
	mock1 := &mockEthClient{}
	mock2 := &mockEthClient{}
	c1 := newTestClient(mock1, 0, 1000)
	c2 := newTestClient(mock2, 0, 1000)
	log := zerolog.Nop()

	m := &Manager{
		clients: map[uint64]*Client{1: c1, 42161: c2},
		log:     log,
	}

	m.Close()
	if !mock1.closed || !mock2.closed {
		t.Error("expected all ethclients to be closed")
	}
}
