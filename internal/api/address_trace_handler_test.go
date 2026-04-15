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

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// ─── helpers ────────────────────────────────────────────────────────────────

func sampleExecutedMessage(id string) *types.Message {
	now := time.Now().Truncate(time.Second)
	latency := int64(45)
	return &types.Message{
		MessageID: id,
		Protocol:  "layerzero_v2",
		Type:      types.TypeTokenTransfer,
		Status:    types.StatusExecuted,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xsrc",
			BlockNumber: 19000000,
			Timestamp:   1706000000,
			Sender:      "0xSender1111111111111111111111111111111111",
			LogIndex:    5,
		},
		Destination: &types.Destination{
			ChainID:     42161,
			TxHash:      "0xdst",
			BlockNumber: 180000000,
			Timestamp:   1706000045,
			Receiver:    "0xReceiver2222222222222222222222222222222222",
			LogIndex:    3,
		},
		Payload: &types.Payload{
			Token:  "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48",
			Amount: "1000000000",
			Nonce:  42,
		},
		Metadata: &types.Metadata{
			Fee:            "50000000000000",
			LatencySeconds: latency,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func samplePendingMessage(id string) *types.Message {
	now := time.Now().Truncate(time.Second)
	return &types.Message{
		MessageID: id,
		Protocol:  "wormhole",
		Type:      types.TypeMessage,
		Status:    types.StatusPending,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xwh_src",
			BlockNumber: 19001000,
			Timestamp:   1706001000,
			Sender:      "0xSender1111111111111111111111111111111111",
			LogIndex:    7,
		},
		Payload:   &types.Payload{Data: "0xdeadbeef"},
		Metadata:  &types.Metadata{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ─── extended mock ──────────────────────────────────────────────────────────

// extendedMockQuerier wraps mockMessageQuerier and adds the two new methods.
type extendedMockQuerier struct {
	mockMessageQuerier
	historyFn func(ctx context.Context, address string, f AddressHistoryFilter) (*MessageListResult, error)
	traceFn   func(ctx context.Context, messageID string) (*TraceResponse, error)
}

func (m *extendedMockQuerier) GetAddressHistory(ctx context.Context, address string, f AddressHistoryFilter) (*MessageListResult, error) {
	return m.historyFn(ctx, address, f)
}

func (m *extendedMockQuerier) GetTrace(ctx context.Context, messageID string) (*TraceResponse, error) {
	return m.traceFn(ctx, messageID)
}

// ─── GetAddressHistoryHandler tests ─────────────────────────────────────────

func TestGetAddressHistoryHandler_Found(t *testing.T) {
	msg := sampleExecutedMessage("addr-msg-001")
	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, address string, _ AddressHistoryFilter) (*MessageListResult, error) {
			if address != "0xSender1111111111111111111111111111111111" {
				return &MessageListResult{}, nil
			}
			return &MessageListResult{Messages: []*types.Message{msg}}, nil
		},
	}

	handler := GetAddressHistoryHandler(mq)
	req := newChiRequest(http.MethodGet,
		"/address/0xSender1111111111111111111111111111111111/history",
		map[string]string{"address": "0xSender1111111111111111111111111111111111"},
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var resp addressHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Address != "0xSender1111111111111111111111111111111111" {
		t.Errorf("address = %q, want sender", resp.Address)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor should be empty, got %q", resp.NextCursor)
	}
}

func TestGetAddressHistoryHandler_NoResults(t *testing.T) {
	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, _ string, _ AddressHistoryFilter) (*MessageListResult, error) {
			return &MessageListResult{Messages: []*types.Message{}}, nil
		},
	}

	handler := GetAddressHistoryHandler(mq)
	req := newChiRequest(http.MethodGet, "/address/0xUnknown/history",
		map[string]string{"address": "0xUnknown"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp addressHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
}

func TestGetAddressHistoryHandler_WithFilters(t *testing.T) {
	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, _ string, f AddressHistoryFilter) (*MessageListResult, error) {
			if f.Protocol != "ccip" {
				t.Errorf("protocol = %q, want ccip", f.Protocol)
			}
			if f.Status != "executed" {
				t.Errorf("status = %q, want executed", f.Status)
			}
			if f.Limit != 5 {
				t.Errorf("limit = %d, want 5", f.Limit)
			}
			if f.SortOrder != "asc" {
				t.Errorf("sort = %q, want asc", f.SortOrder)
			}
			return &MessageListResult{}, nil
		},
	}

	handler := GetAddressHistoryHandler(mq)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("address", "0xAddr")
	req := httptest.NewRequest(http.MethodGet,
		"/address/0xAddr/history?protocol=ccip&status=executed&limit=5&sort=asc", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestGetAddressHistoryHandler_WithPagination(t *testing.T) {
	msg := sampleExecutedMessage("paged-msg-001")
	cursorTok := encodeCursor(cursor{CreatedAt: msg.CreatedAt, MessageID: msg.MessageID})

	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, _ string, _ AddressHistoryFilter) (*MessageListResult, error) {
			return &MessageListResult{
				Messages:   []*types.Message{msg},
				NextCursor: cursorTok,
			}, nil
		},
	}

	handler := GetAddressHistoryHandler(mq)
	req := newChiRequest(http.MethodGet, "/address/0xAddr/history?limit=1",
		map[string]string{"address": "0xAddr"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp addressHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.NextCursor == "" {
		t.Error("next_cursor should be set when there are more pages")
	}
}

func TestGetAddressHistoryHandler_MissingAddress(t *testing.T) {
	mq := &extendedMockQuerier{}

	handler := GetAddressHistoryHandler(mq)
	// No address URL param
	req := newChiRequest(http.MethodGet, "/address//history", map[string]string{"address": ""})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestGetAddressHistoryHandler_InvalidLimit(t *testing.T) {
	mq := &extendedMockQuerier{}

	handler := GetAddressHistoryHandler(mq)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("address", "0xAddr")
	req := httptest.NewRequest(http.MethodGet, "/address/0xAddr/history?limit=bad", nil)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetAddressHistoryHandler_InternalError(t *testing.T) {
	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, _ string, _ AddressHistoryFilter) (*MessageListResult, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := GetAddressHistoryHandler(mq)
	req := newChiRequest(http.MethodGet, "/address/0xAddr/history",
		map[string]string{"address": "0xAddr"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGetAddressHistoryHandler_MultiProtocolMessages(t *testing.T) {
	// Verify that history correctly returns messages across all 4 protocols.
	lzMsg := sampleExecutedMessage("lz-001")
	lzMsg.Protocol = "layerzero_v2"
	whMsg := samplePendingMessage("wh-001")
	whMsg.Protocol = "wormhole"
	axlMsg := sampleExecutedMessage("axl-001")
	axlMsg.Protocol = "axelar"
	axlMsg.Type = types.TypeContractCall
	ccipMsg := sampleExecutedMessage("ccip-001")
	ccipMsg.Protocol = "ccip"

	mq := &extendedMockQuerier{
		historyFn: func(_ context.Context, _ string, _ AddressHistoryFilter) (*MessageListResult, error) {
			return &MessageListResult{
				Messages: []*types.Message{lzMsg, whMsg, axlMsg, ccipMsg},
			}, nil
		},
	}

	handler := GetAddressHistoryHandler(mq)
	req := newChiRequest(http.MethodGet, "/address/0xAddr/history",
		map[string]string{"address": "0xSender1111111111111111111111111111111111"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp addressHistoryResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Count != 4 {
		t.Errorf("count = %d, want 4 (one per protocol)", resp.Count)
	}
}

// ─── GetTraceHandler tests ────────────────────────────────────────────────

func buildTrace(msg *types.Message) *TraceResponse {
	trace := &TraceResponse{
		MessageID: msg.MessageID,
		Protocol:  msg.Protocol,
		Type:      msg.Type,
		Status:    msg.Status,
		Payload:   msg.Payload,
		Metadata:  msg.Metadata,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
		Events: []TraceEvent{
			{
				Type:        "sent",
				ChainID:     msg.Source.ChainID,
				TxHash:      msg.Source.TxHash,
				BlockNumber: msg.Source.BlockNumber,
				Timestamp:   msg.Source.Timestamp,
				LogIndex:    msg.Source.LogIndex,
				Address:     msg.Source.Sender,
			},
		},
	}
	if msg.Destination != nil {
		trace.Events = append(trace.Events, TraceEvent{
			Type:        "received",
			ChainID:     msg.Destination.ChainID,
			TxHash:      msg.Destination.TxHash,
			BlockNumber: msg.Destination.BlockNumber,
			Timestamp:   msg.Destination.Timestamp,
			LogIndex:    msg.Destination.LogIndex,
			Address:     msg.Destination.Receiver,
		})
	}
	if msg.Metadata != nil && msg.Metadata.LatencySeconds > 0 {
		ls := msg.Metadata.LatencySeconds
		trace.LatencySeconds = &ls
	}
	return trace
}

func TestGetTraceHandler_ExecutedMessage(t *testing.T) {
	msg := sampleExecutedMessage("trace-exec-001")
	expectedTrace := buildTrace(msg)

	mq := &extendedMockQuerier{
		traceFn: func(_ context.Context, id string) (*TraceResponse, error) {
			if id == "trace-exec-001" {
				return expectedTrace, nil
			}
			return nil, nil
		},
	}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/trace-exec-001",
		map[string]string{"message_id": "trace-exec-001"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var resp TraceResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Core fields
	if resp.MessageID != "trace-exec-001" {
		t.Errorf("message_id = %q, want %q", resp.MessageID, "trace-exec-001")
	}
	if resp.Protocol != "layerzero_v2" {
		t.Errorf("protocol = %q, want layerzero_v2", resp.Protocol)
	}
	if resp.Status != types.StatusExecuted {
		t.Errorf("status = %q, want executed", resp.Status)
	}

	// Events: executed message must have both source and destination
	if len(resp.Events) != 2 {
		t.Fatalf("events len = %d, want 2", len(resp.Events))
	}
	if resp.Events[0].Type != "sent" {
		t.Errorf("events[0].type = %q, want sent", resp.Events[0].Type)
	}
	if resp.Events[0].ChainID != 1 {
		t.Errorf("events[0].chain_id = %d, want 1", resp.Events[0].ChainID)
	}
	if resp.Events[0].TxHash != "0xsrc" {
		t.Errorf("events[0].tx_hash = %q, want 0xsrc", resp.Events[0].TxHash)
	}
	if resp.Events[0].Address != "0xSender1111111111111111111111111111111111" {
		t.Errorf("events[0].address = %q", resp.Events[0].Address)
	}
	if resp.Events[1].Type != "received" {
		t.Errorf("events[1].type = %q, want received", resp.Events[1].Type)
	}
	if resp.Events[1].ChainID != 42161 {
		t.Errorf("events[1].chain_id = %d, want 42161", resp.Events[1].ChainID)
	}
	if resp.Events[1].TxHash != "0xdst" {
		t.Errorf("events[1].tx_hash = %q, want 0xdst", resp.Events[1].TxHash)
	}
	if resp.Events[1].Address != "0xReceiver2222222222222222222222222222222222" {
		t.Errorf("events[1].address = %q", resp.Events[1].Address)
	}

	// Latency
	if resp.LatencySeconds == nil {
		t.Fatal("latency_seconds should be set for executed message")
	}
	if *resp.LatencySeconds != 45 {
		t.Errorf("latency_seconds = %d, want 45", *resp.LatencySeconds)
	}

	// Payload
	if resp.Payload == nil {
		t.Fatal("payload should not be nil")
	}
	if resp.Payload.Token != "0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48" {
		t.Errorf("payload.token = %q", resp.Payload.Token)
	}
	if resp.Payload.Amount != "1000000000" {
		t.Errorf("payload.amount = %q, want 1000000000", resp.Payload.Amount)
	}
}

func TestGetTraceHandler_PendingMessage(t *testing.T) {
	msg := samplePendingMessage("trace-pend-001")
	expectedTrace := buildTrace(msg)

	mq := &extendedMockQuerier{
		traceFn: func(_ context.Context, _ string) (*TraceResponse, error) {
			return expectedTrace, nil
		},
	}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/trace-pend-001",
		map[string]string{"message_id": "trace-pend-001"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp TraceResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Status != types.StatusPending {
		t.Errorf("status = %q, want pending", resp.Status)
	}

	// Pending message has only one event (source)
	if len(resp.Events) != 1 {
		t.Fatalf("events len = %d, want 1 for pending message", len(resp.Events))
	}
	if resp.Events[0].Type != "sent" {
		t.Errorf("events[0].type = %q, want sent", resp.Events[0].Type)
	}

	// No latency for pending message
	if resp.LatencySeconds != nil {
		t.Errorf("latency_seconds should be nil for pending message, got %d", *resp.LatencySeconds)
	}
}

func TestGetTraceHandler_NotFound(t *testing.T) {
	mq := &extendedMockQuerier{
		traceFn: func(_ context.Context, _ string) (*TraceResponse, error) {
			return nil, nil
		},
	}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/nonexistent",
		map[string]string{"message_id": "nonexistent"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}

	var resp apiError
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Error != "message not found" {
		t.Errorf("error = %q, want 'message not found'", resp.Error)
	}
}

func TestGetTraceHandler_InternalError(t *testing.T) {
	mq := &extendedMockQuerier{
		traceFn: func(_ context.Context, _ string) (*TraceResponse, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/msg-001",
		map[string]string{"message_id": "msg-001"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestGetTraceHandler_MissingMessageID(t *testing.T) {
	mq := &extendedMockQuerier{}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/", map[string]string{"message_id": ""})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestGetTraceHandler_AllProtocols(t *testing.T) {
	// Each protocol should produce a valid trace with consistent structure.
	cases := []struct {
		name     string
		protocol string
		msgType  types.MessageType
		status   types.MessageStatus
	}{
		{"LayerZero executed", "layerzero_v2", types.TypeTokenTransfer, types.StatusExecuted},
		{"Wormhole pending", "wormhole", types.TypeMessage, types.StatusPending},
		{"Axelar executed", "axelar", types.TypeContractCall, types.StatusExecuted},
		{"CCIP failed", "ccip", types.TypeTokenTransfer, types.StatusFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := sampleExecutedMessage("proto-trace-001")
			msg.Protocol = tc.protocol
			msg.Type = tc.msgType
			msg.Status = tc.status
			if tc.status == types.StatusPending {
				msg.Destination = nil
				msg.Metadata.LatencySeconds = 0
			}
			if tc.status == types.StatusFailed {
				msg.Metadata.LatencySeconds = 0
			}
			trace := buildTrace(msg)

			mq := &extendedMockQuerier{
				traceFn: func(_ context.Context, _ string) (*TraceResponse, error) {
					return trace, nil
				},
			}

			handler := GetTraceHandler(mq)
			req := newChiRequest(http.MethodGet, "/trace/proto-trace-001",
				map[string]string{"message_id": "proto-trace-001"})
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("[%s] status = %d, want 200", tc.name, rec.Code)
			}

			var resp TraceResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("[%s] decode: %v", tc.name, err)
			}
			if resp.Protocol != tc.protocol {
				t.Errorf("[%s] protocol = %q, want %q", tc.name, resp.Protocol, tc.protocol)
			}
			if resp.Status != tc.status {
				t.Errorf("[%s] status = %q, want %q", tc.name, resp.Status, tc.status)
			}
			if len(resp.Events) == 0 {
				t.Errorf("[%s] events must not be empty", tc.name)
			}
			if resp.Events[0].Type != "sent" {
				t.Errorf("[%s] first event must be 'sent'", tc.name)
			}
		})
	}
}

func TestGetTraceHandler_EventTimestamps(t *testing.T) {
	// Verify that timestamps flow correctly from source/destination into events.
	msg := sampleExecutedMessage("ts-msg-001")
	msg.Source.Timestamp = 1706000000
	msg.Destination.Timestamp = 1706000045
	trace := buildTrace(msg)

	mq := &extendedMockQuerier{
		traceFn: func(_ context.Context, _ string) (*TraceResponse, error) { return trace, nil },
	}

	handler := GetTraceHandler(mq)
	req := newChiRequest(http.MethodGet, "/trace/ts-msg-001",
		map[string]string{"message_id": "ts-msg-001"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp TraceResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Events[0].Timestamp != 1706000000 {
		t.Errorf("source timestamp = %d, want 1706000000", resp.Events[0].Timestamp)
	}
	if resp.Events[1].Timestamp != 1706000045 {
		t.Errorf("dest timestamp = %d, want 1706000045", resp.Events[1].Timestamp)
	}
}
