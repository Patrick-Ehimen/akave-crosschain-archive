package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/chain"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/decoder/layerzero"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/indexer"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/logger"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/postgres"
)

func main() {
	// 1. Load config
	cfgPath := os.Getenv("CROSSCHAIN_CONFIG")
	if cfgPath == "" {
		cfgPath = "configs/config.yaml"
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 2. Init logger
	log := logger.New(cfg.Logging.Level, cfg.Logging.Pretty)
	log.Info().Msg("Starting CrossChain Archive Indexer")

	// 3. Setup context with graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	// 4. Connect to DB
	dbpool, err := postgres.NewPool(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer dbpool.Close()
	log.Info().Msg("Connected to PostgreSQL")

	// 5. Connect to RPCs
	chainMgr, err := chain.NewManager(ctx, cfg.Chains, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to chain RPCs")
	}
	defer chainMgr.Close()

	// 6. Init Decoder Registry
	registry := decoder.NewRegistry()
	lzDecoder := layerzero.NewLayerZeroDecoder()
	if err := registry.Register(lzDecoder); err != nil {
		log.Fatal().Err(err).Msg("Failed to register LayerZero decoder")
	}
	log.Info().Strs("protocols", registry.Protocols()).Msg("Decoder registry initialized")

	// 7. Connect to Akave O3
	o3Client, err := akave.NewClient(ctx, cfg.Akave, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Akave O3 storage")
	}

	// 8. Build chain name map from config
	chainNames := make(map[uint64]string, len(cfg.Chains))
	for chainID, chainCfg := range cfg.Chains {
		chainNames[chainID] = chainCfg.Name
	}

	// 9. Initialize cursor store
	cursors := indexer.NewPostgresCursorStore(dbpool)

	// 10. Create and start indexer service
	idx := indexer.New(chainMgr, registry, cursors, dbpool, o3Client, chainNames, cfg.Indexer, log)

	if err := idx.Start(ctx); err != nil && err != context.Canceled {
		log.Error().Err(err).Msg("Indexer stopped with error")
	}

	log.Info().Msg("Shutting down CrossChain Archive Indexer cleanly")
}
