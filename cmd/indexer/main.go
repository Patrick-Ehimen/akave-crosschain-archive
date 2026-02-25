package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/logger"
	"github.com/jackc/pgx/v5/pgxpool"
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
	dbpool, err := pgxpool.New(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer dbpool.Close()

	if err := dbpool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("Database ping failed")
	}
	log.Info().Msg("Connected to PostgreSQL")

	// 5. Connect to RPCs (Stub)
	for id, chain := range cfg.Chains {
		log.Info().
			Uint64("chain_id", id).
			Str("rpc_url", chain.RPCURL).
			Uint64("confirmations", chain.ConfirmationDepth).
			Msg("Connecting to chain RPC")

		// TODO: Initialize ethclient.Dial(chain.RPCURL)
		log.Info().Uint64("chain_id", id).Msg("Successfully connected to RPC. Current block height: stubbed")
	}

	// 6. Connect to O3 (Stub)
	log.Info().
		Str("endpoint", cfg.Akave.Endpoint).
		Str("bucket", cfg.Akave.BucketName).
		Msg("Connecting to Akave O3 storage")
	// TODO: Initialize O3 client

	// 7. Stub indexing loop
	log.Info().Msg("Starting indexing loop")

	<-ctx.Done()
	log.Info().Msg("Shutting down CrossChain Archive Indexer cleanly")
}
