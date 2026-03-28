package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// ─── mock StatsQuerier ────────────────────────────────────────────────────

type mockStatsQuerier struct {
	protocolStatsFn func(ctx context.Context, protocol string, filter StatsFilter) (*ProtocolStats, error)
	routesStatsFn   func(ctx context.Context, filter StatsFilter) (*RoutesStatsResult, error)
}

func (m *mockStatsQuerier) GetProtocolStats(ctx context.Context, protocol string, filter StatsFilter) (*ProtocolStats, error) {
	return m.protocolStatsFn(ctx, protocol, filter)
}

func (m *mockStatsQuerier) GetRoutesStats(ctx context.Context, filter StatsFilter) (*RoutesStatsResult, error) {
	return m.routesStatsFn(ctx, filter)
}

// ─── helpers ─────────────────────────────────────────────────────────────

func ptr64(v float64) *float64 { return &v }
func ptrI64(v int64) *int64    { return &v }

func sampleProtocolStats(protocol string) *ProtocolStats {
	sr := 0.9917
	avg := 42.3
	p50 := 38.0
	p95 := 120.5
	return &ProtocolStats{
		Protocol: protocol,
		Counts: StatusBreakdown{
			Executed: 1200,
			Pending:  50,
			Failed:   10,
			Total:    1260,
		},
		SuccessRate:       &sr,
		AvgLatencySeconds: &avg,
		P50LatencySeconds: &p50,
		P95LatencySeconds: &p95,
		ComputedAt:        time.Now().UTC(),
	}
}

func sampleRoutesResult() *RoutesStatsResult {
	sr1 := 0.9939
	avg1 := 38.4
	sr2 := 0.8500
	return &RoutesStatsResult{
		Routes: []RouteStats{
			{
				SrcChainID:        1,
				DstChainID:        42161,
				Counts:            StatusBreakdown{Executed: 800, Pending: 20, Failed: 5, Total: 825},
				SuccessRate:       &sr1,
				AvgLatencySeconds: &avg1,
			},
			{
				SrcChainID:  1,
				DstChainID:  137,
				Counts:      StatusBreakdown{Executed: 170, Pending: 10, Failed: 30, Total: 210},
				SuccessRate: &sr2,
			},
		},
		TotalRoutes: 2,
		ComputedAt:  time.Now().UTC(),
	}
}

func newProtoRequest(protocol string, query string) *http.Request {
	path := "/protocols/" + protocol + "/stats"
	if query != "" {
		path += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("protocol", protocol)
	return req.WithContext(chiCtx(req, rctx))
}

func chiCtx(req *http.Request, rctx *chi.Context) context.Context {
	return context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
}

// ─── GetProtocolStatsHandler tests ───────────────────────────────────────

func TestGetProtocolStatsHandler_LayerZero(t *testing.T) {
	sq := &mockStatsQuerier{
		protocolStatsFn: func(_ context.Context, protocol string, _ StatsFilter) (*ProtocolStats, error) {
			return sampleProtocolStats(protocol), nil
		},
	}

	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("layerzero_v2", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp ProtocolStats
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Protocol != "layerzero_v2" {
		t.Errorf("protocol = %q, want layerzero_v2", resp.Protocol)
	}
	if resp.Counts.Total != 1260 {
		t.Errorf("counts.total = %d, want 1260", resp.Counts.Total)
	}
	if resp.SuccessRate == nil {
		t.Fatal("success_rate should not be nil")
	}
	if resp.AvgLatencySeconds == nil {
		t.Fatal("avg_latency_seconds should not be nil")
	}
}

func TestGetProtocolStatsHandler_AllKnownProtocols(t *testing.T) {
	protocols := []string{"layerzero_v2", "wormhole", "axelar", "ccip"}
	for _, proto := range protocols {
		t.Run(proto, func(t *testing.T) {
			sq := &mockStatsQuerier{
				protocolStatsFn: func(_ context.Context, p string, _ StatsFilter) (*ProtocolStats, error) {
					return sampleProtocolStats(p), nil
				},
			}
			handler := GetProtocolStatsHandler(sq)
			req := newProtoRequest(proto, "")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("[%s] status = %d, want 200", proto, rec.Code)
			}
		})
	}
}

func TestGetProtocolStatsHandler_UnknownProtocol_404(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("unknown_bridge", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var resp apiError
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error == "" {
		t.Error("error message should not be empty")
	}
}

func TestGetProtocolStatsHandler_MissingProtocol_400(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetProtocolStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/protocols//stats", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("protocol", "")
	req = req.WithContext(chiCtx(req, rctx))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetProtocolStatsHandler_WithTimeRange(t *testing.T) {
	sq := &mockStatsQuerier{
		protocolStatsFn: func(_ context.Context, _ string, f StatsFilter) (*ProtocolStats, error) {
			if f.FromTS == nil || *f.FromTS != 1706000000 {
				t.Errorf("from_ts = %v, want 1706000000", f.FromTS)
			}
			if f.ToTS == nil || *f.ToTS != 1706999999 {
				t.Errorf("to_ts = %v, want 1706999999", f.ToTS)
			}
			return sampleProtocolStats("layerzero_v2"), nil
		},
	}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("layerzero_v2", "from_ts=1706000000&to_ts=1706999999")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetProtocolStatsHandler_InvalidFromTS(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("layerzero_v2", "from_ts=not-a-number")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetProtocolStatsHandler_InvalidToTS(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("layerzero_v2", "to_ts=bad")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetProtocolStatsHandler_FromTSGreaterThanToTS(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("layerzero_v2", "from_ts=1707000000&to_ts=1706000000")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when from_ts > to_ts", rec.Code)
	}
}

func TestGetProtocolStatsHandler_InternalError(t *testing.T) {
	sq := &mockStatsQuerier{
		protocolStatsFn: func(_ context.Context, _ string, _ StatsFilter) (*ProtocolStats, error) {
			return nil, fmt.Errorf("db timeout")
		},
	}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("ccip", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGetProtocolStatsHandler_ZeroCountsAllowed(t *testing.T) {
	// A known protocol with no messages should return 200 with zero counts,
	// not a 404. The 404 is reserved for unknown protocol names.
	sq := &mockStatsQuerier{
		protocolStatsFn: func(_ context.Context, protocol string, _ StatsFilter) (*ProtocolStats, error) {
			return &ProtocolStats{
				Protocol:   protocol,
				Counts:     StatusBreakdown{},
				ComputedAt: time.Now().UTC(),
			}, nil
		},
	}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("axelar", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for known protocol with no data", rec.Code)
	}
	var resp ProtocolStats
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Counts.Total != 0 {
		t.Errorf("counts.total = %d, want 0", resp.Counts.Total)
	}
	if resp.SuccessRate != nil {
		t.Error("success_rate should be nil when no terminal messages exist")
	}
}

func TestGetProtocolStatsHandler_ContentTypeJSON(t *testing.T) {
	sq := &mockStatsQuerier{
		protocolStatsFn: func(_ context.Context, p string, _ StatsFilter) (*ProtocolStats, error) {
			return sampleProtocolStats(p), nil
		},
	}
	handler := GetProtocolStatsHandler(sq)
	req := newProtoRequest("wormhole", "")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

// ─── GetRoutesStatsHandler tests ──────────────────────────────────────────

func TestGetRoutesStatsHandler_Basic(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, _ StatsFilter) (*RoutesStatsResult, error) {
			return sampleRoutesResult(), nil
		},
	}

	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp RoutesStatsResult
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalRoutes != 2 {
		t.Errorf("total_routes = %d, want 2", resp.TotalRoutes)
	}
	if len(resp.Routes) != 2 {
		t.Fatalf("routes len = %d, want 2", len(resp.Routes))
	}
	// First route should be Ethereum → Arbitrum (highest volume)
	if resp.Routes[0].SrcChainID != 1 || resp.Routes[0].DstChainID != 42161 {
		t.Errorf("top route = %d→%d, want 1→42161", resp.Routes[0].SrcChainID, resp.Routes[0].DstChainID)
	}
}

func TestGetRoutesStatsHandler_EmptyResult(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, _ StatsFilter) (*RoutesStatsResult, error) {
			return &RoutesStatsResult{Routes: []RouteStats{}, TotalRoutes: 0, ComputedAt: time.Now()}, nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp RoutesStatsResult
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.TotalRoutes != 0 {
		t.Errorf("total_routes = %d, want 0", resp.TotalRoutes)
	}
}

func TestGetRoutesStatsHandler_WithProtocolFilter(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, f StatsFilter) (*RoutesStatsResult, error) {
			if f.Protocol != "ccip" {
				t.Errorf("protocol = %q, want ccip", f.Protocol)
			}
			return sampleRoutesResult(), nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?protocol=ccip", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetRoutesStatsHandler_InvalidProtocol(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?protocol=invalid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetRoutesStatsHandler_WithTimeRange(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, f StatsFilter) (*RoutesStatsResult, error) {
			if f.FromTS == nil || *f.FromTS != 1706000000 {
				t.Errorf("from_ts = %v, want 1706000000", f.FromTS)
			}
			if f.ToTS == nil || *f.ToTS != 1707000000 {
				t.Errorf("to_ts = %v, want 1707000000", f.ToTS)
			}
			return sampleRoutesResult(), nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?from_ts=1706000000&to_ts=1707000000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetRoutesStatsHandler_WithLimit(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, f StatsFilter) (*RoutesStatsResult, error) {
			if f.Limit != 5 {
				t.Errorf("limit = %d, want 5", f.Limit)
			}
			return sampleRoutesResult(), nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?limit=5", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetRoutesStatsHandler_LimitCappedAt100(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, f StatsFilter) (*RoutesStatsResult, error) {
			if f.Limit != 100 {
				t.Errorf("limit = %d, want 100 (capped)", f.Limit)
			}
			return sampleRoutesResult(), nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?limit=9999", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetRoutesStatsHandler_InvalidLimit(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?limit=bad", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetRoutesStatsHandler_InvalidFromTS(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?from_ts=abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetRoutesStatsHandler_FromTSGreaterThanToTS(t *testing.T) {
	sq := &mockStatsQuerier{}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats?from_ts=9999999999&to_ts=1000000000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when from_ts > to_ts", rec.Code)
	}
}

func TestGetRoutesStatsHandler_InternalError(t *testing.T) {
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, _ StatsFilter) (*RoutesStatsResult, error) {
			return nil, fmt.Errorf("connection pool exhausted")
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGetRoutesStatsHandler_SuccessRateFields(t *testing.T) {
	// Verify success_rate is nil when no terminal messages exist on a route.
	sq := &mockStatsQuerier{
		routesStatsFn: func(_ context.Context, _ StatsFilter) (*RoutesStatsResult, error) {
			return &RoutesStatsResult{
				Routes: []RouteStats{
					{
						SrcChainID: 1,
						DstChainID: 10,
						Counts:     StatusBreakdown{Pending: 5, Total: 5},
						// SuccessRate nil — no terminal messages
					},
				},
				TotalRoutes: 1,
				ComputedAt:  time.Now(),
			}, nil
		},
	}
	handler := GetRoutesStatsHandler(sq)
	req := httptest.NewRequest(http.MethodGet, "/routes/stats", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp RoutesStatsResult
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Routes[0].SuccessRate != nil {
		t.Errorf("success_rate should be nil for all-pending route, got %v", *resp.Routes[0].SuccessRate)
	}
}

// ─── StatusBreakdown unit tests ───────────────────────────────────────────

func TestStatusBreakdown_SuccessRate_Normal(t *testing.T) {
	s := StatusBreakdown{Executed: 90, Failed: 10, Total: 100}
	if got := s.SuccessRate(); got != 0.9 {
		t.Errorf("SuccessRate() = %v, want 0.9", got)
	}
}

func TestStatusBreakdown_SuccessRate_AllExecuted(t *testing.T) {
	s := StatusBreakdown{Executed: 100, Total: 100}
	if got := s.SuccessRate(); got != 1.0 {
		t.Errorf("SuccessRate() = %v, want 1.0", got)
	}
}

func TestStatusBreakdown_SuccessRate_AllFailed(t *testing.T) {
	s := StatusBreakdown{Failed: 50, Total: 50}
	if got := s.SuccessRate(); got != 0.0 {
		t.Errorf("SuccessRate() = %v, want 0.0", got)
	}
}

func TestStatusBreakdown_SuccessRate_NoDivideByZero(t *testing.T) {
	s := StatusBreakdown{Pending: 10, Total: 10}
	if got := s.SuccessRate(); got != 0.0 {
		t.Errorf("SuccessRate() = %v, want 0.0 (no terminal messages)", got)
	}
}

func TestStatusBreakdown_SuccessRate_Empty(t *testing.T) {
	s := StatusBreakdown{}
	if got := s.SuccessRate(); got != 0.0 {
		t.Errorf("SuccessRate() = %v, want 0.0 (zero counts)", got)
	}
}

// ─── Cache unit tests ─────────────────────────────────────────────────────

func TestStatsCache_SetAndGet(t *testing.T) {
	c := newStatsCache(5 * time.Second)
	c.set("key1", "value1")

	v, ok := c.get("key1")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if v.(string) != "value1" {
		t.Errorf("got %v, want value1", v)
	}
}

func TestStatsCache_MissOnUnknownKey(t *testing.T) {
	c := newStatsCache(5 * time.Second)
	_, ok := c.get("nonexistent")
	if ok {
		t.Error("expected cache miss for unknown key")
	}
}

func TestStatsCache_ExpiredEntry(t *testing.T) {
	c := newStatsCache(1 * time.Millisecond)
	c.set("expiring", "value")
	time.Sleep(5 * time.Millisecond)

	_, ok := c.get("expiring")
	if ok {
		t.Error("expected cache miss after TTL expiry")
	}
}

func TestStatsCache_Overwrite(t *testing.T) {
	c := newStatsCache(5 * time.Second)
	c.set("k", "old")
	c.set("k", "new")

	v, ok := c.get("k")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if v.(string) != "new" {
		t.Errorf("got %v, want new", v)
	}
}

func TestStatsCache_Evict(t *testing.T) {
	c := newStatsCache(1 * time.Millisecond)
	c.set("a", 1)
	c.set("b", 2)
	time.Sleep(5 * time.Millisecond)
	c.evict()

	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.entries) != 0 {
		t.Errorf("expected 0 entries after evict, got %d", len(c.entries))
	}
}

func TestStatsCache_ZeroTTL_AlwaysMiss(t *testing.T) {
	// TTL=0 means entries expire immediately — used in integration tests
	// to bypass caching.
	c := newStatsCache(0)
	c.set("k", "v")
	_, ok := c.get("k")
	if ok {
		t.Error("expected miss with TTL=0")
	}
}

// ─── cacheKey uniqueness tests ────────────────────────────────────────────

func TestCacheKey_Uniqueness(t *testing.T) {
	from1 := int64(1706000000)
	to1 := int64(1706999999)
	from2 := int64(1707000000)

	keys := map[string]struct{}{}
	cases := []struct {
		prefix   string
		protocol string
		filter   StatsFilter
	}{
		{"proto", "layerzero_v2", StatsFilter{}},
		{"proto", "wormhole", StatsFilter{}},
		{"proto", "layerzero_v2", StatsFilter{FromTS: &from1}},
		{"proto", "layerzero_v2", StatsFilter{FromTS: &from2}},
		{"proto", "layerzero_v2", StatsFilter{FromTS: &from1, ToTS: &to1}},
		{"routes", "", StatsFilter{}},
		{"routes", "", StatsFilter{Protocol: "ccip"}},
		{"routes", "", StatsFilter{Limit: 10}},
		{"routes", "", StatsFilter{Limit: 50}},
	}

	for _, tc := range cases {
		k := cacheKey(tc.prefix, tc.protocol, tc.filter)
		if _, dup := keys[k]; dup {
			t.Errorf("duplicate cache key: %q", k)
		}
		keys[k] = struct{}{}
	}
}
