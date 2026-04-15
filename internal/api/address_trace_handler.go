package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// GetAddressHistoryHandler returns a handler for
// GET /address/{address}/history
//
// Query parameters (all optional):
//   - protocol      — filter by protocol name (layerzero_v2, wormhole, axelar, ccip)
//   - src_chain     — filter by source chain EVM ID
//   - dst_chain     — filter by destination chain EVM ID
//   - status        — filter by message status (pending, executed, failed)
//   - cursor        — opaque pagination token from previous response
//   - limit         — page size, 1–100, default 20
//   - sort          — "asc" or "desc" (default "desc")
//
// Response:
//
//	{
//	  "address": "0x...",
//	  "messages": [...],
//	  "count": 2,
//	  "next_cursor": "..."
//	}
func GetAddressHistoryHandler(mq MessageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := chi.URLParam(r, "address")
		if address == "" {
			writeError(w, http.StatusBadRequest, "address is required")
			return
		}

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

		limit := 20
		if ls := q.Get("limit"); ls != "" {
			l, err2 := parseInt(ls)
			if err2 != nil || l < 1 {
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

		filter := AddressHistoryFilter{
			Protocol:  q.Get("protocol"),
			Status:    q.Get("status"),
			SrcChain:  srcChain,
			DstChain:  dstChain,
			Cursor:    q.Get("cursor"),
			Limit:     limit,
			SortOrder: sortOrder,
		}

		result, err := mq.GetAddressHistory(r.Context(), address, filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to retrieve address history")
			return
		}

		msgs := make([]any, len(result.Messages))
		for i, m := range result.Messages {
			msgs[i] = m
		}

		writeJSON(w, http.StatusOK, addressHistoryResponse{
			Address:    address,
			Messages:   msgs,
			Count:      len(result.Messages),
			NextCursor: result.NextCursor,
		})
	}
}

// GetTraceHandler returns a handler for GET /trace/{message_id}
//
// Returns the full end-to-end lifecycle for a message:
//   - source event (always present)
//   - destination event (present when message is executed or failed)
//   - status, latency, payload, metadata
//
// Returns 404 when the message_id is not found.
func GetTraceHandler(mq MessageQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		messageID := chi.URLParam(r, "message_id")
		if messageID == "" {
			writeError(w, http.StatusBadRequest, "message_id is required")
			return
		}

		trace, err := mq.GetTrace(r.Context(), messageID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to retrieve trace")
			return
		}
		if trace == nil {
			writeError(w, http.StatusNotFound, "message not found")
			return
		}

		writeJSON(w, http.StatusOK, trace)
	}
}

// addressHistoryResponse is the JSON envelope for address history responses.
type addressHistoryResponse struct {
	Address    string `json:"address"`
	Messages   []any  `json:"messages"`
	Count      int    `json:"count"`
	NextCursor string `json:"next_cursor,omitempty"`
}

// parseInt is a small helper to avoid importing strconv in the handler file.
func parseInt(s string) (int, error) {
	v, err := ParseUint64Param(s)
	if err != nil || v == nil {
		return 0, err
	}
	return int(*v), nil
}
