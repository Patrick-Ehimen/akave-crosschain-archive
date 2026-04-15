// Command backfill indexes a specific block range for one chain and protocol,
// bypassing the live cursor. Useful for re-indexing after a bug fix or decoder update.
//
// Usage:
//
//	backfill --chain=1 --protocol=layerzero --from=18000000 --to=18001000
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/correlator"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/axelar"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/ccip"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/layerzero"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/wormhole"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/indexer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/logger"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/metrics"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
)

func main() {
	var (
		chainID  = flag.Uint64("chain", 0, "Chain ID to backfill (required)")
		protocol = flag.String("protocol", "", "Protocol to backfill: layerzero, axelar, wormhole, ccip (required)")
		fromFlag = flag.Uint64("from", 0, "First block to index (inclusive, required)")
		toFlag   = flag.Uint64("to", 0, "Last block to index (inclusive, required)")
	)
	flag.Parse()

	if *chainID == 0 || *protocol == "" || *fromFlag == 0 || *toFlag == 0 {
		fmt.Fprintln(os.Stderr, "Usage: backfill --chain=<id> --protocol=<name> --from=<block> --to=<block>")
		flag.PrintDefaults()
		os.Exit(1)
	}
	if *fromFlag > *toFlag {
		fmt.Fprintln(os.Stderr, "--from must be <= --to")
		os.Exit(1)
	}

	cfgPath := os.Getenv("CROSSCHAIN_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Logging.Level, cfg.Logging.Pretty)
	log.Info().
		Uint64("chain_id", *chainID).
		Str("protocol", *protocol).
		Uint64("from", *fromFlag).
		Uint64("to", *toFlag).
		Msg("Starting backfill")

	metrics.Register()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	dbpool, err := postgres.NewPool(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer dbpool.Close()

	chainMgr, err := chain.NewManager(ctx, cfg.Chains, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to chain RPCs")
	}
	defer chainMgr.Close()

	chainClient, err := chainMgr.GetClient(*chainID)
	if err != nil {
		log.Fatal().Err(err).Uint64("chain_id", *chainID).Msg("Chain not configured")
	}

	// Build decoder registry and find the requested protocol
	registry := decoder.NewRegistry()
	for _, d := range []decoder.Decoder{
		layerzero.NewLayerZeroDecoder(),
		axelar.NewAxelarDecoder(),
		wormhole.NewWormholeDecoder(),
		ccip.NewCCIPDecoder(),
	} {
		if err := registry.Register(d); err != nil {
			log.Fatal().Err(err).Str("protocol", d.Protocol()).Msg("Failed to register decoder")
		}
	}

	dec, ok := registry.Get(*protocol)
	if !ok {
		log.Fatal().Str("protocol", *protocol).Msg("Unknown protocol — must be one of: layerzero, axelar, wormhole, ccip")
	}

	messages := indexer.NewPostgresMessageStore(dbpool)
	corr := correlator.New(messages, log)

	// Use a fixed-range cursor store that starts at fromBlock-1 and stops at toBlock.
	rangeCursors := &rangedCursorStore{
		inner:    indexer.NewPostgresCursorStore(dbpool),
		start:    *fromFlag - 1,
		stop:     *toFlag,
		chainID:  *chainID,
		protocol: *protocol,
		done:     make(chan struct{}),
	}

	// Backfill does not need reorg detection (it's idempotent by design).
	err = indexer.ProcessChain(
		ctx,
		*chainID,
		*protocol,
		dec,
		chainClient,
		rangeCursors,
		nil, // no reorg detection for backfill
		corr,
		uint64(cfg.Indexer.BatchSize),
		cfg.Indexer.PollInterval,
		log,
	)
	if err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("Backfill stopped with error")
		os.Exit(1)
	}

	log.Info().Msg("Backfill complete")
}

// rangedCursorStore wraps PostgresCursorStore but overrides the initial cursor
// to the caller-specified start block, and signals completion once the toBlock
// is reached so ProcessChain returns naturally.
type rangedCursorStore struct {
	inner    *indexer.PostgresCursorStore
	start    uint64 // initial cursor value (fromBlock - 1)
	stop     uint64 // stop after this block is processed
	chainID  uint64
	protocol string
	done     chan struct{}
	started  bool
}

func (r *rangedCursorStore) LoadCursor(ctx context.Context, chainID uint64, protocol string) (uint64, error) {
	if !r.started && chainID == r.chainID && protocol == r.protocol {
		r.started = true
		return r.start, nil
	}
	return r.inner.LoadCursor(ctx, chainID, protocol)
}

func (r *rangedCursorStore) UpdateCursor(ctx context.Context, chainID uint64, protocol string, blockNumber uint64) error {
	if chainID == r.chainID && protocol == r.protocol && blockNumber >= r.stop {
		// Signal completion but still persist so this run is idempotent.
		_ = r.inner.UpdateCursor(ctx, chainID, protocol, blockNumber)
		select {
		case <-r.done:
		default:
			close(r.done)
		}
		return context.Canceled // causes ProcessChain to exit cleanly
	}
	return r.inner.UpdateCursor(ctx, chainID, protocol, blockNumber)
}
