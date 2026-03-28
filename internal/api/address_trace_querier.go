package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// ─── Address History ──────────────────────────────────────────────────────

// AddressHistoryFilter holds the supported query parameters for address history.
type AddressHistoryFilter struct {
	Protocol  string
	SrcChain  *uint64
	DstChain  *uint64
	Status    string
	Cursor    string
	Limit     int
	SortOrder string // "asc" or "desc"
}

// GetAddressHistory returns all messages where the address appears as sender
// (in message_sources) OR as receiver (in message_destinations), with
// cursor-based pagination and optional protocol/chain/status filters.
//
// The UNION approach is used so that a single indexed query can serve both
// sender and receiver lookups simultaneously without a full-table scan.
// Each branch of the UNION hits its respective composite index:
//   - idx_message_sources_sender_timestamp
//   - idx_message_destinations_receiver_timestamp
func (q *PgMessageQuerier) GetAddressHistory(
	ctx context.Context,
	address string,
	filter AddressHistoryFilter,
) (*MessageListResult, error) {
	if address == "" {
		return nil, fmt.Errorf("address is required")
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	sortOrder := "DESC"
	if strings.ToLower(filter.SortOrder) == "asc" {
		sortOrder = "ASC"
	}

	// Build dynamic WHERE clauses applied to the outer query
	// (wrapping the UNION so filters apply uniformly to both branches).
	var conditions []string
	var args []interface{}
	argIdx := 1

	// $1 is always the address (used in both UNION branches).
	args = append(args, address)
	argIdx++ // argIdx is now 2

	if filter.Protocol != "" {
		conditions = append(conditions, fmt.Sprintf("m.protocol = $%d", argIdx))
		args = append(args, filter.Protocol)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("m.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.SrcChain != nil {
		conditions = append(conditions, fmt.Sprintf("s.chain_id = $%d", argIdx))
		args = append(args, *filter.SrcChain)
		argIdx++
	}
	if filter.DstChain != nil {
		conditions = append(conditions, fmt.Sprintf("d.chain_id = $%d", argIdx))
		args = append(args, *filter.DstChain)
		argIdx++
	}

	// Cursor pagination
	if filter.Cursor != "" {
		cur, err := decodeCursor(filter.Cursor)
		if err != nil {
			return nil, err
		}
		if sortOrder == "DESC" {
			conditions = append(conditions,
				fmt.Sprintf("(m.created_at, m.message_id) < ($%d, $%d)", argIdx, argIdx+1))
		} else {
			conditions = append(conditions,
				fmt.Sprintf("(m.created_at, m.message_id) > ($%d, $%d)", argIdx, argIdx+1))
		}
		args = append(args, cur.CreatedAt, cur.MessageID)
		argIdx += 2
	}

	// Build the WHERE clause for the outer query. The CTE already filters
	// by address, so these conditions use AND against the joined tables.
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "AND " + strings.Join(conditions, " AND ")
	}

	// A CTE first resolves the matching message IDs via indexed UNION,
	// then the outer query joins the full message data and sorts by
	// (created_at, message_id) for correct cursor-based pagination.
	query := fmt.Sprintf(`
		WITH matching_ids AS (
			SELECT message_id FROM message_sources WHERE sender = $1
			UNION
			SELECT message_id FROM message_destinations WHERE receiver = $1
		)
		SELECT m.message_id, m.protocol, m.type, m.status, m.created_at, m.updated_at,
		       s.chain_id, s.tx_hash, s.block_number, s.timestamp, s.sender, s.log_index,
		       d.chain_id, d.tx_hash, d.block_number, d.timestamp, d.receiver, d.log_index,
		       p.token, p.amount, p.data, p.nonce,
		       md.fee, md.relayer, md.gas_used, md.latency_seconds
		FROM messages m
		JOIN matching_ids mi ON m.message_id = mi.message_id
		JOIN message_sources s ON m.message_id = s.message_id
		LEFT JOIN message_destinations d ON m.message_id = d.message_id
		LEFT JOIN message_payloads p ON m.message_id = p.message_id
		LEFT JOIN message_metadata md ON m.message_id = md.message_id
		WHERE TRUE %s
		ORDER BY m.created_at %s, m.message_id %s
		LIMIT $%d`,
		whereClause, sortOrder, sortOrder, argIdx)

	args = append(args, limit+1)

	rows, err := q.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying address history: %w", err)
	}
	defer rows.Close()

	var messages []*types.Message
	for rows.Next() {
		msg, err := scanFullMessageFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning address history row: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating address history rows: %w", err)
	}

	result := &MessageListResult{}
	if len(messages) > limit {
		last := messages[limit-1]
		result.NextCursor = encodeCursor(cursor{
			CreatedAt: last.CreatedAt,
			MessageID: last.MessageID,
		})
		messages = messages[:limit]
	}
	result.Messages = messages
	return result, nil
}

// ─── Trace ────────────────────────────────────────────────────────────────

// TraceEvent describes a single point in a message's cross-chain lifecycle.
type TraceEvent struct {
	// Type identifies the event: "sent" (source) or "received" (destination).
	Type string `json:"type"`

	// ChainID is the EVM chain ID where this event was observed.
	ChainID uint64 `json:"chain_id"`

	// TxHash is the transaction hash that contained this event.
	TxHash string `json:"tx_hash"`

	// BlockNumber is the block at which this event was emitted.
	BlockNumber uint64 `json:"block_number"`

	// Timestamp is the Unix timestamp of the block (seconds).
	Timestamp int64 `json:"timestamp"`

	// LogIndex is the position of the event log within the transaction.
	LogIndex uint `json:"log_index"`

	// Address is the sender (for "sent") or receiver (for "received").
	Address string `json:"address"`
}

// TraceResponse is the full end-to-end lifecycle for a single message.
type TraceResponse struct {
	// MessageID is the protocol-specific unique identifier.
	MessageID string `json:"message_id"`

	// Protocol identifies which bridge protocol handled this message.
	Protocol string `json:"protocol"`

	// Type categorises the message (token_transfer, message, contract_call).
	Type types.MessageType `json:"type"`

	// Status is the current lifecycle state (pending, executed, failed).
	Status types.MessageStatus `json:"status"`

	// Events is the ordered sequence of observed on-chain events.
	// Always has at least one entry (the source event).
	// Has two entries when the destination has been observed.
	Events []TraceEvent `json:"events"`

	// LatencySeconds is the time between source and destination events.
	// Nil when the message is still pending.
	LatencySeconds *int64 `json:"latency_seconds,omitempty"`

	// Payload holds the message or token transfer details.
	Payload *types.Payload `json:"payload,omitempty"`

	// Metadata holds fee and relayer details.
	Metadata *types.Metadata `json:"metadata,omitempty"`

	// CreatedAt is when the source event was first indexed.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the message was last modified.
	UpdatedAt time.Time `json:"updated_at"`
}

// GetTrace returns the full event lifecycle for a single message identified
// by its protocol-specific message_id.  Returns nil, nil when the message
// is not found (the handler maps this to HTTP 404).
//
// The trace is assembled from the full message row plus its source and
// destination events.  Source is always present; destination is present
// only when the message has been correlated (status != pending).
func (q *PgMessageQuerier) GetTrace(
	ctx context.Context,
	messageID string,
) (*TraceResponse, error) {
	if messageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}

	// Re-use the shared full-message scanner; it already handles all NULL
	// fields for pending (no destination) messages.
	query := baseSelect + ` WHERE m.message_id = $1`
	row := q.pool.QueryRow(ctx, query, messageID)

	msg, err := scanFullMessage(row)
	if err == pgx.ErrNoRows {
		return nil, nil // caller returns 404
	}
	if err != nil {
		return nil, fmt.Errorf("querying trace for message %q: %w", messageID, err)
	}

	trace := &TraceResponse{
		MessageID: msg.MessageID,
		Protocol:  msg.Protocol,
		Type:      msg.Type,
		Status:    msg.Status,
		Payload:   msg.Payload,
		Metadata:  msg.Metadata,
		CreatedAt: msg.CreatedAt,
		UpdatedAt: msg.UpdatedAt,
	}

	// Source event is always present.
	trace.Events = append(trace.Events, TraceEvent{
		Type:        "sent",
		ChainID:     msg.Source.ChainID,
		TxHash:      msg.Source.TxHash,
		BlockNumber: msg.Source.BlockNumber,
		Timestamp:   msg.Source.Timestamp,
		LogIndex:    msg.Source.LogIndex,
		Address:     msg.Source.Sender,
	})

	// Destination event is present only once correlation has occurred.
	if msg.Destination != nil && msg.Destination.TxHash != "" {
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

	// Surface latency at the top level for easy consumption.
	if msg.Metadata != nil && msg.Metadata.LatencySeconds > 0 {
		ls := msg.Metadata.LatencySeconds
		trace.LatencySeconds = &ls
	}

	return trace, nil
}