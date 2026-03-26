package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type mockPinger struct {
	err error
}

func (m *mockPinger) Ping(_ context.Context) error {
	return m.err
}

type mockCursorQuerier struct {
	cursors []CursorInfo
	err     error
}

func (m *mockCursorQuerier) QueryCursors(_ context.Context) ([]CursorInfo, error) {
	return m.cursors, m.err
}

func TestHealthHandler_DBConnected(t *testing.T) {
	handler := HealthHandler(&mockPinger{err: nil}, &mockCursorQuerier{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Database != "connected" {
		t.Errorf("database = %q, want connected", resp.Database)
	}
	if resp.Error != "" {
		t.Errorf("error = %q, want empty", resp.Error)
	}
	if resp.Indexer == nil {
		t.Error("indexer field should be present when DB is connected")
	}
}

func TestHealthHandler_DBDisconnected(t *testing.T) {
	handler := HealthHandler(
		&mockPinger{err: errors.New("connection refused")},
		&mockCursorQuerier{},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "degraded" {
		t.Errorf("status = %q, want degraded", resp.Status)
	}
	if resp.Database != "disconnected" {
		t.Errorf("database = %q, want disconnected", resp.Database)
	}
	if resp.Error != "connection refused" {
		t.Errorf("error = %q, want 'connection refused'", resp.Error)
	}
}

func TestHealthHandler_WithCursors(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	cursors := []CursorInfo{
		{ChainID: 1, Protocol: "layerzero", LastBlock: 12345, UpdatedAt: now},
		{ChainID: 42161, Protocol: "axelar", LastBlock: 67890, UpdatedAt: now},
	}

	handler := HealthHandler(
		&mockPinger{err: nil},
		&mockCursorQuerier{cursors: cursors},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Indexer == nil {
		t.Fatal("indexer field should be present")
	}
	if len(resp.Indexer.Cursors) != 2 {
		t.Fatalf("cursors len = %d, want 2", len(resp.Indexer.Cursors))
	}
	if resp.Indexer.Cursors[0].ChainID != 1 {
		t.Errorf("cursor[0].chain_id = %d, want 1", resp.Indexer.Cursors[0].ChainID)
	}
	if resp.Indexer.Cursors[0].Protocol != "layerzero" {
		t.Errorf("cursor[0].protocol = %q, want layerzero", resp.Indexer.Cursors[0].Protocol)
	}
	if resp.Indexer.Cursors[0].LastBlock != 12345 {
		t.Errorf("cursor[0].last_block = %d, want 12345", resp.Indexer.Cursors[0].LastBlock)
	}
	if resp.Indexer.Cursors[1].ChainID != 42161 {
		t.Errorf("cursor[1].chain_id = %d, want 42161", resp.Indexer.Cursors[1].ChainID)
	}
}

func TestHealthHandler_CursorQueryFails(t *testing.T) {
	handler := HealthHandler(
		&mockPinger{err: nil},
		&mockCursorQuerier{err: errors.New("query failed")},
	)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (cursor failure is non-fatal)", rec.Code, http.StatusOK)
	}

	var resp healthResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "ok" {
		t.Errorf("status = %q, want ok", resp.Status)
	}
	if resp.Indexer == nil {
		t.Fatal("indexer field should be present even on cursor query failure")
	}
	if resp.Indexer.Cursors != nil {
		t.Errorf("cursors = %v, want nil on query failure", resp.Indexer.Cursors)
	}
}
