// Halal Equity & IPO Research Platform — Background Worker
//
// Processes background jobs: data ingestion, indicator calculation,
// Shariah screening, signal generation, notifications, etc.
package main

import (
	breakoutworker "github.com/halal-equity/backend/internal/workers/breakouts"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/config"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/notifications"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/internal/workers/alerts"
	"github.com/halal-equity/backend/internal/workers/ingestion"
	"github.com/halal-equity/backend/internal/workers/prediction"
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

	// Support on-demand operational EOD ingestion via CLI flag
	if len(os.Args) > 1 && os.Args[1] == "--run-eod" {
		log.Info().Msg("manual EOD run requested via --run-eod")
		pipeline := ingestion.NewPipeline(db, notifier)
		stored, failed := pipeline.IngestDailyEOD(ctx, 10)
		log.Info().Int("stored", stored).Int("failed", len(failed)).Msg("manual EOD run complete")
		if len(failed) > 0 {
			log.Error().Strs("failed_symbols", failed).Msg("EOD ingestion had errors")
			os.Exit(1)
		}
		return
	}

	// Operator command: load NSE's official index history (default ~3 years; idempotent, skips stored days)
	if len(os.Args) > 1 && os.Args[1] == "--backfill-index" {
		days := 1100
		if len(os.Args) > 2 {
			if n, err := strconv.Atoi(os.Args[2]); err == nil && n > 0 {
				days = n
			}
		}
		pipeline := ingestion.NewPipeline(db, notifier)
		n, rows, err := pipeline.IngestIndexBars(ctx, time.Now().AddDate(0, 0, -days), time.Now())
		log.Info().Int("days_loaded", n).Int("rows", rows).Msg("index backfill complete")
		if err != nil {
			log.Error().Err(err).Msg("index backfill stopped early; re-run to continue")
			os.Exit(1)
		}
		return
	}

	// Operator command: load ~60 trading days of 5-minute history (idempotent; never overwrites existing bars)
	if len(os.Args) > 1 && os.Args[1] == "--backfill-5m" {
		log.Info().Msg("5-minute history backfill requested via --backfill-5m")
		pipeline := ingestion.NewPipeline(db, notifier)
		stored, failed := pipeline.BackfillFiveMinute(ctx)
		log.Info().Int("stored", stored).Int("failed", len(failed)).Msg("5-minute backfill complete")
		if len(failed) > 0 {
			log.Error().Strs("failed_symbols", failed).Msg("backfill had errors; re-run to retry the failed symbols")
			os.Exit(1)
		}
		return
	}

	// Initialize and run the market data ingestion worker
	marketWorker := ingestion.NewWorker(db, notifier)
	go marketWorker.Run(ctx)

	// Initialize and run IPO ingestion worker
	ipoWorker := ingestion.NewIPOWorker(db, nil, notifier)
	go ipoWorker.Run(ctx)

	// Intraday prediction engine: forecasts during the session, outcomes after each horizon, daily re-validation.
	go prediction.NewWorker(db, quant.NewClient(cfg.QuantEngineURL), 3*time.Minute).Run(ctx)

	// Record every confirmed breakout and follow what happens next
	go breakoutworker.NewWorker(db, quant.NewClient(cfg.QuantEngineURL), 10*time.Minute).Run(ctx)

	// Evaluate user price alerts against the latest stored prices
	go alerts.NewEvaluator(db, notifier, 5*time.Minute).Run(ctx)

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
