package correlator

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// MessageStore is the subset of indexer.MessageStore needed by the correlator.
type MessageStore interface {
	UpsertMessage(ctx context.Context, msg *types.Message) error
	FindByCorrelationKey(ctx context.Context, key *normalizer.CorrelationKey) (*types.Message, error)
	UpdateDestination(ctx context.Context, messageID string, dest *types.Destination, status types.MessageStatus) error
}

// Correlator handles cross-chain message matching by routing decoded events
// through the normalizer and persisting or correlating them via the message store.
type Correlator struct {
	store MessageStore
	log   zerolog.Logger
}

// New creates a new Correlator.
func New(store MessageStore, log zerolog.Logger) *Correlator {
	return &Correlator{
		store: store,
		log:   log.With().Str("component", "correlator").Logger(),
	}
}

// Process normalizes a decoded event and either stores it as a new message
// or correlates it with an existing one.
func (c *Correlator) Process(ctx context.Context, event *decoder.RawEvent) error {
	result, err := normalizer.Normalize(event)
	if err != nil {
		return fmt.Errorf("normalizing event: %w", err)
	}

	if result.IsCorrelation {
		return c.correlate(ctx, event, result)
	}

	return c.store.UpsertMessage(ctx, result.Message)
}

// correlate finds an existing pending message and updates it with destination details.
func (c *Correlator) correlate(ctx context.Context, event *decoder.RawEvent, result *normalizer.Result) error {
	existing, err := c.store.FindByCorrelationKey(ctx, result.CorrelationKey)
	if err != nil {
		return fmt.Errorf("finding message for correlation: %w", err)
	}

	if existing == nil {
		c.log.Warn().
			Str("event_type", event.EventType).
			Uint64("nonce", result.CorrelationKey.Nonce).
			Str("sender", result.CorrelationKey.Sender).
			Uint64("src_chain_id", result.CorrelationKey.SrcChainID).
			Msg("No pending message found for correlation, skipping")
		return nil
	}

	err = c.store.UpdateDestination(ctx, existing.MessageID, result.Message.Destination, types.StatusExecuted)
	if err != nil {
		return fmt.Errorf("updating destination for message %s: %w", existing.MessageID, err)
	}

	c.log.Info().
		Str("message_id", existing.MessageID).
		Str("status", string(types.StatusExecuted)).
		Msg("Correlated destination event with existing message")

	return nil
}
