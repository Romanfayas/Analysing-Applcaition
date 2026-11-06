package ingestion

import (
	"context"
	"database/sql"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/evidence"
	"github.com/halal-equity/backend/internal/notifications"
	"github.com/halal-equity/backend/internal/providers/corporateactions/official"
	"github.com/halal-equity/backend/internal/services/market"
)

// Worker runs the ingestion pipeline periodically.
type Worker struct {
	pipeline        *Pipeline
	interval        time.Duration
	lastExchangeTry time.Time
	lastExchangeDay string
	lastBackfillDay string
	lastEODDate     string // IST date (YYYY-MM-DD) of the last completed daily EOD run
}

// NewWorker creates a new ingestion worker.
func NewWorker(db *database.DB, notifier *notifications.TelegramNotifier) *Worker {
	return &Worker{
		pipeline: NewPipeline(db, notifier),
		interval: intradayPollInterval(),
	}
}

// intradayPollInterval reads INTRADAY_POLL_MINUTES (1-60, default 3). Shorter intervals make short-horizon
// forecasts fresher but raise the risk of provider rate limiting (HTTP 429) on the free Yahoo feed.
func intradayPollInterval() time.Duration {
	if v, err := strconv.Atoi(os.Getenv("INTRADAY_POLL_MINUTES")); err == nil && v >= 1 && v <= 60 {
		return time.Duration(v) * time.Minute
	}
	return 3 * time.Minute
}

// Run starts the periodic ingestion loop.
func (w *Worker) Run(ctx context.Context) {
	log.Println("[INGEST-WORKER] Starting periodic data ingestion worker")

	// Run initial ingestion on startup
	w.runIngestion(ctx)
	w.maybeRunDailyEOD(ctx)

	// Setup periodic ticker
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[INGEST-WORKER] Stopped")
			return
		case <-ticker.C:
			// Run ingestion frequently during market hours, or once after market close to get EOD data
			session := market.GetCurrentSession(time.Now())
			if session == market.SessionOpen {
				w.runIngestion(ctx)
			} else {
				log.Println("[INGEST-WORKER] Market is closed, checking daily EOD ingestion")
				w.maybeLoadIndex(ctx)
				w.maybeLoadExchangeFiles(ctx)
				w.maybeRunDailyEOD(ctx)
			}
		}
	}
}

// runIngestion executes a single pass of the ingestion pipeline.
func (w *Worker) runIngestion(ctx context.Context) {
	log.Println("[INGEST-WORKER] Starting scheduled ingestion run")

	// Start with the NIFTY 50 universe
	reports := w.pipeline.IngestNIFTY50(ctx)
	reports = append(reports, w.pipeline.IngestUserAdded(ctx)...)

	var totalFetched, totalStored int
	var issues []string

	for _, r := range reports {
		totalFetched += r.CandlesFetched
		totalStored += r.CandlesStored
		if len(r.Issues) > 0 {
			issues = append(issues, r.Issues...)
		}
		if r.Error != "" {
			log.Printf("[INGEST-WORKER] Error for %s: %s", r.Symbol, r.Error)
		}
	}

	log.Printf("[INGEST-WORKER] Run complete: Processed %d symbols, fetched %d, stored %d. %d quality issues detected.",
		len(reports), totalFetched, totalStored, len(issues))
}

// maybeRunDailyEOD runs the daily EOD ingestion once per IST weekday after 16:00 IST (market closes at 15:30 IST).
func (w *Worker) maybeRunDailyEOD(ctx context.Context) {
	ist := time.Now().In(istLoc)
	today := ist.Format("2006-01-02")
	if ist.Weekday() == time.Saturday || ist.Weekday() == time.Sunday || w.lastEODDate == today {
		return
	}
	if ist.Hour() < 16 {
		return
	}
	stored, failed := w.pipeline.IngestDailyEODWithTrigger(ctx, 10, "SCHEDULED")
	if len(failed) == 0 {
		w.lastEODDate = today
		log.Printf("[INGEST-WORKER] Daily EOD ingestion successful: stored %d candles across all active symbols", stored)
	} else {
		log.Printf("[INGEST-WORKER] Daily EOD had %d failures; will retry next tick: %v", len(failed), failed)
	}
}

// maybeLoadIndex loads NSE's official index closes once the day's file is published (it appears after the close,
// later than the bhavcopy), retrying each tick until today's session is stored. A failure never blocks equity ingestion.
func (w *Worker) maybeLoadIndex(ctx context.Context) {
	ist := time.Now().In(istLoc)
	if ist.Weekday() == time.Saturday || ist.Weekday() == time.Sunday || ist.Hour() < 17 {
		return
	}
	have, err := w.pipeline.db.IndexDatesStored(ctx, ist.AddDate(0, 0, -1), ist)
	if err != nil || have[ist.Format("2006-01-02")] {
		return
	}
	if days, _, err := w.pipeline.IngestIndexBars(ctx, ist.AddDate(0, 0, -5), ist); err != nil {
		log.Printf("[INGEST-WORKER] index ingestion: %v", err)
	} else if days > 0 {
		log.Printf("[INGEST-WORKER] index ingestion: stored %d new session(s)", days)
	}
}

// maybeLoadExchangeFiles appends NSE's bhavcopy for recent sessions once it is published (around 18:00-20:00 IST). It runs
// only when EVIDENCE_DATABASE_URL gives a role allowed to write evidence, at most every 30 minutes, and never blocks
// other ingestion. It also refreshes the last few sessions of 5-minute bars, which fill the 15:16-15:28 hole in the
// 1-minute feed.
func (w *Worker) maybeLoadExchangeFiles(ctx context.Context) {
	dsn := os.Getenv("EVIDENCE_DATABASE_URL")
	ist := time.Now().In(istLoc)
	if ist.Weekday() == time.Saturday || ist.Weekday() == time.Sunday || ist.Hour() < 18 || time.Since(w.lastExchangeTry) < 30*time.Minute {
		return
	}
	today := ist.Format("2006-01-02")
	if w.lastExchangeDay == today {
		return
	}
	w.lastExchangeTry = time.Now()
	if w.lastBackfillDay != today {
		w.lastBackfillDay = today
		if n, failed := w.pipeline.RefreshRecentFiveMinute(ctx); len(failed) > 0 {
			log.Printf("[INGEST-WORKER] 5-minute refresh: %d new bars, %d symbols failed", n, len(failed))
		}
	}
	if dsn == "" {
		return
	}
	edb, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Printf("[INGEST-WORKER] exchange files: %v", err)
		return
	}
	defer edb.Close()
	day := time.Date(ist.Year(), ist.Month(), ist.Day(), 0, 0, 0, 0, time.UTC)
	st, err := evidence.LoadBhavcopies(ctx, edb, official.NewPipeline(1200*time.Millisecond), day.AddDate(0, 0, -7), day, false)
	if err != nil {
		log.Printf("[INGEST-WORKER] exchange files: %v", err)
		return
	}
	log.Printf("[INGEST-WORKER] exchange files: loaded=%d present=%d no_session=%d failed=%d", st.Loaded, st.Skipped, st.NoSession, st.Failed)
	var have bool
	if err := edb.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM exchange_source_files WHERE trade_date = $1)`, day).Scan(&have); err == nil && have {
		w.lastExchangeDay = today
	}
}
