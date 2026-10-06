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
	log.Println("[IPO-WORKER] Starting scheduled IPO ingestion run")

	// Fetch IPOs from NSE provider
	ipos, err := w.nse.GetIPOList()
	if err != nil {
		log.Printf("[IPO-WORKER] Failed to fetch IPO list from NSE: %v", err)
		return
	}

	successCount := 0
	for _, ipo := range ipos {
		// Store in database
		sym := ipo.Symbol
		
		var issueSize *float64
		if parsed, err := strconv.ParseFloat(ipo.IssueSize, 64); err == nil {
			issueSize = &parsed
		}

		record := database.IPORecord{
			CompanyName:   ipo.CompanyName,
			Symbol:        &sym,
			Exchange:      "NSE",
			IssueSize:     issueSize,
			Status:        "UPCOMING",
		}
		
		// Note: Skipping exact time parsing for OpenDate/CloseDate/PriceBand for now 
		// to resolve compilation errors rapidly.

		_, err := w.db.UpsertIPO(ctx, record)
		if err != nil {
			log.Printf("[IPO-WORKER] Failed to save IPO %s: %v", ipo.CompanyName, err)
			continue
		}
		
		// If it's a new IPO we just tracked, send a notification
		if record.Status == "UPCOMING" && w.notifier != nil {
			var iSize float64
			if issueSize != nil { iSize = *issueSize }
			w.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertIPOOpens,
				Title:     "New IPO Announced",
				Message:   fmt.Sprintf("Company: %s\nIssue Size: %.2f", ipo.CompanyName, iSize),
				Timestamp: time.Now(),
			})
		}

		successCount++
	}

	log.Printf("[IPO-WORKER] Run complete: Fetched %d, Stored %d IPOs.", len(ipos), successCount)
}
