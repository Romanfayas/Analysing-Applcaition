package ingestion

import (
	"context"
	"log"
	"time"

	"fmt"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/notifications"
	"github.com/halal-equity/backend/internal/providers/ipowatch"
	"github.com/halal-equity/backend/internal/providers/marketdata/nsedata"
	"github.com/halal-equity/backend/internal/providers/marketdata/yahoofinance"
	"github.com/halal-equity/backend/internal/services/quant"
)

// IPOWorker handles periodic fetching of IPO data and GMP.
type IPOWorker struct {
	db          *database.DB
	nse         *nsedata.Provider
	watch       *ipowatch.Client
	yahoo       *yahoofinance.Provider
	quantClient *quant.Client
	notifier    *notifications.TelegramNotifier
	interval    time.Duration
}

// NewIPOWorker creates a new IPO data ingestion worker.
func NewIPOWorker(db *database.DB, quantClient *quant.Client, notifier *notifications.TelegramNotifier) *IPOWorker {
	return &IPOWorker{
		db:          db,
		nse:         nsedata.NewProvider(),
		watch:       ipowatch.NewClient(),
		yahoo:       yahoofinance.NewProvider(),
		quantClient: quantClient,
		notifier:    notifier,
		interval:    time.Hour, // subscription changes through each bidding day; hourly keeps it current without hammering NSE
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
		// A failed or quiet current-issue feed must not stop listings, GMP and lifecycle from updating.
		w.ingestPastIssues(ctx)
		w.enrich(ctx)
		return
	}

	runRecord.RecordsFetched = len(ipos)

	parsed, rejected := nsedata.ParseIPOIssues(ipos, time.Now())
	for _, rej := range rejected {
		log.Printf("[IPO-WORKER] Rejected feed record: %v", rej)
	}
	runRecord.RecordsRejected = len(rejected)

	if len(ipos) == 0 {
		now := time.Now()
		log.Printf("[IPO-WORKER] NSE current-issue endpoint returned 0 active issues. Feed verified reachable and active.")
		runRecord.Status = "EMPTY"
		runRecord.ErrorDetails = "HTTP 200: Current issue endpoint returned 0 active bidding issues. No mainboard or SME issues open for bidding at this observation time."
		runRecord.LastSuccessfulObservation = &now
		if _, dbErr := w.db.RecordIPOIngestionRun(ctx, runRecord); dbErr != nil {
			log.Printf("[IPO-WORKER] Failed to persist ingestion run record: %v", dbErr)
		}
		// Listings arrive through the past-issues feed, so a quiet day must still refresh it.
		w.ingestPastIssues(ctx)
		w.enrich(ctx)
		return
	}

	insertedCount := 0
	updatedCount := 0
	source := "NSE_OFFICIAL_IPO_API"
	for _, ipo := range parsed {
		sym := ipo.Symbol
		board := ipo.Board
		var boardPtr *string
		if board != "" {
			boardPtr = &board
		}
		record := database.IPORecord{
			Board:         boardPtr,
			CompanyName:   ipo.CompanyName,
			Symbol:        &sym,
			Exchange:      "NSE",
			OpenDate:      ipo.OpenDate,
			CloseDate:     ipo.CloseDate,
			PriceBandLow:  ipo.PriceBandLow,
			PriceBandHigh: ipo.PriceBandHigh,
			IssueSize:     ipo.IssueSizeCrore,
			Status:        ipo.Status,
			Source:        &source,
		}

		ipoID, isNew, err := w.db.UpsertIPOReportingInsert(ctx, record)
		if err != nil {
			log.Printf("[IPO-WORKER] Failed to save IPO %s: %v", ipo.CompanyName, err)
			runRecord.RecordsRejected++
			continue
		}
		w.recordSubscription(ctx, ipoID, ipo)
		if !isNew {
			updatedCount++
			continue
		}
		insertedCount++

		// Announce each IPO once, when first seen. Re-sending on every refresh was alert spam.
		if w.notifier != nil {
			msg := fmt.Sprintf("Company: %s", ipo.CompanyName)
			if ipo.IssueSizeCrore != nil {
				msg += fmt.Sprintf("\nIssue size: Rs.%.2f crore", *ipo.IssueSizeCrore)
			}
			w.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertIPOOpens,
				Title:     "New IPO Announced",
				Message:   msg,
				Symbol:    ipo.Symbol,
				Timestamp: time.Now(),
			})
		}
	}

	w.ingestPastIssues(ctx)
	w.enrich(ctx)

	now := time.Now()
	runRecord.Status = "SUCCESS"
	runRecord.RecordsInserted = insertedCount
	runRecord.RecordsUpdated = updatedCount
	runRecord.LastSuccessfulObservation = &now
	if _, dbErr := w.db.RecordIPOIngestionRun(ctx, runRecord); dbErr != nil {
		log.Printf("[IPO-WORKER] Failed to persist ingestion run record: %v", dbErr)
	}

	log.Printf("[IPO-WORKER] Run complete: fetched %d, new %d, refreshed %d, rejected %d.", len(ipos), insertedCount, updatedCount, runRecord.RecordsRejected)
}

// recordSubscription stores the exchange's current demand for an issue. Mainboard issues get per-category multiples
// from /ipo-detail; SME issues publish only a total multiple (from the list feed), so categories stay empty.
func (w *IPOWorker) recordSubscription(ctx context.Context, ipoID int, ipo nsedata.ParsedIPO) {
	snap := database.IPOSubscriptionSnapshot{Total: ipo.SubscriptionTimes, Source: "NSE_IPO_API"}
	if ipo.Board == "MAINBOARD" {
		series := "EQ"
		if d, err := w.nse.GetIPOFullDetail(ipo.Symbol, series); err != nil {
			log.Printf("[IPO-WORKER] %s: bid details unavailable: %v", ipo.Symbol, err)
		} else {
			s := d.Subscription
			snap.QIB, snap.NII, snap.Retail = s.QIB, s.NII, s.Retail
			if s.Total != nil {
				snap.Total = s.Total
			}
			snap.Source = "NSE_IPO_DETAIL"
		}
	}
	if _, err := w.db.RecordIPOSubscription(ctx, ipoID, ipo.OpenDate, time.Now(), snap); err != nil {
		log.Printf("[IPO-WORKER] %s: subscription not stored: %v", ipo.Symbol, err)
	}
}

// ingestPastIssues adds recently closed and listed issues from NSE (with the exchange-announced listing dates), so
// an issue does not disappear from the app the moment its bidding closes.
func (w *IPOWorker) ingestPastIssues(ctx context.Context) {
	raw, err := w.nse.GetPastIssues()
	if err != nil {
		log.Printf("[IPO-WORKER] Past issues unavailable: %v", err)
		return
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	source := "NSE_OFFICIAL_IPO_API"
	stored := 0
	for _, p := range nsedata.ParsePastIssues(raw, now, 45*24*time.Hour) {
		status := "CLOSED_AWAITING_LISTING"
		if p.ListingDate != nil && !p.ListingDate.After(today) {
			status = "LISTED"
		}
		sym, board := p.Symbol, p.Board
		rec := database.IPORecord{
			CompanyName: p.CompanyName, Symbol: &sym, Exchange: "NSE", Board: &board, Source: &source, Status: status,
			OpenDate: p.OpenDate, CloseDate: p.CloseDate, ListingDate: p.ListingDate,
			PriceBandLow: p.PriceBandLow, PriceBandHigh: p.PriceBandHigh, IssuePrice: p.IssuePrice,
		}
		id, _, err := w.db.UpsertIPOReportingInsert(ctx, rec)
		if err != nil {
			log.Printf("[IPO-WORKER] Past issue %s not stored: %v", p.Symbol, err)
			continue
		}
		stored++
		if p.Board == "MAINBOARD" && now.Sub(*p.CloseDate) < 7*24*time.Hour {
			if d, err := w.nse.GetIPOFullDetail(p.Symbol, "EQ"); err == nil {
				s := d.Subscription
				_, _ = w.db.RecordIPOSubscription(ctx, id, p.OpenDate, now, database.IPOSubscriptionSnapshot{
					QIB: s.QIB, NII: s.NII, Retail: s.Retail, Total: s.Total, Source: "NSE_IPO_DETAIL"})
			}
		}
	}
	log.Printf("[IPO-WORKER] Past issues: %d recent mainboard/SME issues refreshed", stored)
}
