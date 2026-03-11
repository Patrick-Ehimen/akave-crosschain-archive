package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

// ProcessChain runs the indexing loop for a single chain and protocol combination.
// It continuously fetches, decodes, and processes events from the specified chain.
func ProcessChain(
	ctx context.Context,
	chainID uint64,
	protocol string,
	dec decoder.Decoder,
	chainClient *chain.Client,
	cursors CursorStore,
	batchSize uint64,
	pollInterval time.Duration,
	log zerolog.Logger,
) error {
	log = log.With().
		Uint64("chain_id", chainID).
		Str("protocol", protocol).
		Logger()

	log.Info().Msg("Starting processor")

	// Load initial cursor
	cursor, err := cursors.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		return fmt.Errorf("loading initial cursor: %w", err)
	}

	log.Info().Uint64("cursor", cursor).Msg("Loaded cursor")

	// Get contract addresses to monitor
	addresses := dec.ContractAddresses(chainID)
	if len(addresses) == 0 {
		log.Warn().Msg("No contract addresses configured for this chain, processor will not fetch events")
		// Still run the loop in case addresses are added later
	}

	// Get event topics to filter
	topics := dec.EventTopics()
	if len(topics) == 0 {
		return fmt.Errorf("decoder returned no event topics")
	}

	// Main indexing loop
	for {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			log.Info().Msg("Processor shutting down")
			return ctx.Err()
		default:
		}

		// Get latest confirmed block
		latestBlock, err := chainClient.LatestConfirmedBlock(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get latest confirmed block, retrying")
			time.Sleep(pollInterval)
			continue
		}

		// Check if we're caught up
		if cursor >= latestBlock {
			log.Debug().
				Uint64("cursor", cursor).
				Uint64("latest", latestBlock).
				Msg("Caught up to chain head, sleeping")
			time.Sleep(pollInterval)
			continue
		}

		// Calculate batch range
		fromBlock := cursor + 1
		toBlock := min(cursor+batchSize, latestBlock)

		// Skip if no addresses to monitor
		if len(addresses) == 0 {
			log.Debug().
				Uint64("from", fromBlock).
				Uint64("to", toBlock).
				Msg("Skipping batch (no addresses configured)")
			cursor = toBlock
			if err := cursors.UpdateCursor(ctx, chainID, protocol, cursor); err != nil {
				log.Error().Err(err).Msg("Failed to update cursor after skipped batch")
				return fmt.Errorf("updating cursor: %w", err)
			}
			continue
		}

		// Fetch logs for this batch
		log.Debug().
			Uint64("from", fromBlock).
			Uint64("to", toBlock).
			Int("addresses", len(addresses)).
			Msg("Fetching logs")

		logs, err := chainClient.FetchLogs(ctx, fromBlock, toBlock, addresses, [][]common.Hash{topics})
		if err != nil {
			log.Error().Err(err).
				Uint64("from", fromBlock).
				Uint64("to", toBlock).
				Msg("Failed to fetch logs, retrying")
			time.Sleep(pollInterval)
			continue
		}

		log.Debug().
			Int("count", len(logs)).
			Uint64("from", fromBlock).
			Uint64("to", toBlock).
			Msg("Fetched logs")

		// Process each log
		for _, ethLog := range logs {
			// Decode the log
			rawEvent, err := dec.Decode(ethLog, chainID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tx_hash", ethLog.TxHash.Hex()).
					Uint("log_index", ethLog.Index).
					Msg("Failed to decode log, skipping")
				continue
			}

			// For now, just log the decoded event
			// In Issue #16, this will be normalized and stored in the database
			log.Info().
				Str("event_type", rawEvent.EventType).
				Str("tx_hash", rawEvent.TxHash).
				Uint("log_index", rawEvent.LogIndex).
				Uint64("block_number", rawEvent.BlockNumber).
				Msg("Decoded event")
		}

		// Update cursor to end of batch
		cursor = toBlock
		if err := cursors.UpdateCursor(ctx, chainID, protocol, cursor); err != nil {
			log.Error().Err(err).Msg("Failed to update cursor")
			return fmt.Errorf("updating cursor: %w", err)
		}

		log.Debug().Uint64("cursor", cursor).Msg("Updated cursor")
	}
}

// min returns the smaller of two uint64 values.
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
