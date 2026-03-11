package correlator

import (
	"context"
	"fmt"

	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/normalizer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/types"
)

// Correlator handles cross-chain message matching by correlating
// source events (PacketSent) with destination events (PacketReceived)
// using GUID or nonce-based matching.
type Correlator struct {
	repo *postgres.MessageRepository
	log  zerolog.Logger
}

// New creates a new Correlator with the provided message repository and logger.
func New(repo *postgres.MessageRepository, log zerolog.Logger) *Correlator {
	return &Correlator{
		repo: repo,
		log:  log.With().Str("component", "correlator").Logger(),
	}
}

// ProcessEvent handles a decoded event by normalizing it and either:
// 1. Creating a new pending message (for PacketSent/OFTSent), or
// 2. Correlating it with an existing pending message and updating to executed (for PacketReceived).
func (c *Correlator) ProcessEvent(ctx context.Context, event *decoder.RawEvent) error {
	if event == nil {
		return fmt.Errorf("nil event")
	}

	switch event.EventType {
	case "PacketSent", "OFTSent":
		return c.handleSourceEvent(ctx, event)
	case "PacketReceived":
		return c.handleDestinationEvent(ctx, event)
	default:
		c.log.Warn().Str("event_type", event.EventType).Msg("Unhandled event type")
		return nil
	}
}

// handleSourceEvent normalizes a PacketSent or OFTSent event
// and creates a new pending message in the database.
func (c *Correlator) handleSourceEvent(ctx context.Context, event *decoder.RawEvent) error {
	msg, err := normalizer.Normalize(event)
	if err != nil {
		return fmt.Errorf("normalizing source event: %w", err)
	}

	if err := c.repo.UpsertMessage(ctx, msg); err != nil {
		return fmt.Errorf("upserting source message: %w", err)
	}

	c.log.Info().
		Str("message_id", msg.MessageID).
		Str("event_type", event.EventType).
		Uint64("chain_id", event.ChainID).
		Str("status", string(msg.Status)).
		Msg("Created pending message")

	return nil
}

// handleDestinationEvent normalizes a PacketReceived event and
// attempts to correlate it with an existing pending message.
// If a matching pending message is found, it is updated to "executed"
// with destination details and latency computation.
func (c *Correlator) handleDestinationEvent(ctx context.Context, event *decoder.RawEvent) error {
	msg, err := normalizer.Normalize(event)
	if err != nil {
		return fmt.Errorf("normalizing destination event: %w", err)
	}

	// Try to find the matching pending message by nonce + source chain + sender.
	srcChainID := msg.Source.ChainID
	sender := msg.Source.Sender
	nonce := uint64(0)
	if msg.Payload != nil {
		nonce = msg.Payload.Nonce
	}

	existing, err := c.repo.FindPendingByNonce(ctx, event.Protocol, srcChainID, sender, nonce)
	if err != nil {
		c.log.Warn().
			Err(err).
			Uint64("src_chain_id", srcChainID).
			Str("sender", sender).
			Uint64("nonce", nonce).
			Msg("No matching pending message found for PacketReceived")
		return nil // Not an error — the source event may not have been indexed yet
	}

	// Compute latency if both timestamps are available.
	var latencySeconds int64
	if existing.Source.Timestamp > 0 && msg.Destination != nil && msg.Destination.Timestamp > 0 {
		latencySeconds = msg.Destination.Timestamp - existing.Source.Timestamp
		if latencySeconds < 0 {
			latencySeconds = 0
		}
	}

	// Update the existing message with destination details.
	if err := c.repo.UpdateDestination(ctx, existing.MessageID, msg.Destination, latencySeconds); err != nil {
		return fmt.Errorf("updating destination for message %s: %w", existing.MessageID, err)
	}

	c.log.Info().
		Str("message_id", existing.MessageID).
		Str("status", string(types.StatusExecuted)).
		Int64("latency_seconds", latencySeconds).
		Msg("Correlated message: pending → executed")

	return nil
}
