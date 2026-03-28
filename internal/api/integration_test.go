//go:build integration

package api

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/testhelper"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── Seed data ───────────────────────────────────────────────────────────────

// seedMessage defines a single test message with all its related rows.
type seedMessage struct {
	ID       string
	Protocol string
	Type     types.MessageType
	Status   types.MessageStatus

	SrcChainID     uint64
	SrcTxHash      string
	SrcBlockNumber uint64
	SrcTimestamp   int64
	SrcSender      string
	SrcLogIndex    int

	// Destination fields — only populated for executed/failed messages.
	HasDst         bool
	DstChainID     uint64
	DstTxHash      string
	DstBlockNumber uint64
	DstTimestamp   int64
	DstReceiver    string
	DstLogIndex    int

	// Payload
	Token  string
	Amount string

	// Metadata
	Fee            string
	LatencySeconds int64
}

var seeds = []seedMessage{
	{
		ID: "lz-msg-001", Protocol: "layerzero_v2",
		Type: types.TypeTokenTransfer, Status: types.StatusExecuted,
		SrcChainID: 1, SrcTxHash: "0xlz01src", SrcBlockNumber: 1000, SrcTimestamp: 1700000000, SrcSender: "0xalice", SrcLogIndex: 0,
		HasDst: true, DstChainID: 42161, DstTxHash: "0xlz01dst", DstBlockNumber: 5000, DstTimestamp: 1700000045, DstReceiver: "0xbob", DstLogIndex: 1,
		Token: "0xusdc", Amount: "1000000", Fee: "5000", LatencySeconds: 45,
	},
	{
		ID: "lz-msg-002", Protocol: "layerzero_v2",
		Type: types.TypeMessage, Status: types.StatusPending,
		SrcChainID: 1, SrcTxHash: "0xlz02src", SrcBlockNumber: 1001, SrcTimestamp: 1700000100, SrcSender: "0xalice", SrcLogIndex: 0,
	},
	{
		ID: "wh-msg-001", Protocol: "wormhole",
		Type: types.TypeTokenTransfer, Status: types.StatusExecuted,
		SrcChainID: 1, SrcTxHash: "0xwh01src", SrcBlockNumber: 1002, SrcTimestamp: 1700000200, SrcSender: "0xcharlie", SrcLogIndex: 0,
		HasDst: true, DstChainID: 56, DstTxHash: "0xwh01dst", DstBlockNumber: 8000, DstTimestamp: 1700000260, DstReceiver: "0xdave", DstLogIndex: 2,
		Token: "0xweth", Amount: "500000", Fee: "3000", LatencySeconds: 60,
	},
	{
		ID: "wh-msg-002", Protocol: "wormhole",
		Type: types.TypeMessage, Status: types.StatusFailed,
		SrcChainID: 43114, SrcTxHash: "0xwh02src", SrcBlockNumber: 2000, SrcTimestamp: 1700000300, SrcSender: "0xeve", SrcLogIndex: 0,
		HasDst: true, DstChainID: 1, DstTxHash: "0xwh02dst", DstBlockNumber: 1010, DstTimestamp: 1700000400, DstReceiver: "0xfrank", DstLogIndex: 0,
		Fee: "1000", LatencySeconds: 100,
	},
	{
		ID: "ax-msg-001", Protocol: "axelar",
		Type: types.TypeContractCall, Status: types.StatusExecuted,
		SrcChainID: 1, SrcTxHash: "0xax01src", SrcBlockNumber: 1003, SrcTimestamp: 1700000400, SrcSender: "0xalice", SrcLogIndex: 0,
		HasDst: true, DstChainID: 137, DstTxHash: "0xax01dst", DstBlockNumber: 9000, DstTimestamp: 1700000430, DstReceiver: "0xgrace", DstLogIndex: 3,
		Token: "0xlink", Amount: "200000", Fee: "4000", LatencySeconds: 30,
	},
	{
		ID: "ax-msg-002", Protocol: "axelar",
		Type: types.TypeTokenTransfer, Status: types.StatusPending,
		SrcChainID: 137, SrcTxHash: "0xax02src", SrcBlockNumber: 9001, SrcTimestamp: 1700000500, SrcSender: "0xgrace", SrcLogIndex: 0,
	},
	{
		ID: "ccip-msg-001", Protocol: "ccip",
		Type: types.TypeTokenTransfer, Status: types.StatusExecuted,
		SrcChainID: 1, SrcTxHash: "0xcc01src", SrcBlockNumber: 1004, SrcTimestamp: 1700000600, SrcSender: "0xalice", SrcLogIndex: 0,
		HasDst: true, DstChainID: 42161, DstTxHash: "0xcc01dst", DstBlockNumber: 5500, DstTimestamp: 1700000650, DstReceiver: "0xbob", DstLogIndex: 4,
		Token: "0xusdt", Amount: "750000", Fee: "6000", LatencySeconds: 50,
	},
	{
		ID: "ccip-msg-002", Protocol: "ccip",
		Type: types.TypeMessage, Status: types.StatusFailed,
		SrcChainID: 10, SrcTxHash: "0xcc02src", SrcBlockNumber: 3000, SrcTimestamp: 1700000700, SrcSender: "0xhenry", SrcLogIndex: 0,
		HasDst: true, DstChainID: 1, DstTxHash: "0xcc02dst", DstBlockNumber: 1020, DstTimestamp: 1700000780, DstReceiver: "0xirene", DstLogIndex: 0,
		Fee: "2000", LatencySeconds: 80,
	},
}

// insertSeeds truncates the message tables and inserts all seed rows.
func insertSeeds(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	// Truncate in correct FK order.
	_, err := pool.Exec(ctx, `
		TRUNCATE message_metadata, message_payloads, message_destinations, message_sources, messages CASCADE
	`)
	require.NoError(t, err, "truncating tables")

	for _, s := range seeds {
		_, err := pool.Exec(ctx,
			`INSERT INTO messages (message_id, protocol, type, status) VALUES ($1, $2, $3, $4)`,
			s.ID, s.Protocol, string(s.Type), string(s.Status))
		require.NoError(t, err, "inserting message %s", s.ID)

		_, err = pool.Exec(ctx,
			`INSERT INTO message_sources (message_id, chain_id, tx_hash, block_number, timestamp, sender, log_index)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			s.ID, s.SrcChainID, s.SrcTxHash, s.SrcBlockNumber, s.SrcTimestamp, s.SrcSender, s.SrcLogIndex)
		require.NoError(t, err, "inserting source for %s", s.ID)

		if s.HasDst {
			_, err = pool.Exec(ctx,
				`INSERT INTO message_destinations (message_id, chain_id, tx_hash, block_number, timestamp, receiver, log_index)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				s.ID, s.DstChainID, s.DstTxHash, s.DstBlockNumber, s.DstTimestamp, s.DstReceiver, s.DstLogIndex)
			require.NoError(t, err, "inserting destination for %s", s.ID)
		}

		if s.Token != "" || s.Amount != "" {
			_, err = pool.Exec(ctx,
				`INSERT INTO message_payloads (message_id, token, amount) VALUES ($1, $2, $3)`,
				s.ID, s.Token, s.Amount)
			require.NoError(t, err, "inserting payload for %s", s.ID)
		}

		if s.Fee != "" || s.LatencySeconds > 0 {
			_, err = pool.Exec(ctx,
				`INSERT INTO message_metadata (message_id, fee, latency_seconds) VALUES ($1, $2, $3)`,
				s.ID, s.Fee, s.LatencySeconds)
			require.NoError(t, err, "inserting metadata for %s", s.ID)
		}
	}
}

// ─── Test setup ──────────────────────────────────────────────────────────────

func setupQueriers(t *testing.T) (*PgMessageQuerier, *PgStatsQuerier, *pgxpool.Pool) {
	t.Helper()
	pool := testhelper.NewTestPool(t)
	insertSeeds(t, pool)
	mq := NewPgMessageQuerier(pool)
	sq := NewPgStatsQuerierWithTTL(pool, 0) // disable cache
	return mq, sq, pool
}

// ─── Health ──────────────────────────────────────────────────────────────────

func TestIntegration_Health(t *testing.T) {
	_, _, pool := setupQueriers(t)
	ctx := context.Background()

	// Verify the DB connection works (same check the health handler performs).
	err := pool.Ping(ctx)
	require.NoError(t, err)

	// Verify cursor querier works against the real DB.
	cq := NewPgCursorQuerier(pool)
	cursors, err := cq.QueryCursors(ctx)
	require.NoError(t, err)
	// No indexer cursors seeded, so expect empty (but no error).
	assert.NotNil(t, cursors)
}

// ─── GetByID ─────────────────────────────────────────────────────────────────

func TestIntegration_GetByID_Found(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	msg, err := mq.GetByID(ctx, "lz-msg-001")
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, "lz-msg-001", msg.MessageID)
	assert.Equal(t, "layerzero_v2", msg.Protocol)
	assert.Equal(t, types.StatusExecuted, msg.Status)
	assert.Equal(t, types.TypeTokenTransfer, msg.Type)
	assert.Equal(t, uint64(1), msg.Source.ChainID)
	assert.Equal(t, "0xalice", msg.Source.Sender)
	require.NotNil(t, msg.Destination)
	assert.Equal(t, uint64(42161), msg.Destination.ChainID)
	assert.Equal(t, "0xbob", msg.Destination.Receiver)
	require.NotNil(t, msg.Payload)
	assert.Equal(t, "0xusdc", msg.Payload.Token)
	require.NotNil(t, msg.Metadata)
	assert.Equal(t, int64(45), msg.Metadata.LatencySeconds)
}

func TestIntegration_GetByID_NotFound(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	msg, err := mq.GetByID(ctx, "nonexistent-id")
	require.NoError(t, err)
	assert.Nil(t, msg)
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestIntegration_List_NoFilter(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := mq.List(ctx, MessageFilter{Limit: 100})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 8)
}

func TestIntegration_List_FilterByProtocol(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := mq.List(ctx, MessageFilter{Protocol: "layerzero_v2", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 2)
	for _, m := range result.Messages {
		assert.Equal(t, "layerzero_v2", m.Protocol)
	}
}

func TestIntegration_List_FilterByStatus(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := mq.List(ctx, MessageFilter{Status: "executed", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 4)
	for _, m := range result.Messages {
		assert.Equal(t, types.StatusExecuted, m.Status)
	}
}

func TestIntegration_List_FilterBySrcChain(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	chain1 := uint64(1)
	result, err := mq.List(ctx, MessageFilter{SrcChain: &chain1, Limit: 100})
	require.NoError(t, err)
	// Seeds with src_chain=1: lz-001, lz-002, wh-001, ax-001, ccip-001
	assert.Len(t, result.Messages, 5)
	for _, m := range result.Messages {
		assert.Equal(t, uint64(1), m.Source.ChainID)
	}
}

func TestIntegration_List_Pagination(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	// First page of 3.
	r1, err := mq.List(ctx, MessageFilter{Limit: 3, SortOrder: "desc"})
	require.NoError(t, err)
	assert.Len(t, r1.Messages, 3)
	assert.NotEmpty(t, r1.NextCursor, "should have next cursor")

	// Second page using cursor.
	r2, err := mq.List(ctx, MessageFilter{Limit: 3, Cursor: r1.NextCursor, SortOrder: "desc"})
	require.NoError(t, err)
	assert.Len(t, r2.Messages, 3)

	// Verify no overlap.
	seen := map[string]bool{}
	for _, m := range r1.Messages {
		seen[m.MessageID] = true
	}
	for _, m := range r2.Messages {
		assert.False(t, seen[m.MessageID], "pages should not overlap: %s", m.MessageID)
	}
}

func TestIntegration_List_EmptyResult(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := mq.List(ctx, MessageFilter{Protocol: "nonexistent_protocol", Limit: 20})
	require.NoError(t, err)
	assert.Empty(t, result.Messages)
}

// ─── GetByTxHash ─────────────────────────────────────────────────────────────

func TestIntegration_GetByTxHash_Found(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	msgs, err := mq.GetByTxHash(ctx, "0xlz01src")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "lz-msg-001", msgs[0].MessageID)
}

func TestIntegration_GetByTxHash_FoundByDstHash(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	msgs, err := mq.GetByTxHash(ctx, "0xlz01dst")
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "lz-msg-001", msgs[0].MessageID)
}

func TestIntegration_GetByTxHash_NotFound(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	msgs, err := mq.GetByTxHash(ctx, "0xnonexistent")
	require.NoError(t, err)
	assert.Empty(t, msgs)
}

// ─── Address History ─────────────────────────────────────────────────────────

func TestIntegration_AddressHistory_Sender(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	// 0xalice is sender on lz-001, lz-002, ax-001, ccip-001
	result, err := mq.GetAddressHistory(ctx, "0xalice", AddressHistoryFilter{Limit: 100})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 4)
}

func TestIntegration_AddressHistory_Receiver(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	// 0xbob is receiver on lz-001 and ccip-001
	result, err := mq.GetAddressHistory(ctx, "0xbob", AddressHistoryFilter{Limit: 100})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 2)
}

func TestIntegration_AddressHistory_FilterByProtocol(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := mq.GetAddressHistory(ctx, "0xalice", AddressHistoryFilter{
		Protocol: "layerzero_v2",
		Limit:    100,
	})
	require.NoError(t, err)
	assert.Len(t, result.Messages, 2)
	for _, m := range result.Messages {
		assert.Equal(t, "layerzero_v2", m.Protocol)
	}
}

// ─── Trace ───────────────────────────────────────────────────────────────────

func TestIntegration_Trace_Executed(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	trace, err := mq.GetTrace(ctx, "lz-msg-001")
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Equal(t, "lz-msg-001", trace.MessageID)
	assert.Equal(t, types.StatusExecuted, trace.Status)
	assert.Len(t, trace.Events, 2)
	assert.Equal(t, "sent", trace.Events[0].Type)
	assert.Equal(t, "received", trace.Events[1].Type)
	assert.Equal(t, uint64(1), trace.Events[0].ChainID)
	assert.Equal(t, uint64(42161), trace.Events[1].ChainID)
	require.NotNil(t, trace.LatencySeconds)
	assert.Equal(t, int64(45), *trace.LatencySeconds)
}

func TestIntegration_Trace_Pending(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	trace, err := mq.GetTrace(ctx, "lz-msg-002")
	require.NoError(t, err)
	require.NotNil(t, trace)
	assert.Equal(t, types.StatusPending, trace.Status)
	assert.Len(t, trace.Events, 1)
	assert.Equal(t, "sent", trace.Events[0].Type)
	assert.Nil(t, trace.LatencySeconds)
}

func TestIntegration_Trace_NotFound(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	trace, err := mq.GetTrace(ctx, "nonexistent-id")
	require.NoError(t, err)
	assert.Nil(t, trace)
}

// ─── Protocol Stats ──────────────────────────────────────────────────────────

func TestIntegration_ProtocolStats(t *testing.T) {
	_, sq, _ := setupQueriers(t)
	ctx := context.Background()

	stats, err := sq.GetProtocolStats(ctx, "layerzero_v2", StatsFilter{})
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, "layerzero_v2", stats.Protocol)
	assert.Equal(t, int64(1), stats.Counts.Executed)
	assert.Equal(t, int64(1), stats.Counts.Pending)
	assert.Equal(t, int64(0), stats.Counts.Failed)
	assert.Equal(t, int64(2), stats.Counts.Total)
	require.NotNil(t, stats.SuccessRate)
	assert.Equal(t, 1.0, *stats.SuccessRate) // 1 executed, 0 failed
	require.NotNil(t, stats.AvgLatencySeconds)
	assert.Equal(t, float64(45), *stats.AvgLatencySeconds)
}

func TestIntegration_ProtocolStats_AllProtocols(t *testing.T) {
	_, sq, _ := setupQueriers(t)
	ctx := context.Background()

	for _, proto := range []string{"layerzero_v2", "wormhole", "axelar", "ccip"} {
		stats, err := sq.GetProtocolStats(ctx, proto, StatsFilter{})
		require.NoError(t, err, "protocol: %s", proto)
		require.NotNil(t, stats, "protocol: %s", proto)
		assert.Equal(t, proto, stats.Protocol)
		assert.Equal(t, int64(2), stats.Counts.Total, "protocol %s should have 2 messages", proto)
	}
}

// ─── Routes Stats ────────────────────────────────────────────────────────────

func TestIntegration_RoutesStats(t *testing.T) {
	_, sq, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := sq.GetRoutesStats(ctx, StatsFilter{Limit: 20})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Only executed/failed messages with destinations form routes.
	// Routes: (1→42161), (1→56), (43114→1), (1→137), (10→1) = 5 routes
	assert.GreaterOrEqual(t, len(result.Routes), 1)
	assert.Equal(t, len(result.Routes), result.TotalRoutes)

	// The top route by volume should be 1→42161 (lz-001 + ccip-001 = 2 messages).
	if len(result.Routes) > 0 {
		top := result.Routes[0]
		assert.Equal(t, uint64(1), top.SrcChainID)
		assert.Equal(t, uint64(42161), top.DstChainID)
		assert.Equal(t, int64(2), top.Counts.Total)
	}
}

func TestIntegration_RoutesStats_FilterByProtocol(t *testing.T) {
	_, sq, _ := setupQueriers(t)
	ctx := context.Background()

	result, err := sq.GetRoutesStats(ctx, StatsFilter{Protocol: "wormhole", Limit: 20})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Wormhole has 2 routes: (1→56) and (43114→1)
	assert.Len(t, result.Routes, 2)
}

// ─── Response Time ───────────────────────────────────────────────────────────

func TestIntegration_ResponseTime_SingleLookup(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	start := time.Now()
	msg, err := mq.GetByID(ctx, "lz-msg-001")
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Less(t, elapsed, 500*time.Millisecond,
		fmt.Sprintf("single lookup took %v, expected < 500ms", elapsed))
}

func TestIntegration_ResponseTime_List(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	start := time.Now()
	_, err := mq.List(ctx, MessageFilter{Limit: 20})
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond,
		fmt.Sprintf("list query took %v, expected < 500ms", elapsed))
}

func TestIntegration_ResponseTime_Trace(t *testing.T) {
	mq, _, _ := setupQueriers(t)
	ctx := context.Background()

	start := time.Now()
	_, err := mq.GetTrace(ctx, "lz-msg-001")
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 500*time.Millisecond,
		fmt.Sprintf("trace query took %v, expected < 500ms", elapsed))
}
