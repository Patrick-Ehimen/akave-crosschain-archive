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

// mockMessageQuerier implements MessageQuerier for testing.
type mockMessageQuerier struct {
	getByIDFn    func(ctx context.Context, id string) (*types.Message, error)
	listFn       func(ctx context.Context, f MessageFilter) (*MessageListResult, error)
	getByTxFn    func(ctx context.Context, txHash string) ([]*types.Message, error)
}

func (m *mockMessageQuerier) GetByID(ctx context.Context, id string) (*types.Message, error) {
	return m.getByIDFn(ctx, id)
}

func (m *mockMessageQuerier) List(ctx context.Context, f MessageFilter) (*MessageListResult, error) {
	return m.listFn(ctx, f)
}

func (m *mockMessageQuerier) GetByTxHash(ctx context.Context, txHash string) ([]*types.Message, error) {
	return m.getByTxFn(ctx, txHash)
}

func sampleMessage() *types.Message {
	now := time.Now().Truncate(time.Second)
	return &types.Message{
		MessageID: "msg-123",
		Protocol:  "layerzero",
		Type:      types.TypeTokenTransfer,
		Status:    types.StatusExecuted,
		Source: types.Source{
			ChainID:     1,
			TxHash:      "0xabc",
			BlockNumber: 100,
			Timestamp:   1700000000,
			Sender:      "0xsender",
			LogIndex:    0,
		},
		Destination: &types.Destination{
			ChainID:     42161,
			TxHash:      "0xdef",
			BlockNumber: 200,
			Timestamp:   1700000060,
			Receiver:    "0xreceiver",
			LogIndex:    1,
		},
		Payload: &types.Payload{
			Token:  "0xtoken",
			Amount: "1000000",
		},
		Metadata: &types.Metadata{
			Fee:            "5000",
			LatencySeconds: 60,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// newChiRequest builds a request with chi URL params set.
func newChiRequest(method, path string, params map[string]string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestGetMessageHandler_Found(t *testing.T) {
	msg := sampleMessage()
	mq := &mockMessageQuerier{
		getByIDFn: func(_ context.Context, id string) (*types.Message, error) {
			if id == "msg-123" {
				return msg, nil
			}
			return nil, nil
		},
	}

	handler := GetMessageHandler(mq)
	req := newChiRequest(http.MethodGet, "/messages/msg-123", map[string]string{"message_id": "msg-123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp types.Message
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.MessageID != "msg-123" {
		t.Errorf("message_id = %q, want %q", resp.MessageID, "msg-123")
	}
	if resp.Protocol != "layerzero" {
		t.Errorf("protocol = %q, want %q", resp.Protocol, "layerzero")
	}
	if resp.Status != types.StatusExecuted {
		t.Errorf("status = %q, want %q", resp.Status, types.StatusExecuted)
	}
	if resp.Source.ChainID != 1 {
		t.Errorf("source.chain_id = %d, want 1", resp.Source.ChainID)
	}
	if resp.Destination == nil {
		t.Fatal("destination should not be nil")
	}
	if resp.Destination.ChainID != 42161 {
		t.Errorf("destination.chain_id = %d, want 42161", resp.Destination.ChainID)
	}
}

func TestGetMessageHandler_NotFound(t *testing.T) {
	mq := &mockMessageQuerier{
		getByIDFn: func(_ context.Context, _ string) (*types.Message, error) {
			return nil, nil
		},
	}

	handler := GetMessageHandler(mq)
	req := newChiRequest(http.MethodGet, "/messages/nonexistent", map[string]string{"message_id": "nonexistent"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var resp apiError
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "message not found" {
		t.Errorf("error = %q, want %q", resp.Error, "message not found")
	}
}

func TestGetMessageHandler_InternalError(t *testing.T) {
	mq := &mockMessageQuerier{
		getByIDFn: func(_ context.Context, _ string) (*types.Message, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := GetMessageHandler(mq)
	req := newChiRequest(http.MethodGet, "/messages/msg-123", map[string]string{"message_id": "msg-123"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestListMessagesHandler_NoFilters(t *testing.T) {
	msg := sampleMessage()
	mq := &mockMessageQuerier{
		listFn: func(_ context.Context, f MessageFilter) (*MessageListResult, error) {
			if f.Limit != 20 {
				t.Errorf("default limit = %d, want 20", f.Limit)
			}
			if f.SortOrder != "desc" {
				t.Errorf("default sort = %q, want desc", f.SortOrder)
			}
			return &MessageListResult{Messages: []*types.Message{msg}}, nil
		},
	}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet, "/messages", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp messagesListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
	if resp.NextCursor != "" {
		t.Errorf("next_cursor = %q, want empty", resp.NextCursor)
	}
}

func TestListMessagesHandler_WithFilters(t *testing.T) {
	mq := &mockMessageQuerier{
		listFn: func(_ context.Context, f MessageFilter) (*MessageListResult, error) {
			if f.Protocol != "axelar" {
				t.Errorf("protocol = %q, want axelar", f.Protocol)
			}
			if f.Status != "pending" {
				t.Errorf("status = %q, want pending", f.Status)
			}
			if f.SrcChain == nil || *f.SrcChain != 1 {
				t.Errorf("src_chain = %v, want 1", f.SrcChain)
			}
			if f.Sender != "0xsender" {
				t.Errorf("sender = %q, want 0xsender", f.Sender)
			}
			if f.Limit != 10 {
				t.Errorf("limit = %d, want 10", f.Limit)
			}
			if f.SortOrder != "asc" {
				t.Errorf("sort = %q, want asc", f.SortOrder)
			}
			return &MessageListResult{Messages: []*types.Message{}}, nil
		},
	}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet,
		"/messages?protocol=axelar&status=pending&src_chain=1&sender=0xsender&limit=10&sort=asc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestListMessagesHandler_WithPagination(t *testing.T) {
	msg := sampleMessage()
	cursorToken := encodeCursor(cursor{CreatedAt: msg.CreatedAt, MessageID: msg.MessageID})

	mq := &mockMessageQuerier{
		listFn: func(_ context.Context, f MessageFilter) (*MessageListResult, error) {
			return &MessageListResult{
				Messages:   []*types.Message{msg},
				NextCursor: cursorToken,
			}, nil
		},
	}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet, "/messages?limit=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp messagesListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.NextCursor == "" {
		t.Error("next_cursor should not be empty when there are more pages")
	}
}

func TestListMessagesHandler_InvalidSrcChain(t *testing.T) {
	mq := &mockMessageQuerier{}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet, "/messages?src_chain=invalid", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListMessagesHandler_InvalidLimit(t *testing.T) {
	mq := &mockMessageQuerier{}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet, "/messages?limit=-1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestListMessagesHandler_InternalError(t *testing.T) {
	mq := &mockMessageQuerier{
		listFn: func(_ context.Context, _ MessageFilter) (*MessageListResult, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet, "/messages", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestGetTxMessagesHandler_Found(t *testing.T) {
	msg := sampleMessage()
	mq := &mockMessageQuerier{
		getByTxFn: func(_ context.Context, txHash string) ([]*types.Message, error) {
			if txHash == "0xabc" {
				return []*types.Message{msg}, nil
			}
			return nil, nil
		},
	}

	handler := GetTxMessagesHandler(mq)
	req := newChiRequest(http.MethodGet, "/transactions/0xabc/messages", map[string]string{"tx_hash": "0xabc"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp txMessagesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.TxHash != "0xabc" {
		t.Errorf("tx_hash = %q, want %q", resp.TxHash, "0xabc")
	}
	if resp.Count != 1 {
		t.Errorf("count = %d, want 1", resp.Count)
	}
}

func TestGetTxMessagesHandler_NoResults(t *testing.T) {
	mq := &mockMessageQuerier{
		getByTxFn: func(_ context.Context, _ string) ([]*types.Message, error) {
			return []*types.Message{}, nil
		},
	}

	handler := GetTxMessagesHandler(mq)
	req := newChiRequest(http.MethodGet, "/transactions/0xnonexistent/messages",
		map[string]string{"tx_hash": "0xnonexistent"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var resp txMessagesResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Count != 0 {
		t.Errorf("count = %d, want 0", resp.Count)
	}
}

func TestGetTxMessagesHandler_InternalError(t *testing.T) {
	mq := &mockMessageQuerier{
		getByTxFn: func(_ context.Context, _ string) ([]*types.Message, error) {
			return nil, fmt.Errorf("db error")
		},
	}

	handler := GetTxMessagesHandler(mq)
	req := newChiRequest(http.MethodGet, "/transactions/0xabc/messages", map[string]string{"tx_hash": "0xabc"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCursorEncodeDecode(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	original := cursor{CreatedAt: now, MessageID: "test-id"}

	encoded := encodeCursor(original)
	decoded, err := decodeCursor(encoded)
	if err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if !decoded.CreatedAt.Equal(original.CreatedAt) {
		t.Errorf("created_at = %v, want %v", decoded.CreatedAt, original.CreatedAt)
	}
	if decoded.MessageID != original.MessageID {
		t.Errorf("message_id = %q, want %q", decoded.MessageID, original.MessageID)
	}
}

func TestDecodeCursor_Invalid(t *testing.T) {
	_, err := decodeCursor("not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid cursor")
	}
}

func TestListMessagesHandler_TimeRange(t *testing.T) {
	mq := &mockMessageQuerier{
		listFn: func(_ context.Context, f MessageFilter) (*MessageListResult, error) {
			if f.FromTS == nil || *f.FromTS != 1700000000 {
				t.Errorf("from_ts = %v, want 1700000000", f.FromTS)
			}
			if f.ToTS == nil || *f.ToTS != 1700099999 {
				t.Errorf("to_ts = %v, want 1700099999", f.ToTS)
			}
			return &MessageListResult{Messages: []*types.Message{}}, nil
		},
	}

	handler := ListMessagesHandler(mq)
	req := httptest.NewRequest(http.MethodGet,
		"/messages?from_ts=1700000000&to_ts=1700099999", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
