package indexer

import (
	"context"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/correlator"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

type chainReader interface {
	LatestConfirmedBlock(ctx context.Context) (uint64, error)
	FetchLogs(ctx context.Context, fromBlock, toBlock uint64, addresses []common.Address, topics [][]common.Hash) ([]ethtypes.Log, error)
	BlockTimestamp(ctx context.Context, blockNumber uint64) (int64, error)
}

// EventCallback is a function called for each decoded event during processing.
// It is used to plug in correlation, archival, or any other event handling logic.
type EventCallback func(ctx context.Context, event *decoder.RawEvent) error

// BatchCallback runs after all events in a batch have been decoded and processed.
type BatchCallback func(ctx context.Context, events []*decoder.RawEvent) error

// ProcessorHooks allows callers to plug correlation and archival work into the indexing loop.
type ProcessorHooks struct {
	OnEvent    EventCallback
	AfterBatch BatchCallback
}

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
	return processChainInternal(ctx, chainID, protocol, dec, chainClient, cursors, batchSize, pollInterval, log, ProcessorHooks{})
}

// ProcessChainWithCorrelator runs the indexing loop with event correlation enabled.
// Decoded events are passed through the correlator for normalization, DB storage,
// and cross-chain message matching.
func ProcessChainWithCorrelator(
	ctx context.Context,
	chainID uint64,
	protocol string,
	dec decoder.Decoder,
	chainClient *chain.Client,
	cursors CursorStore,
	batchSize uint64,
	pollInterval time.Duration,
	log zerolog.Logger,
	corr *correlator.Correlator,
) error {
	hooks := ProcessorHooks{}
	if corr != nil {
		hooks.OnEvent = corr.ProcessEvent
	}
	return processChainInternal(ctx, chainID, protocol, dec, chainClient, cursors, batchSize, pollInterval, log, hooks)
}

// processChainInternal is the shared implementation for both ProcessChain and ProcessChainWithCorrelator.
func processChainInternal(
	ctx context.Context,
	chainID uint64,
	protocol string,
	dec decoder.Decoder,
	chainClient chainReader,
	cursors CursorStore,
	batchSize uint64,
	pollInterval time.Duration,
	log zerolog.Logger,
	hooks ProcessorHooks,
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

		decodedEvents := make([]*decoder.RawEvent, 0, len(logs))
		blockTimestamps := make(map[uint64]int64)
		var batchErr error

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

			rawEvent.Timestamp, err = blockTimestamp(ctx, chainClient, ethLog.BlockNumber, blockTimestamps)
			if err != nil {
				batchErr = fmt.Errorf("loading block timestamp for block %d: %w", ethLog.BlockNumber, err)
				break
			}

			log.Info().
				Str("event_type", rawEvent.EventType).
				Str("tx_hash", rawEvent.TxHash).
				Uint("log_index", rawEvent.LogIndex).
				Uint64("block_number", rawEvent.BlockNumber).
				Msg("Decoded event")

			decodedEvents = append(decodedEvents, rawEvent)

			// If an event callback is registered, process through it
			// (e.g., correlation, normalization, DB storage)
			if hooks.OnEvent != nil {
				if err := hooks.OnEvent(ctx, rawEvent); err != nil {
					batchErr = fmt.Errorf("processing event %s %s: %w", rawEvent.EventType, rawEvent.TxHash, err)
					break
				}
			}
		}

		if batchErr == nil && hooks.AfterBatch != nil && len(decodedEvents) > 0 {
			if err := hooks.AfterBatch(ctx, decodedEvents); err != nil {
				batchErr = fmt.Errorf("processing archived batch %d-%d: %w", fromBlock, toBlock, err)
			}
		}

		if batchErr != nil {
			log.Error().
				Err(batchErr).
				Uint64("from", fromBlock).
				Uint64("to", toBlock).
				Msg("Batch processing failed, retrying without advancing cursor")
			time.Sleep(pollInterval)
			continue
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

func blockTimestamp(ctx context.Context, chainClient chainReader, blockNumber uint64, cache map[uint64]int64) (int64, error) {
	if timestamp, ok := cache[blockNumber]; ok {
		return timestamp, nil
	}

	timestamp, err := chainClient.BlockTimestamp(ctx, blockNumber)
	if err != nil {
		return 0, err
	}

	cache[blockNumber] = timestamp
	return timestamp, nil
}

// min returns the smaller of two uint64 values.
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
