package indexer

import (
	"context"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
)

// Indexer orchestrates the indexing of cross-chain bridge events across multiple
// chains and protocols. It spawns a processor goroutine for each (chain, protocol)
// combination and manages their lifecycle.
type Indexer struct {
	chainMgr *chain.Manager
	registry *decoder.Registry
	cursors  CursorStore
	pool     *pgxpool.Pool
	config   config.Indexer
	log      zerolog.Logger
}

// New creates a new Indexer instance with the provided dependencies.
func New(
	chainMgr *chain.Manager,
	registry *decoder.Registry,
	cursors CursorStore,
	pool *pgxpool.Pool,
	cfg config.Indexer,
	log zerolog.Logger,
) *Indexer {
	return &Indexer{
		chainMgr: chainMgr,
		registry: registry,
		cursors:  cursors,
		pool:     pool,
		config:   cfg,
		log:      log.With().Str("component", "indexer").Logger(),
	}
}

// Start begins the indexing loop. It spawns a goroutine for each (chain, protocol)
// combination and blocks until the context is cancelled. All processors complete
// their current batch before shutdown.
func (idx *Indexer) Start(ctx context.Context) error {
	idx.log.Info().Msg("Starting indexing loop")

	// Get all registered decoders
	decoders := idx.registry.All()
	if len(decoders) == 0 {
		return fmt.Errorf("no decoders registered")
	}

	idx.log.Info().
		Int("decoder_count", len(decoders)).
		Strs("protocols", idx.registry.Protocols()).
		Msg("Decoders loaded")

	// WaitGroup to track all processor goroutines
	var wg sync.WaitGroup

	// Spawn a processor for each (chain, protocol) combination
	for _, dec := range decoders {
		protocol := dec.Protocol()

		// Get all chains this decoder supports
		chains := idx.chainMgr.AllClients()
		if len(chains) == 0 {
			idx.log.Warn().Str("protocol", protocol).Msg("No chains configured")
			continue
		}

		for chainID := range chains {
			// Check if decoder supports this chain
			addresses := dec.ContractAddresses(chainID)
			// Note: addresses might be empty, but we still start the processor
			// in case addresses are added later or for other initialization

			// Get chain client
			chainClient, err := idx.chainMgr.GetClient(chainID)
			if err != nil {
				idx.log.Error().
					Err(err).
					Uint64("chain_id", chainID).
					Str("protocol", protocol).
					Msg("Failed to get chain client, skipping")
				continue
			}

			// Spawn processor goroutine
			wg.Add(1)
			go func(chainID uint64, protocol string, dec decoder.Decoder, client *chain.Client) {
				defer wg.Done()

				err := ProcessChain(
					ctx,
					chainID,
					protocol,
					dec,
					client,
					idx.cursors,
					uint64(idx.config.BatchSize),
					idx.config.PollInterval,
					idx.log,
				)

				if err != nil && err != context.Canceled {
					idx.log.Error().
						Err(err).
						Uint64("chain_id", chainID).
						Str("protocol", protocol).
						Msg("Processor stopped with error")
				}
			}(chainID, protocol, dec, chainClient)

			idx.log.Info().
				Uint64("chain_id", chainID).
				Str("protocol", protocol).
				Int("addresses", len(addresses)).
				Msg("Started processor")
		}
	}

	// Wait for all processors to finish (on context cancellation)
	wg.Wait()

	idx.log.Info().Msg("All processors stopped, indexer shutting down")
	return nil
}
