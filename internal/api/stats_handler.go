package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// knownProtocols is the set of valid protocol names. Requests for anything
// outside this set get a 404 rather than an expensive empty-result query.
var knownProtocols = map[string]struct{}{
	"layerzero_v2": {},
	"wormhole":     {},
	"axelar":       {},
	"ccip":         {},
}

// GetProtocolStatsHandler returns a handler for GET /protocols/{protocol}/stats
//
// Path parameter:
//   - protocol — one of: layerzero_v2, wormhole, axelar, ccip
//
// Query parameters (all optional):
//   - from_ts  — Unix timestamp (seconds), lower bound on source event time
//   - to_ts    — Unix timestamp (seconds), upper bound on source event time
//
// Response:
//
//	{
//	  "protocol":            "layerzero_v2",
//	  "counts": {
//	    "executed": 1200,
//	    "pending":  50,
//	    "failed":   10,
//	    "total":    1260
//	  },
//	  "success_rate":          0.9917,
//	  "avg_latency_seconds":   42.3,
//	  "p50_latency_seconds":   38.0,
//	  "p95_latency_seconds":   120.5,
//	  "computed_at":           "2025-01-15T10:00:00Z"
//	}
//
// Returns HTTP 404 for unknown protocol names.
// Returns HTTP 200 with zero counts when the protocol is known but has no data.
func GetProtocolStatsHandler(sq StatsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		protocol := chi.URLParam(r, "protocol")
		if protocol == "" {
			writeError(w, http.StatusBadRequest, "protocol is required")
			return
		}

		// Reject unknown protocol names immediately — avoids a spurious DB
		// round-trip and gives a clear 404 rather than empty-but-200 data.
		if _, ok := knownProtocols[strings.ToLower(protocol)]; !ok {
			writeError(w, http.StatusNotFound,
				"unknown protocol: must be one of layerzero_v2, wormhole, axelar, ccip")
			return
		}
		protocol = strings.ToLower(protocol)

		q := r.URL.Query()

		fromTS, err := ParseInt64Param(q.Get("from_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from_ts: must be a Unix timestamp")
			return
		}
		toTS, err := ParseInt64Param(q.Get("to_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to_ts: must be a Unix timestamp")
			return
		}
		if fromTS != nil && toTS != nil && *fromTS > *toTS {
			writeError(w, http.StatusBadRequest, "from_ts must not be greater than to_ts")
			return
		}

		filter := StatsFilter{
			FromTS: fromTS,
			ToTS:   toTS,
		}

		stats, err := sq.GetProtocolStats(r.Context(), protocol, filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to compute protocol stats")
			return
		}

		writeJSON(w, http.StatusOK, stats)
	}
}

// GetRoutesStatsHandler returns a handler for GET /routes/stats
//
// Query parameters (all optional):
//   - protocol — filter by protocol name
//   - from_ts  — Unix timestamp (seconds), lower bound on source event time
//   - to_ts    — Unix timestamp (seconds), upper bound on source event time
//   - limit    — max routes to return (1–100, default 20)
//
// Response:
//
//	{
//	  "routes": [
//	    {
//	      "src_chain_id": 1,
//	      "dst_chain_id": 42161,
//	      "counts": { "executed": 800, "pending": 20, "failed": 5, "total": 825 },
//	      "success_rate": 0.9939,
//	      "avg_latency_seconds": 38.4
//	    },
//	    ...
//	  ],
//	  "total_routes": 12,
//	  "computed_at": "2025-01-15T10:00:00Z"
//	}
func GetRoutesStatsHandler(sq StatsQuerier) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		fromTS, err := ParseInt64Param(q.Get("from_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from_ts: must be a Unix timestamp")
			return
		}
		toTS, err := ParseInt64Param(q.Get("to_ts"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid to_ts: must be a Unix timestamp")
			return
		}
		if fromTS != nil && toTS != nil && *fromTS > *toTS {
			writeError(w, http.StatusBadRequest, "from_ts must not be greater than to_ts")
			return
		}

		protocol := strings.ToLower(q.Get("protocol"))
		if protocol != "" {
			if _, ok := knownProtocols[protocol]; !ok {
				writeError(w, http.StatusBadRequest,
					"invalid protocol: must be one of layerzero_v2, wormhole, axelar, ccip")
				return
			}
		}

		limit := 20
		if ls := q.Get("limit"); ls != "" {
			l, err := strconv.Atoi(ls)
			if err != nil || l < 1 {
				writeError(w, http.StatusBadRequest, "invalid limit: must be a positive integer")
				return
			}
			if l > 100 {
				l = 100
			}
			limit = l
		}

		filter := StatsFilter{
			FromTS:   fromTS,
			ToTS:     toTS,
			Protocol: protocol,
			Limit:    limit,
		}

		result, err := sq.GetRoutesStats(r.Context(), filter)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to compute routes stats")
			return
		}

		writeJSON(w, http.StatusOK, result)
	}
}
