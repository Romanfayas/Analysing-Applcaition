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
	pipeline *Pipeline
	interval time.Duration
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

	// Setup periodic ticker
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	log.Println("[INGEST-WORKER] Performing initial startup ingestion...")
	w.runIngestion(ctx)

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
				// We can optionally run a check here to fetch EOD data if missed, but for simplicity:
				log.Println("[INGEST-WORKER] Market is closed, skipping ingestion")
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
