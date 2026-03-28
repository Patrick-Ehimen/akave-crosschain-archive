package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// MessageFilter contains the supported query parameters for listing messages.
type MessageFilter struct {
	SrcChain  *uint64
	DstChain  *uint64
	Protocol  string
	Status    string
	Sender    string
	Receiver  string
	FromTS    *int64
	ToTS      *int64
	Cursor    string // opaque cursor token
	Limit     int
	SortOrder string // "asc" or "desc"
}

// MessageListResult wraps paginated results.
type MessageListResult struct {
	Messages   []*types.Message `json:"messages"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// cursor is the internal representation of a pagination cursor.
type cursor struct {
	CreatedAt time.Time `json:"c"`
	MessageID string    `json:"m"`
}

func encodeCursor(c cursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	var c cursor
	if err := json.Unmarshal(b, &c); err != nil {
		return cursor{}, fmt.Errorf("invalid cursor: %w", err)
	}
	return c, nil
}

// MessageQuerier provides read-only access to cross-chain messages.
type MessageQuerier interface {
	GetByID(ctx context.Context, messageID string) (*types.Message, error)
	List(ctx context.Context, filter MessageFilter) (*MessageListResult, error)
	GetByTxHash(ctx context.Context, txHash string) ([]*types.Message, error)
}

// PgMessageQuerier implements MessageQuerier using PostgreSQL.
type PgMessageQuerier struct {
	pool *pgxpool.Pool
}

// NewPgMessageQuerier creates a new PgMessageQuerier.
func NewPgMessageQuerier(pool *pgxpool.Pool) *PgMessageQuerier {
	return &PgMessageQuerier{pool: pool}
}

const baseSelect = `
	SELECT m.message_id, m.protocol, m.type, m.status, m.created_at, m.updated_at,
	       s.chain_id, s.tx_hash, s.block_number, s.timestamp, s.sender, s.log_index,
	       d.chain_id, d.tx_hash, d.block_number, d.timestamp, d.receiver, d.log_index,
	       p.token, p.amount, p.data, p.nonce,
	       md.fee, md.relayer, md.gas_used, md.latency_seconds
	FROM messages m
	JOIN message_sources s ON m.message_id = s.message_id
	LEFT JOIN message_destinations d ON m.message_id = d.message_id
	LEFT JOIN message_payloads p ON m.message_id = p.message_id
	LEFT JOIN message_metadata md ON m.message_id = md.message_id`

// GetByID returns a single message by its message_id.
func (q *PgMessageQuerier) GetByID(ctx context.Context, messageID string) (*types.Message, error) {
	query := baseSelect + ` WHERE m.message_id = $1`
	row := q.pool.QueryRow(ctx, query, messageID)
	msg, err := scanFullMessage(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying message by id: %w", err)
	}
	return msg, nil
}

// List returns messages matching the filter with cursor-based pagination.
func (q *PgMessageQuerier) List(ctx context.Context, filter MessageFilter) (*MessageListResult, error) {
	var conditions []string
	var args []any
	argIdx := 1

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
	if filter.Sender != "" {
		conditions = append(conditions, fmt.Sprintf("s.sender = $%d", argIdx))
		args = append(args, filter.Sender)
		argIdx++
	}
	if filter.Receiver != "" {
		conditions = append(conditions, fmt.Sprintf("d.receiver = $%d", argIdx))
		args = append(args, filter.Receiver)
		argIdx++
	}
	if filter.FromTS != nil {
		conditions = append(conditions, fmt.Sprintf("s.timestamp >= $%d", argIdx))
		args = append(args, *filter.FromTS)
		argIdx++
	}
	if filter.ToTS != nil {
		conditions = append(conditions, fmt.Sprintf("s.timestamp <= $%d", argIdx))
		args = append(args, *filter.ToTS)
		argIdx++
	}

	// Cursor-based pagination
	sortOrder := "DESC"
	if filter.SortOrder == "asc" {
		sortOrder = "ASC"
	}

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

	query := baseSelect
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += fmt.Sprintf(" ORDER BY m.created_at %s, m.message_id %s", sortOrder, sortOrder)

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query += fmt.Sprintf(" LIMIT $%d", argIdx)
	args = append(args, limit+1) // fetch one extra to detect next page

	rows, err := q.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing messages: %w", err)
	}
	defer rows.Close()

	var messages []*types.Message
	for rows.Next() {
		msg, err := scanFullMessageFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}

	result := &MessageListResult{}
	if len(messages) > limit {
		// There's a next page
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

// GetByTxHash returns all messages associated with a transaction hash (source or destination).
func (q *PgMessageQuerier) GetByTxHash(ctx context.Context, txHash string) ([]*types.Message, error) {
	query := baseSelect + `
		WHERE s.tx_hash = $1 OR d.tx_hash = $1
		ORDER BY m.created_at DESC`

	rows, err := q.pool.Query(ctx, query, txHash)
	if err != nil {
		return nil, fmt.Errorf("querying messages by tx hash: %w", err)
	}
	defer rows.Close()

	var messages []*types.Message
	for rows.Next() {
		msg, err := scanFullMessageFromRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning message row: %w", err)
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating message rows: %w", err)
	}

	return messages, nil
}

// scanFullMessage scans a single row from a query returning all message fields.
func scanFullMessage(row pgx.Row) (*types.Message, error) {
	msg := &types.Message{}
	var (
		msgType, msgStatus             string
		dstChainID, dstBlockNumber     *uint64
		dstTxHash, dstReceiver         *string
		dstTimestamp                    *int64
		dstLogIndex                    *uint
		payloadToken, payloadAmount    *string
		payloadData                    *string
		payloadNonce                   *uint64
		metaFee, metaRelayer           *string
		metaGasUsed, metaLatency       *int64
	)

	err := row.Scan(
		&msg.MessageID, &msg.Protocol, &msgType, &msgStatus, &msg.CreatedAt, &msg.UpdatedAt,
		&msg.Source.ChainID, &msg.Source.TxHash, &msg.Source.BlockNumber,
		&msg.Source.Timestamp, &msg.Source.Sender, &msg.Source.LogIndex,
		&dstChainID, &dstTxHash, &dstBlockNumber, &dstTimestamp, &dstReceiver, &dstLogIndex,
		&payloadToken, &payloadAmount, &payloadData, &payloadNonce,
		&metaFee, &metaRelayer, &metaGasUsed, &metaLatency,
	)
	if err != nil {
		return nil, err
	}

	msg.Type = types.MessageType(msgType)
	msg.Status = types.MessageStatus(msgStatus)

	if dstChainID != nil {
		msg.Destination = &types.Destination{
			ChainID:     *dstChainID,
			TxHash:      derefStr(dstTxHash),
			BlockNumber: *dstBlockNumber,
			Timestamp:   derefInt64(dstTimestamp),
			Receiver:    derefStr(dstReceiver),
			LogIndex:    derefUint(dstLogIndex),
		}
	}

	if payloadToken != nil || payloadAmount != nil || payloadData != nil || payloadNonce != nil {
		msg.Payload = &types.Payload{
			Token:  derefStr(payloadToken),
			Amount: derefStr(payloadAmount),
			Data:   derefStr(payloadData),
			Nonce:  derefUint64(payloadNonce),
		}
	}

	if metaFee != nil || metaRelayer != nil || metaGasUsed != nil || metaLatency != nil {
		msg.Metadata = &types.Metadata{
			Fee:            derefStr(metaFee),
			Relayer:        derefStr(metaRelayer),
			GasUsed:        uint64(derefInt64OrZero(metaGasUsed)),
			LatencySeconds: derefInt64(metaLatency),
		}
	}

	return msg, nil
}

// scanFullMessageFromRows scans a pgx.Rows result.
func scanFullMessageFromRows(rows pgx.Rows) (*types.Message, error) {
	msg := &types.Message{}
	var (
		msgType, msgStatus             string
		dstChainID, dstBlockNumber     *uint64
		dstTxHash, dstReceiver         *string
		dstTimestamp                    *int64
		dstLogIndex                    *uint
		payloadToken, payloadAmount    *string
		payloadData                    *string
		payloadNonce                   *uint64
		metaFee, metaRelayer           *string
		metaGasUsed, metaLatency       *int64
	)

	err := rows.Scan(
		&msg.MessageID, &msg.Protocol, &msgType, &msgStatus, &msg.CreatedAt, &msg.UpdatedAt,
		&msg.Source.ChainID, &msg.Source.TxHash, &msg.Source.BlockNumber,
		&msg.Source.Timestamp, &msg.Source.Sender, &msg.Source.LogIndex,
		&dstChainID, &dstTxHash, &dstBlockNumber, &dstTimestamp, &dstReceiver, &dstLogIndex,
		&payloadToken, &payloadAmount, &payloadData, &payloadNonce,
		&metaFee, &metaRelayer, &metaGasUsed, &metaLatency,
	)
	if err != nil {
		return nil, err
	}

	msg.Type = types.MessageType(msgType)
	msg.Status = types.MessageStatus(msgStatus)

	if dstChainID != nil {
		msg.Destination = &types.Destination{
			ChainID:     *dstChainID,
			TxHash:      derefStr(dstTxHash),
			BlockNumber: *dstBlockNumber,
			Timestamp:   derefInt64(dstTimestamp),
			Receiver:    derefStr(dstReceiver),
			LogIndex:    derefUint(dstLogIndex),
		}
	}

	if payloadToken != nil || payloadAmount != nil || payloadData != nil || payloadNonce != nil {
		msg.Payload = &types.Payload{
			Token:  derefStr(payloadToken),
			Amount: derefStr(payloadAmount),
			Data:   derefStr(payloadData),
			Nonce:  derefUint64(payloadNonce),
		}
	}

	if metaFee != nil || metaRelayer != nil || metaGasUsed != nil || metaLatency != nil {
		msg.Metadata = &types.Metadata{
			Fee:            derefStr(metaFee),
			Relayer:        derefStr(metaRelayer),
			GasUsed:        uint64(derefInt64OrZero(metaGasUsed)),
			LatencySeconds: derefInt64(metaLatency),
		}
	}

	return msg, nil
}

// ParseUint64Param parses a uint64 from a query parameter string.
func ParseUint64Param(s string) (*uint64, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid numeric parameter %q: %w", s, err)
	}
	return &v, nil
}

// ParseInt64Param parses an int64 from a query parameter string.
func ParseInt64Param(s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid numeric parameter %q: %w", s, err)
	}
	return &v, nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func derefInt64(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func derefInt64OrZero(i *int64) int64 {
	if i == nil {
		return 0
	}
	return *i
}

func derefUint(u *uint) uint {
	if u == nil {
		return 0
	}
	return *u
}

func derefUint64(u *uint64) uint64 {
	if u == nil {
		return 0
	}
	return *u
}
