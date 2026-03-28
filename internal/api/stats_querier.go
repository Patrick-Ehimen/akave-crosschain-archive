package api

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// ─── Types ────────────────────────────────────────────────────────────────

// StatusBreakdown holds per-status message counts.
type StatusBreakdown struct {
	Executed int64 `json:"executed"`
	Pending  int64 `json:"pending"`
	Failed   int64 `json:"failed"`
	Total    int64 `json:"total"`
}

// SuccessRate returns executed / (executed + failed).
// Returns 0 when no terminal messages exist (avoids divide-by-zero).
func (s StatusBreakdown) SuccessRate() float64 {
	terminal := s.Executed + s.Failed
	if terminal == 0 {
		return 0
	}
	return float64(s.Executed) / float64(terminal)
}

// ProtocolStats holds aggregate metrics for a single protocol.
type ProtocolStats struct {
	Protocol          string          `json:"protocol"`
	Counts            StatusBreakdown `json:"counts"`
	SuccessRate       *float64        `json:"success_rate"`
	AvgLatencySeconds *float64        `json:"avg_latency_seconds"`
	P50LatencySeconds *float64        `json:"p50_latency_seconds,omitempty"`
	P95LatencySeconds *float64        `json:"p95_latency_seconds,omitempty"`
	WindowStart       *time.Time      `json:"window_start,omitempty"`
	WindowEnd         *time.Time      `json:"window_end,omitempty"`
	ComputedAt        time.Time       `json:"computed_at"`
}

// RouteStats holds aggregate metrics for a src→dst chain pair.
type RouteStats struct {
	SrcChainID        uint64          `json:"src_chain_id"`
	DstChainID        uint64          `json:"dst_chain_id"`
	Protocol          string          `json:"protocol,omitempty"`
	Counts            StatusBreakdown `json:"counts"`
	SuccessRate       *float64        `json:"success_rate"`
	AvgLatencySeconds *float64        `json:"avg_latency_seconds"`
}

// RoutesStatsResult is the top-N routes response.
type RoutesStatsResult struct {
	Routes      []RouteStats `json:"routes"`
	TotalRoutes int          `json:"total_routes"`
	ComputedAt  time.Time    `json:"computed_at"`
}

// StatsFilter holds optional filters for stats queries.
type StatsFilter struct {
	FromTS   *int64
	ToTS     *int64
	Protocol string
	Limit    int
}

// ─── Interface ────────────────────────────────────────────────────────────

// StatsQuerier computes aggregate metrics from the message store.
type StatsQuerier interface {
	GetProtocolStats(ctx context.Context, protocol string, filter StatsFilter) (*ProtocolStats, error)
	GetRoutesStats(ctx context.Context, filter StatsFilter) (*RoutesStatsResult, error)
}

// ─── In-memory TTL cache ──────────────────────────────────────────────────

type cacheEntry struct {
	value     interface{}
	expiresAt time.Time
}

func (e *cacheEntry) valid() bool {
	return time.Now().Before(e.expiresAt)
}

// statsCache is a lightweight in-memory TTL cache for aggregate results.
// A 60-second TTL keeps aggregate query load low while providing acceptable
// freshness for analytics use-cases.
type statsCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

func newStatsCache(ttl time.Duration) *statsCache {
	return &statsCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

func (c *statsCache) get(key string) (interface{}, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || !e.valid() {
		return nil, false
	}
	return e.value, true
}

func (c *statsCache) set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// evict removes all expired entries. Called lazily after each set to keep
// memory bounded without a background goroutine.
func (c *statsCache) evict() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

// ─── Cache key ────────────────────────────────────────────────────────────

// cacheKey builds a deterministic string key for a (prefix, protocol, filter) triple.
func cacheKey(prefix, protocol string, filter StatsFilter) string {
	from := int64(0)
	if filter.FromTS != nil {
		from = *filter.FromTS
	}
	to := int64(0)
	if filter.ToTS != nil {
		to = *filter.ToTS
	}
	return fmt.Sprintf("%s:%s:%s:%d:%d:%d", prefix, protocol, filter.Protocol, from, to, filter.Limit)
}

// ─── PgStatsQuerier ───────────────────────────────────────────────────────

// PgStatsQuerier implements StatsQuerier using PostgreSQL aggregate queries.
type PgStatsQuerier struct {
	pool  *pgxpool.Pool
	cache *statsCache
}

// NewPgStatsQuerier returns a PgStatsQuerier with the default 60-second cache TTL.
func NewPgStatsQuerier(pool *pgxpool.Pool) *PgStatsQuerier {
	return &PgStatsQuerier{
		pool:  pool,
		cache: newStatsCache(60 * time.Second),
	}
}

// NewPgStatsQuerierWithTTL returns a PgStatsQuerier with a custom cache TTL.
// Pass ttl=0 to disable caching (useful in integration tests).
func NewPgStatsQuerierWithTTL(pool *pgxpool.Pool, ttl time.Duration) *PgStatsQuerier {
	return &PgStatsQuerier{
		pool:  pool,
		cache: newStatsCache(ttl),
	}
}

// GetProtocolStats returns aggregate metrics for a single named protocol.
//
// Two queries run per call (both served from cache on subsequent requests):
//
//  1. Status breakdown: COUNT grouped by status, filtered by protocol and
//     optional time window.  Hits idx_messages_protocol_status.
//
//  2. Latency percentiles: AVG + PERCENTILE_CONT(0.5) + PERCENTILE_CONT(0.95)
//     over message_metadata.latency_seconds for executed messages only.
func (q *PgStatsQuerier) GetProtocolStats(ctx context.Context, protocol string, filter StatsFilter) (*ProtocolStats, error) {
	if protocol == "" {
		return nil, fmt.Errorf("protocol is required")
	}

	key := cacheKey("proto", protocol, filter)
	if v, ok := q.cache.get(key); ok {
		return v.(*ProtocolStats), nil
	}

	// ── Build shared arg list ─────────────────────────────────────────────
	// $1 = protocol always.  $2/$3 = optional timestamps.
	args := []interface{}{protocol}
	var tsWhere strings.Builder
	argIdx := 2

	if filter.FromTS != nil {
		tsWhere.WriteString(fmt.Sprintf(" AND s.timestamp >= $%d", argIdx))
		args = append(args, *filter.FromTS)
		argIdx++
	}
	if filter.ToTS != nil {
		tsWhere.WriteString(fmt.Sprintf(" AND s.timestamp <= $%d", argIdx))
		args = append(args, *filter.ToTS)
		argIdx++
	}

	tsJoin := ""
	if filter.FromTS != nil || filter.ToTS != nil {
		tsJoin = "JOIN message_sources s ON m.message_id = s.message_id"
	}

	// ── 1. Status counts ──────────────────────────────────────────────────
	statusSQL := fmt.Sprintf(`
		SELECT m.status, COUNT(*) AS cnt
		FROM   messages m
		%s
		WHERE  m.protocol = $1
		%s
		GROUP  BY m.status`,
		tsJoin, tsWhere.String(),
	)

	rows, err := q.pool.Query(ctx, statusSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("querying protocol status counts: %w", err)
	}
	var counts StatusBreakdown
	for rows.Next() {
		var status string
		var cnt int64
		if err := rows.Scan(&status, &cnt); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scanning status row: %w", err)
		}
		switch types.MessageStatus(status) {
		case types.StatusExecuted:
			counts.Executed = cnt
		case types.StatusPending:
			counts.Pending = cnt
		case types.StatusFailed:
			counts.Failed = cnt
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating status rows: %w", err)
	}
	counts.Total = counts.Executed + counts.Pending + counts.Failed

	// ── 2. Latency percentiles ────────────────────────────────────────────
	// Build a separate args slice for the latency query so the status
	// filter uses a parameterized placeholder instead of interpolation.
	latencyArgs := make([]interface{}, len(args))
	copy(latencyArgs, args)
	latencyArgs = append(latencyArgs, string(types.StatusExecuted))
	statusArgIdx := len(latencyArgs)

	latencySQL := fmt.Sprintf(`
		SELECT
			AVG(md.latency_seconds)::float8,
			PERCENTILE_CONT(0.5)  WITHIN GROUP (ORDER BY md.latency_seconds)::float8,
			PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY md.latency_seconds)::float8
		FROM   message_metadata md
		JOIN   messages m ON md.message_id = m.message_id
		%s
		WHERE  m.protocol  = $1
		  AND  m.status    = $%d
		  AND  md.latency_seconds > 0
		%s`,
		tsJoin,
		statusArgIdx,
		tsWhere.String(),
	)

	var avgLat, p50Lat, p95Lat *float64
	if err := q.pool.QueryRow(ctx, latencySQL, latencyArgs...).
		Scan(&avgLat, &p50Lat, &p95Lat); err != nil {
		return nil, fmt.Errorf("querying protocol latency: %w", err)
	}

	// ── 3. Assemble ───────────────────────────────────────────────────────
	stats := &ProtocolStats{
		Protocol:          protocol,
		Counts:            counts,
		AvgLatencySeconds: avgLat,
		P50LatencySeconds: p50Lat,
		P95LatencySeconds: p95Lat,
		ComputedAt:        time.Now().UTC(),
	}
	if counts.Executed+counts.Failed > 0 {
		sr := counts.SuccessRate()
		stats.SuccessRate = &sr
	}
	if filter.FromTS != nil {
		t := time.Unix(*filter.FromTS, 0).UTC()
		stats.WindowStart = &t
	}
	if filter.ToTS != nil {
		t := time.Unix(*filter.ToTS, 0).UTC()
		stats.WindowEnd = &t
	}

	q.cache.set(key, stats)
	q.cache.evict()
	return stats, nil
}

// GetRoutesStats returns the top N source→destination chain pairs by volume.
//
// A single query groups by (src_chain_id, dst_chain_id) using conditional
// aggregation (SUM CASE WHEN) so status counts, total, and avg latency all
// come from one table scan.  ORDER BY total DESC LIMIT N.
func (q *PgStatsQuerier) GetRoutesStats(ctx context.Context, filter StatsFilter) (*RoutesStatsResult, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	key := cacheKey("routes", "", filter)
	if v, ok := q.cache.get(key); ok {
		return v.(*RoutesStatsResult), nil
	}

	var where []string
	var args []interface{}
	argIdx := 1

	if filter.Protocol != "" {
		where = append(where, fmt.Sprintf("m.protocol = $%d", argIdx))
		args = append(args, filter.Protocol)
		argIdx++
	}
	if filter.FromTS != nil {
		where = append(where, fmt.Sprintf("s.timestamp >= $%d", argIdx))
		args = append(args, *filter.FromTS)
		argIdx++
	}
	if filter.ToTS != nil {
		where = append(where, fmt.Sprintf("s.timestamp <= $%d", argIdx))
		args = append(args, *filter.ToTS)
		argIdx++
	}
	// Only include messages that have been correlated (destination known).
	where = append(where, "d.chain_id IS NOT NULL")

	whereClause := ""
	if len(where) > 0 {
		whereClause = "WHERE " + strings.Join(where, " AND ")
	}

	args = append(args, limit)

	routesSQL := fmt.Sprintf(`
		SELECT
			s.chain_id                                                   AS src_chain_id,
			d.chain_id                                                   AS dst_chain_id,
			COUNT(*)                                                     AS total,
			SUM(CASE WHEN m.status = 'executed' THEN 1 ELSE 0 END)      AS executed,
			SUM(CASE WHEN m.status = 'pending'  THEN 1 ELSE 0 END)      AS pending,
			SUM(CASE WHEN m.status = 'failed'   THEN 1 ELSE 0 END)      AS failed,
			AVG(CASE WHEN md.latency_seconds > 0 THEN md.latency_seconds::float8 END) AS avg_latency
		FROM      messages           m
		JOIN      message_sources    s  ON m.message_id = s.message_id
		JOIN      message_destinations d ON m.message_id = d.message_id
		LEFT JOIN message_metadata   md ON m.message_id = md.message_id
		%s
		GROUP BY  s.chain_id, d.chain_id
		ORDER BY  total DESC
		LIMIT $%d`,
		whereClause, argIdx,
	)

	rows, err := q.pool.Query(ctx, routesSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("querying routes stats: %w", err)
	}
	defer rows.Close()

	var routes []RouteStats
	for rows.Next() {
		var r RouteStats
		var executed, pending, failed int64
		var avgLat *float64

		if err := rows.Scan(
			&r.SrcChainID, &r.DstChainID,
			&r.Counts.Total,
			&executed, &pending, &failed,
			&avgLat,
		); err != nil {
			return nil, fmt.Errorf("scanning route row: %w", err)
		}
		r.Counts.Executed = executed
		r.Counts.Pending = pending
		r.Counts.Failed = failed
		r.AvgLatencySeconds = avgLat
		r.Protocol = filter.Protocol

		if executed+failed > 0 {
			sr := r.Counts.SuccessRate()
			r.SuccessRate = &sr
		}
		routes = append(routes, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating route rows: %w", err)
	}

	result := &RoutesStatsResult{
		Routes:      routes,
		TotalRoutes: len(routes),
		ComputedAt:  time.Now().UTC(),
	}

	q.cache.set(key, result)
	q.cache.evict()
	return result, nil
}
