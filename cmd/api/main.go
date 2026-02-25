package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/logger"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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
	log.Info().Msg("Starting CrossChain Archive API")

	// 3. Connect to DB
	ctx := context.Background()
	dbpool, err := pgxpool.New(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer dbpool.Close()

	if err := dbpool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("Database ping failed")
	}
	log.Info().Msg("Connected to PostgreSQL")

	// 4. Stub HTTP server with chi
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	server := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// 5. Graceful shutdown
	go func() {
		log.Info().Str("addr", server.Addr).Msg("HTTP server listening")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server error")
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	sig := <-sigCh
	log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("HTTP server shutdown error")
	}
	log.Info().Msg("Shutting down CrossChain Archive API cleanly")
}
