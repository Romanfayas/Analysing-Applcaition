package ingestion

import (
	"context"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/notifications"
	"github.com/halal-equity/backend/internal/services/market"
)

// Worker runs the ingestion pipeline periodically.
type Worker struct {
	pipeline     *Pipeline
	interval     time.Duration
	lastEODDate  string // IST date (YYYY-MM-DD) of the last completed daily EOD run
}

// NewWorker creates a new ingestion worker.
func NewWorker(db *database.DB, notifier *notifications.TelegramNotifier) *Worker {
	return &Worker{
		pipeline: NewPipeline(db, notifier),
		interval: 15 * time.Minute, // Less aggressive polling to avoid Yahoo 429
	}
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
