package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// apiError is the consistent error response format.
type apiError struct {
	Error string `json:"error"`
}

// messagesListResponse wraps the paginated list response.
type messagesListResponse struct {
	Messages   []any  `json:"messages"`
	NextCursor string `json:"next_cursor,omitempty"`
	Count      int    `json:"count"`
}

// txMessagesResponse wraps the tx hash lookup response.
type txMessagesResponse struct {
	TxHash   string `json:"tx_hash"`
	Messages []any  `json:"messages"`
	Count    int    `json:"count"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, apiError{Error: msg})
}

// GetMessageHandler returns a handler for GET /messages/{message_id}.
func GetMessageHandler(mq MessageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		messageID := chi.URLParam(r, "message_id")
		if messageID == "" {
			writeError(w, http.StatusBadRequest, "message_id is required")
			return
		}

		msg, err := mq.GetByID(r.Context(), messageID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to retrieve message")
			return
		}
		if msg == nil {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}

		writeJSON(w, http.StatusOK, msg)
	}
}

// ListMessagesHandler returns a handler for GET /messages with filtering and pagination.
func ListMessagesHandler(mq MessageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		srcChain, err := ParseUint64Param(q.Get("src_chain"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid src_chain parameter")
			return
		}
		dstChain, err := ParseUint64Param(q.Get("dst_chain"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid dst_chain parameter")
			return
		}
		fromTS, err := ParseInt64Param(q.Get("from_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from_ts parameter")
			return
		}
		toTS, err := ParseInt64Param(q.Get("to_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to_ts parameter")
			return
		}

		limit := 20
		if ls := q.Get("limit"); ls != "" {
			l, err := strconv.Atoi(ls)
			if err != nil || l < 1 {
				writeError(w, http.StatusBadRequest, "invalid limit parameter")
				return
			}
			if l > 100 {
				l = 100
			}
			limit = l
		}

		sortOrder := "desc"
		if so := q.Get("sort"); so == "asc" || so == "desc" {
			sortOrder = so
		}

		filter := MessageFilter{
			SrcChain:  srcChain,
			DstChain:  dstChain,
			Protocol:  q.Get("protocol"),
			Status:    q.Get("status"),
			Sender:    q.Get("sender"),
			Receiver:  q.Get("receiver"),
			FromTS:    fromTS,
			ToTS:      toTS,
			Cursor:    q.Get("cursor"),
			Limit:     limit,
			SortOrder: sortOrder,
		}

		result, err := mq.List(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list messages")
			return
		}

		// Convert to []any to ensure null-safe JSON array
		msgs := make([]any, len(result.Messages))
		for i, m := range result.Messages {
			msgs[i] = m
		}

		writeJSON(w, http.StatusOK, messagesListResponse{
			Messages:   msgs,
			NextCursor: result.NextCursor,
			Count:      len(result.Messages),
		})
	}
}

// GetTxMessagesHandler returns a handler for GET /transactions/{tx_hash}/messages.
func GetTxMessagesHandler(mq MessageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		txHash := chi.URLParam(r, "tx_hash")
		if txHash == "" {
			writeError(w, http.StatusBadRequest, "tx_hash is required")
			return
		}

		messages, err := mq.GetByTxHash(r.Context(), txHash)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to retrieve messages")
			return
		}

		msgs := make([]any, len(messages))
		for i, m := range messages {
			msgs[i] = m
		}

		writeJSON(w, http.StatusOK, txMessagesResponse{
			TxHash:   txHash,
			Messages: msgs,
			Count:    len(messages),
		})
	}
}
