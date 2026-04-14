package indexer

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/correlator"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/metrics"
)

// reorgPruneDepth is how many older block hash records we keep around for
// detecting deep reorgs. Should be at least the chain's confirmation_depth.
const reorgPruneDepth = 256

// ProcessChain runs the indexing loop for a single chain and protocol combination.
// It continuously fetches, decodes, normalizes, and correlates events from the chain.
// On each batch it validates parent hash continuity to detect chain reorganizations.
func ProcessChain(
	ctx context.Context,
	chainID uint64,
	protocol string,
	dec decoder.Decoder,
	chainClient *chain.Client,
	cursors CursorStore,
	blockHashes BlockHashStore,
	corr *correlator.Correlator,
	batchSize uint64,
	pollInterval time.Duration,
	log zerolog.Logger,
) error {
	log = log.With().
		Uint64("chain_id", chainID).
		Str("protocol", protocol).
		Logger()

	chainIDStr := strconv.FormatUint(chainID, 10)

	log.Info().Msg("Starting processor")

	cursor, err := cursors.LoadCursor(ctx, chainID, protocol)
	if err != nil {
		return fmt.Errorf("loading initial cursor: %w", err)
	}
	log.Info().Uint64("cursor", cursor).Msg("Loaded cursor")

	addresses := dec.ContractAddresses(chainID)
	if len(addresses) == 0 {
		log.Warn().Msg("No contract addresses configured for this chain, processor will not fetch events")
	}

	topics := dec.EventTopics()
	if len(topics) == 0 {
		return fmt.Errorf("decoder returned no event topics")
	}

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Processor shutting down")
			return ctx.Err()
		default:
		}

		latestBlock, err := chainClient.LatestConfirmedBlock(ctx)
		if err != nil {
			log.Error().Err(err).Msg("Failed to get latest confirmed block, retrying")
			time.Sleep(pollInterval)
			continue
		}

		// Update lag metric
		if latestBlock > cursor {
			metrics.ProcessorLagBlocks.WithLabelValues(chainIDStr, protocol).Set(float64(latestBlock - cursor))
		} else {
			metrics.ProcessorLagBlocks.WithLabelValues(chainIDStr, protocol).Set(0)
		}

		if cursor >= latestBlock {
			log.Debug().
				Uint64("cursor", cursor).
				Uint64("latest", latestBlock).
				Msg("Caught up to chain head, sleeping")
			time.Sleep(pollInterval)
			continue
		}

		fromBlock := cursor + 1
		toBlock := min(cursor+batchSize, latestBlock)

		// Reorg detection: validate parent hash of fromBlock against stored hash.
		rewound, err := checkAndHandleReorg(ctx, chainID, protocol, fromBlock, chainClient, cursors, blockHashes, log)
		if err != nil {
			log.Error().Err(err).Uint64("from_block", fromBlock).Msg("Reorg check failed, retrying")
			time.Sleep(pollInterval)
			continue
		}
		if rewound {
			// Cursor was rewound; reload and restart the iteration.
			cursor, err = cursors.LoadCursor(ctx, chainID, protocol)
			if err != nil {
				return fmt.Errorf("reloading cursor after reorg rewind: %w", err)
			}
			metrics.ReorgsDetectedTotal.WithLabelValues(chainIDStr, protocol).Inc()
			continue
		}

		// Skip if no addresses to monitor
		if len(addresses) == 0 {
			log.Debug().
				Uint64("from", fromBlock).
				Uint64("to", toBlock).
				Msg("Skipping batch (no addresses configured)")
			cursor = toBlock
			if err := cursors.UpdateCursor(ctx, chainID, protocol, cursor); err != nil {
				return fmt.Errorf("updating cursor: %w", err)
			}
			continue
		}

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

		blockTimestamps := make(map[uint64]int64)

		for _, ethLog := range logs {
			rawEvent, err := dec.Decode(ethLog, chainID)
			if err != nil {
				log.Warn().
					Err(err).
					Str("tx_hash", ethLog.TxHash.Hex()).
					Uint("log_index", ethLog.Index).
					Msg("Failed to decode log, skipping")
				continue
			}

			if _, cached := blockTimestamps[rawEvent.BlockNumber]; !cached {
				ts, err := chainClient.BlockTimestamp(ctx, rawEvent.BlockNumber)
				if err != nil {
					log.Warn().Err(err).Uint64("block", rawEvent.BlockNumber).Msg("Failed to get block timestamp")
				} else {
					blockTimestamps[rawEvent.BlockNumber] = ts
				}
			}
			rawEvent.Timestamp = blockTimestamps[rawEvent.BlockNumber]

			if err := corr.Process(ctx, rawEvent); err != nil {
				log.Warn().
					Err(err).
					Str("event_type", rawEvent.EventType).
					Str("tx_hash", rawEvent.TxHash).
					Uint("log_index", rawEvent.LogIndex).
					Msg("Failed to process event, skipping")
				continue
			}

			metrics.IndexedMessagesTotal.WithLabelValues(chainIDStr, protocol, rawEvent.EventType).Inc()

			log.Info().
				Str("event_type", rawEvent.EventType).
				Str("tx_hash", rawEvent.TxHash).
				Uint("log_index", rawEvent.LogIndex).
				Uint64("block_number", rawEvent.BlockNumber).
				Msg("Processed event")
		}

		// Store block hashes for the batch boundary blocks for future reorg checks.
		if blockHashes != nil {
			if err := storeBlockHashes(ctx, chainID, fromBlock, toBlock, chainClient, blockHashes, log); err != nil {
				log.Warn().Err(err).Msg("Failed to store block hashes (non-fatal)")
			}
			// Prune old records to bound table growth.
			if toBlock > reorgPruneDepth {
				if err := blockHashes.PruneBlockHashes(ctx, chainID, toBlock-reorgPruneDepth); err != nil {
					log.Warn().Err(err).Msg("Failed to prune block hashes (non-fatal)")
				}
			}
		}

		cursor = toBlock
		if err := cursors.UpdateCursor(ctx, chainID, protocol, cursor); err != nil {
			return fmt.Errorf("updating cursor: %w", err)
		}

		metrics.IndexedBlocksTotal.WithLabelValues(chainIDStr, protocol).Add(float64(toBlock - fromBlock + 1))

		log.Debug().Uint64("cursor", cursor).Msg("Updated cursor")
	}
}

// checkAndHandleReorg fetches the header for fromBlock, compares its parentHash
// against our stored record for (fromBlock-1), and rewinds the cursor if a fork
// is detected. Returns true if a reorg was handled (cursor rewound).
func checkAndHandleReorg(
	ctx context.Context,
	chainID uint64,
	protocol string,
	fromBlock uint64,
	chainClient *chain.Client,
	cursors CursorStore,
	blockHashes BlockHashStore,
	log zerolog.Logger,
) (rewound bool, err error) {
	if blockHashes == nil || fromBlock == 0 {
		return false, nil
	}

	// No stored parent to compare against for the very first block.
	storedHash, _, err := blockHashes.GetBlockHash(ctx, chainID, fromBlock-1)
	if err != nil {
		return false, fmt.Errorf("getting stored block hash: %w", err)
	}
	if storedHash == "" {
		// No record yet — skip reorg check for this batch.
		return false, nil
	}

	// Fetch the live header for fromBlock to get its parentHash.
	header, err := chainClient.BlockHeader(ctx, fromBlock)
	if err != nil {
		return false, fmt.Errorf("fetching header for block %d: %w", fromBlock, err)
	}

	liveParentHash := header.ParentHash.Hex()
	if liveParentHash == storedHash {
		return false, nil // chain is continuous, no reorg
	}

	// Reorg detected: find common ancestor by walking backwards.
	log.Warn().
		Uint64("block", fromBlock).
		Str("stored_parent", storedHash).
		Str("live_parent", liveParentHash).
		Msg("Reorg detected — rewinding cursor to common ancestor")

	rewindTo := fromBlock - 1
	for rewindTo > 0 {
		stored, _, err := blockHashes.GetBlockHash(ctx, chainID, rewindTo)
		if err != nil || stored == "" {
			break // can't go further back; rewind to this point
		}
		h, err := chainClient.BlockHeader(ctx, rewindTo)
		if err != nil {
			break
		}
		if h.Hash().Hex() == stored {
			break // found common ancestor
		}
		rewindTo--
	}

	log.Warn().Uint64("rewind_to", rewindTo).Msg("Rewinding cursor")

	if err := cursors.UpdateCursor(ctx, chainID, protocol, rewindTo); err != nil {
		return false, fmt.Errorf("updating cursor for reorg rewind: %w", err)
	}
	if err := blockHashes.DeleteBlockHashesFrom(ctx, chainID, rewindTo+1); err != nil {
		log.Warn().Err(err).Msg("Failed to delete stale block hashes after reorg (non-fatal)")
	}

	return true, nil
}

// storeBlockHashes persists the hash and parent hash for each block in [from, to].
// Only stores the boundary blocks to keep writes bounded — for very large ranges
// the intermediate blocks are skipped.
func storeBlockHashes(
	ctx context.Context,
	chainID uint64,
	fromBlock, toBlock uint64,
	chainClient *chain.Client,
	store BlockHashStore,
	log zerolog.Logger,
) error {
	blocksToStore := []uint64{fromBlock, toBlock}
	seen := map[uint64]bool{}
	for _, b := range blocksToStore {
		if seen[b] {
			continue
		}
		seen[b] = true
		header, err := chainClient.BlockHeader(ctx, b)
		if err != nil {
			return fmt.Errorf("fetching header for block %d: %w", b, err)
		}
		if err := store.SaveBlockHash(ctx, BlockHashRecord{
			ChainID:     chainID,
			BlockNumber: b,
			BlockHash:   header.Hash().Hex(),
			ParentHash:  header.ParentHash.Hex(),
		}); err != nil {
			return err
		}
	}
	return nil
}

// min returns the smaller of two uint64 values.
func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
