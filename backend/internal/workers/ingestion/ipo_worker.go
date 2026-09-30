package ingestion

import (
	"context"
	"log"
	"time"

	"halal-equity/internal/database"
	"halal-equity/internal/providers/marketdata/nsedata"
	"halal-equity/internal/services/quant"
	"halal-equity/internal/notifications"
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
		record := database.IPORecord{
			CompanyName:   ipo.CompanyName,
			Symbol:        &ipo.Symbol,
			Exchange:      "NSE",
			OpenDate:      &ipo.IssueOpen,
			CloseDate:     &ipo.IssueClose,
			PriceBandLow:  &ipo.PriceBandLow,
			PriceBandHigh: &ipo.PriceBandHigh,
			IssueSize:     &ipo.IssueSize,
			Status:        "UPCOMING",
		}
		
		if ipo.IssueClose.Before(time.Now()) {
			record.Status = "CLOSED"
		}

		_, err := w.db.UpsertIPO(ctx, record)
		if err != nil {
			log.Printf("[IPO-WORKER] Failed to save IPO %s: %v", ipo.CompanyName, err)
			continue
		}
		
		// If it's a new IPO we just tracked, send a notification
		if record.Status == "UPCOMING" && w.notifier != nil {
			w.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertIPOOpens,
				Title:     "New IPO Announced",
				Message:   fmt.Sprintf("Company: %s\nIssue Size: %.2f\nPrice Band: %.2f - %.2f", ipo.CompanyName, ipo.IssueSize, ipo.PriceBandLow, ipo.PriceBandHigh),
				Timestamp: time.Now(),
			})
		}

		successCount++
	}

	log.Printf("[IPO-WORKER] Run complete: Fetched %d, Stored %d IPOs.", len(ipos), successCount)
}
