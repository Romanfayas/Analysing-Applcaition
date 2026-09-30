package ingestion

import (
	"context"
	"log"
	"time"

	"halal-equity/internal/database"
	"halal-equity/internal/notifications"
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
		interval: 24 * time.Hour, // Daily ingestion is sufficient for EOD spot equity analysis
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

	for {
		select {
		case <-ctx.Done():
			log.Println("[INGEST-WORKER] Stopped")
			return
		case <-ticker.C:
			w.runIngestion(ctx)
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
	}
	
	log.Printf("[INGEST-WORKER] Run complete: Processed %d symbols, fetched %d, stored %d. %d quality issues detected.", 
		len(reports), totalFetched, totalStored, len(issues))
}
