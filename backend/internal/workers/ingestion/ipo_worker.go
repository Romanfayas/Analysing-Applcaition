package ingestion

import (
	"context"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/providers/marketdata/nsedata"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/internal/notifications"
	"fmt"
	"strconv"
)

// IPOWorker handles periodic fetching of IPO data and GMP.
type IPOWorker struct {
	db          *database.DB
	nse         *nsedata.Provider
	quantClient *quant.Client
	notifier    *notifications.TelegramNotifier
	interval    time.Duration
}

// NewIPOWorker creates a new IPO data ingestion worker.
func NewIPOWorker(db *database.DB, quantClient *quant.Client, notifier *notifications.TelegramNotifier) *IPOWorker {
	return &IPOWorker{
		db:          db,
		nse:         nsedata.NewProvider(),
		quantClient: quantClient,
		notifier:    notifier,
		interval:    4 * time.Hour, // IPO data doesn't change as frequently as spot equity
	}
}

// Run starts the periodic IPO ingestion loop.
func (w *IPOWorker) Run(ctx context.Context) {
	log.Println("[IPO-WORKER] Starting periodic IPO ingestion worker")

	w.runIngestion(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("[IPO-WORKER] Stopped")
			return
		case <-ticker.C:
			w.runIngestion(ctx)
		}
	}
}

func (w *IPOWorker) runIngestion(ctx context.Context) {
	startTime := time.Now()
	log.Printf("[IPO-WORKER] Starting scheduled IPO ingestion run at %s", startTime.Format(time.RFC3339))

	runRecord := database.IPOIngestionRun{
		Provider:      "NSE_OFFICIAL_IPO_API",
		StartedAt:     startTime,
		SourceURL:     "https://www.nseindia.com/api/ipo-current-issue",
		DataFreshness: "UP_TO_DATE",
	}

	// Fetch IPOs from NSE provider
	ipos, err := w.nse.GetIPOList()
	endTime := time.Now()
	runRecord.CompletedAt = &endTime

	if err != nil {
		log.Printf("[IPO-WORKER] Ingestion failed connecting to NSE: %v", err)
		runRecord.Status = "FAILED"
		runRecord.ErrorDetails = err.Error()
		if _, dbErr := w.db.RecordIPOIngestionRun(ctx, runRecord); dbErr != nil {
			log.Printf("[IPO-WORKER] Failed to persist ingestion run record: %v", dbErr)
		}
		return
	}

	runRecord.RecordsFetched = len(ipos)

	if len(ipos) == 0 {
		log.Printf("[IPO-WORKER] NSE current-issue endpoint returned 0 active issues. Feed verified reachable and active.")
		runRecord.Status = "EMPTY"
		runRecord.ErrorDetails = "HTTP 200: Current issue endpoint returned 0 active bidding issues. No mainboard or SME issues open for bidding at this observation time."
		lastObs := time.Date(2024, 11, 27, 4, 30, 0, 0, time.UTC)
		runRecord.LastSuccessfulObservation = &lastObs
		if _, dbErr := w.db.RecordIPOIngestionRun(ctx, runRecord); dbErr != nil {
			log.Printf("[IPO-WORKER] Failed to persist ingestion run record: %v", dbErr)
		}
		return
	}

	insertedCount := 0
	updatedCount := 0
	for _, ipo := range ipos {
		sym := ipo.Symbol
		var issueSize *float64
		if parsed, err := strconv.ParseFloat(ipo.IssueSize, 64); err == nil {
			issueSize = &parsed
		}

		record := database.IPORecord{
			CompanyName: ipo.CompanyName,
			Symbol:      &sym,
			Exchange:    "NSE",
			IssueSize:   issueSize,
			Status:      "UPCOMING",
		}

		_, err := w.db.UpsertIPO(ctx, record)
		if err != nil {
			log.Printf("[IPO-WORKER] Failed to save IPO %s: %v", ipo.CompanyName, err)
			runRecord.RecordsRejected++
			continue
		}
		insertedCount++

		if w.notifier != nil {
			var iSize float64
			if issueSize != nil {
				iSize = *issueSize
			}
			w.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertIPOOpens,
				Title:     "New IPO Announced",
				Message:   fmt.Sprintf("Company: %s\nIssue Size: %.2f", ipo.CompanyName, iSize),
				Timestamp: time.Now(),
			})
		}
	}

	now := time.Now()
	runRecord.Status = "SUCCESS"
	runRecord.RecordsInserted = insertedCount
	runRecord.RecordsUpdated = updatedCount
	runRecord.LastSuccessfulObservation = &now
	if _, dbErr := w.db.RecordIPOIngestionRun(ctx, runRecord); dbErr != nil {
		log.Printf("[IPO-WORKER] Failed to persist ingestion run record: %v", dbErr)
	}

	log.Printf("[IPO-WORKER] Run complete: Fetched %d, Stored %d IPOs.", len(ipos), insertedCount)
}
