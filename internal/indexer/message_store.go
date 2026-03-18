package indexer

import (
	"context"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// MessageStore persists and retrieves cross-chain messages.
type MessageStore interface {
	// UpsertMessage inserts a new message or updates an existing one.
	// Writes to messages, message_sources, message_payloads, and message_metadata
	// tables atomically using ON CONFLICT for idempotent upserts.
	UpsertMessage(ctx context.Context, msg *types.Message) error

	// FindByCorrelationKey looks up an existing pending message using the
	// composite key (protocol, nonce, sender, src_chain_id).
	// Returns nil, nil if no matching message is found.
	FindByCorrelationKey(ctx context.Context, key *normalizer.CorrelationKey) (*types.Message, error)

	// UpdateDestination sets the destination details and updates the message status.
	// Computes and stores latency when both source and destination timestamps are available.
	UpdateDestination(ctx context.Context, messageID string, dest *types.Destination, status types.MessageStatus) error
}
