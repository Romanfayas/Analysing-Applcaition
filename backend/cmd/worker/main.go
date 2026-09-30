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

	// TODO: Initialize Redis queue consumer
	// TODO: Register job handlers:
	//   - market_data_ingest: Fetch OHLCV from providers
	//   - calculate_indicators: Run technical indicator calculations
	//   - run_shariah_screening: Execute Shariah screening
	//   - generate_signals: Calculate composite signals
	//   - send_notifications: Dispatch alerts via Telegram/email/push
	//   - run_backtest: Execute backtest strategies
	//   - ipo_data_ingest: Fetch IPO data
	//   - data_quality_check: Run data quality validation

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
