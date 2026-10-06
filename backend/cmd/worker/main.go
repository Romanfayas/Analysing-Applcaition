// Halal Equity & IPO Research Platform — Background Worker
//
// Processes background jobs: data ingestion, indicator calculation,
// Shariah screening, signal generation, notifications, etc.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/config"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/notifications"
	"github.com/halal-equity/backend/internal/workers/ingestion"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	setupLogger(cfg)
	log.Info().Str("env", cfg.Env).Msg("starting background worker")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Info().Msg("shutting down worker...")
		cancel()
	}()

	// Initialize Database
	db, err := database.NewDB(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer db.Close()

	// Initialize Telegram Notifier (optional)
	var notifier *notifications.TelegramNotifier
	if cfg.TelegramBotToken != "" {
		notifier = notifications.NewTelegramNotifier(cfg.TelegramBotToken, cfg.TelegramChatID)
	}

	// Initialize and run the market data ingestion worker
	marketWorker := ingestion.NewWorker(db, notifier)
	go marketWorker.Run(ctx)
	
	// Initialize and run IPO ingestion worker
	// NOTE: quant client is needed for IPO Worker (pass nil or a valid client if available)
	// ipoWorker := ingestion.NewIPOWorker(db, nil, notifier)
	// go ipoWorker.Run(ctx)

	log.Info().Msg("worker running, waiting for jobs...")

	// Block until context is cancelled
	<-ctx.Done()
	log.Info().Msg("worker stopped")
}

func setupLogger(cfg *config.Config) {
	level, err := zerolog.ParseLevel(cfg.LogLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	if cfg.IsDevelopment() {
		log.Logger = log.Output(zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		})
	}

	log.Logger = log.Logger.With().Str("service", "worker").Caller().Logger()
}
