package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/api"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/archiver"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/config"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/logger"
	"github.com/Patrick-Ehimen/akave-crosschain-archive/internal/metrics"
	akavestorage "github.com/Patrick-Ehimen/akave-crosschain-archive/internal/storage/akave"
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
	log.Info().Msg("Starting CrossChain Archive API")

	// 3. Register Prometheus metrics
	metrics.Register()

	// 4. Connect to DB
	ctx := context.Background()
	dbpool, err := postgres.NewPool(ctx, cfg.Database.DSN())
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer dbpool.Close()
	log.Info().Msg("Connected to PostgreSQL")

	// 4. HTTP server with chi
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	allowedOrigins := cfg.API.CORSAllowedOrigins
	if len(allowedOrigins) == 0 {
		allowedOrigins = []string{"*"}
	}
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	r.Use(middleware.Timeout(60 * time.Second))

	r.Get("/health", api.HealthHandler(dbpool, api.NewPgCursorQuerier(dbpool)))

	// Message query endpoints
	mq := api.NewPgMessageQuerier(dbpool)
	r.Get("/messages", api.ListMessagesHandler(mq))
	r.Get("/messages/{message_id}", api.GetMessageHandler(mq))
	r.Get("/transactions/{tx_hash}/messages", api.GetTxMessagesHandler(mq))

	r.Get("/address/{address}/history", api.GetAddressHistoryHandler(mq))
	r.Get("/trace/{message_id}", api.GetTraceHandler(mq))

	sq := api.NewPgStatsQuerier(dbpool)
	r.Get("/protocols/{protocol}/stats", api.GetProtocolStatsHandler(sq))
	r.Get("/routes/stats", api.GetRoutesStatsHandler(sq))

	// Historical read-back from Akave O3 (registered only when credentials are set)
	histStore := archiver.NewArchiveStore(dbpool)
	if cfg.Akave.AccessKey != "" && cfg.Akave.SecretKey != "" {
		o3Client, err := akavestorage.NewClient(ctx, cfg.Akave, log)
		if err != nil {
			log.Warn().Err(err).Msg("Akave O3 client init failed — historical endpoints disabled")
		} else {
			r.Get("/historical/index", api.GetHistoricalIndexHandler(histStore))
			r.Get("/historical/messages", api.GetHistoricalMessagesHandler(histStore, o3Client))
			log.Info().Msg("Historical O3 endpoints enabled")
		}
	} else {
		r.Get("/historical/index", api.GetHistoricalIndexHandler(histStore))
		log.Info().Msg("Akave credentials not set — /historical/messages disabled")
	}

	// Prometheus metrics
	r.Get("/metrics", promhttp.Handler().ServeHTTP)

	port := cfg.API.Port
	if port == 0 {
		port = 8080
	}

	readTimeout := cfg.API.ReadTimeout
	if readTimeout == 0 {
		readTimeout = 15 * time.Second
	}

	writeTimeout := cfg.API.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 30 * time.Second
	}

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      r,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
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
