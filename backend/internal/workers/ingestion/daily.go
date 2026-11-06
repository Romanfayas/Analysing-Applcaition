package ingestion

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/marketdata"
)

var istLoc = time.FixedZone("IST", 5*3600+30*60)

// dailyStamp returns the canonical ohlcv_daily timestamp for an IST session date: 09:15 IST (03:45 UTC).
func dailyStamp(t time.Time) time.Time {
	d := t.In(istLoc)
	return time.Date(d.Year(), d.Month(), d.Day(), 9, 15, 0, 0, istLoc).UTC()
}

// sessionComplete reports whether the daily bar for IST date d is final at time now (>= 16:00 IST on or after d).
func sessionComplete(d time.Time, now time.Time) bool {
	di, ni := d.In(istLoc), now.In(istLoc)
	dd := time.Date(di.Year(), di.Month(), di.Day(), 0, 0, 0, 0, istLoc)
	nd := time.Date(ni.Year(), ni.Month(), ni.Day(), 0, 0, 0, 0, istLoc)
	if dd.Before(nd) {
		return true
	}
	return dd.Equal(nd) && ni.Hour() >= 16
}

// IngestDailyEOD runs EOD daily ingestion with default "MANUAL" trigger.
func (p *Pipeline) IngestDailyEOD(ctx context.Context, lookbackDays int) (stored int, failed []string) {
	return p.IngestDailyEODWithTrigger(ctx, lookbackDays, "MANUAL")
}

// IngestDailyEODWithTrigger fetches completed end-of-day bars for active symbols with daily history.
// Features:
// 1. Records durable execution runs in eod_ingestion_runs table.
// 2. Bounded retries (up to 2 retries per symbol on network/transient error).
// 3. Degraded status reporting (PARTIAL_DEGRADED if some symbols fail, FAILED if all fail).
// 4. Clear provenance separation: provider is marked unverified (is_exchange_verified=false)
//    until official exchange bhavcopy reconciliation is performed.
// 5. Never writes the in-progress session and never synthesizes phantom candles.
func (p *Pipeline) IngestDailyEODWithTrigger(ctx context.Context, lookbackDays int, triggerType string) (stored int, failed []string) {
	startedAt := time.Now().UTC()
	runID := fmt.Sprintf("eod_%s_%06d", startedAt.Format("20060102_150405"), startedAt.UnixNano()%1000000)

	runRecord := &models.EODIngestionRun{
		RunID:              runID,
		TriggerType:        triggerType,
		Provider:           "yahoo_finance",
		StartedAt:          startedAt,
		Status:             "RUNNING",
		IsExchangeVerified: false,
	}

	// Create durable run record
	if _, err := p.db.CreateEODIngestionRun(ctx, runRecord); err != nil {
		log.Printf("[EOD] WARN: failed to create durable run record: %v", err)
	}

	rows, err := p.db.QueryContext(ctx, `SELECT DISTINCT s.id, s.symbol FROM symbols s JOIN ohlcv_daily o ON o.symbol_id = s.id WHERE s.is_active ORDER BY s.symbol`)
	if err != nil {
		log.Printf("[EOD] cannot list symbols: %v", err)
		failed = []string{"*"}
		runRecord.CompletedAt = &time.Time{}
		*runRecord.CompletedAt = time.Now().UTC()
		runRecord.Status = "FAILED"
		runRecord.ErrorDetails = fmt.Sprintf("Failed to list active symbols: %v", err)
		_ = p.db.UpdateEODIngestionRun(ctx, runRecord)
		return 0, failed
	}

	type sym struct {
		id   int
		code string
	}
	var syms []sym
	for rows.Next() {
		var s sym
		if rows.Scan(&s.id, &s.code) == nil {
			syms = append(syms, s)
		}
	}
	rows.Close()

	runRecord.SymbolsAttempted = len(syms)
	now := time.Now()
	nowIST := now.In(istLoc)
	startOfPeriod := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day()-lookbackDays, 0, 0, 0, 0, istLoc)
	from := startOfPeriod.UTC()

	var (
		totalFetched           int
		totalRetries           int
		totalInserted          int
		totalUpdated           int
		totalUnchanged         int
		totalQuarantined       int
		totalRejected          int
		totalExplicitlySkipped int
		totalFailedRecords     int
	)

	for _, s := range syms {
		var dp *marketdata.DataPoint[[]models.OHLCV]
		var fetchErr error
		const maxRetries = 2

		// Bounded retries per symbol
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				totalRetries++
				time.Sleep(time.Duration(attempt*300) * time.Millisecond)
			}
			dp, fetchErr = p.marketProvider.GetDailyCandles(ctx, s.code, "NSE", from, now)
			if fetchErr == nil && dp != nil {
				break
			}
		}

		if fetchErr != nil || dp == nil {
			failed = append(failed, fmt.Sprintf("%s: %v", s.code, fetchErr))
			continue
		}

		totalFetched += len(dp.Data)
		var recs []database.OHLCVRecord
		for _, c := range dp.Data {
			if !sessionComplete(c.Timestamp, now) {
				totalExplicitlySkipped++
				continue // in-progress session: never store as a daily bar
			}
			recs = append(recs, dailyRecord(s.id, c))
		}

		stats, err := p.db.UpsertOHLCVBatchWithStats(ctx, recs)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", s.code, err))
			totalFailedRecords += len(recs)
			continue
		}
		totalInserted += stats.Inserted
		totalUpdated += stats.Updated
		totalUnchanged += stats.Unchanged
		totalQuarantined += stats.Quarantined
		totalRejected += stats.Rejected
		totalFailedRecords += stats.Failed
		stored += (stats.Inserted + stats.Updated + stats.Unchanged)
		time.Sleep(200 * time.Millisecond) // be gentle with the public endpoint
	}

	completedAt := time.Now().UTC()
	runRecord.CompletedAt = &completedAt
	runRecord.CandlesFetched = totalFetched
	runRecord.CandlesStored = stored
	runRecord.CandlesFailed = totalFailedRecords + len(failed)
	runRecord.RetryCount = totalRetries

	accountedTotal := totalInserted + totalUpdated + totalUnchanged + totalQuarantined + totalRejected + totalExplicitlySkipped
	reconciliationDelta := totalFetched - accountedTotal

	if reconciliationDelta == 0 && totalFailedRecords == 0 && len(failed) == 0 {
		runRecord.ReconciliationStatus = "RECONCILED"
		runRecord.Status = "SUCCESS"
	} else if (totalInserted + totalUpdated + totalUnchanged) > 0 {
		runRecord.ReconciliationStatus = "UNRECONCILED"
		runRecord.Status = "PARTIAL_DEGRADED"
		runRecord.ErrorDetails = fmt.Sprintf("Reconciliation mismatch: fetched=%d accounted=%d delta=%d failed_records=%d failed_symbols=%d",
			totalFetched, accountedTotal, reconciliationDelta, totalFailedRecords, len(failed))
	} else {
		runRecord.ReconciliationStatus = "UNRECONCILED"
		runRecord.Status = "FAILED"
		runRecord.ErrorDetails = strings.Join(failed, "; ")
	}

	detailsMap := map[string]any{
		"execution_duration_sec":     completedAt.Sub(startedAt).Seconds(),
		"lookback_days":              lookbackDays,
		"symbols_succeeded":          len(syms) - len(failed),
		"failure_count":              len(failed),
		"candles_fetched":            totalFetched,
		"candles_inserted":           totalInserted,
		"candles_updated":            totalUpdated,
		"candles_unchanged":          totalUnchanged,
		"candles_quarantined":        totalQuarantined,
		"candles_rejected":           totalRejected,
		"candles_explicitly_skipped": totalExplicitlySkipped,
		"candles_failed":             totalFailedRecords + len(failed),
		"reconciliation_status":      runRecord.ReconciliationStatus,
		"reconciliation_delta":       reconciliationDelta,
		"reconciliation_invariant":   "fetched == inserted + updated + unchanged + quarantined + rejected + explicitly_skipped",
		"verification_policy":        "SECONDARY_PROVIDER_UNVERIFIED_PENDING_BHAVCOPY",
	}
	if b, err := json.Marshal(detailsMap); err == nil {
		str := string(b)
		runRecord.Details = &str
	}

	if err := p.db.UpdateEODIngestionRun(ctx, runRecord); err != nil {
		log.Printf("[EOD] WARN: failed to update durable run record: %v", err)
	}

	log.Printf("[EOD] Run %s complete: status=%s recon=%s stored=%d failed=%d symbols=%d retries=%d (exchange_verified=false)",
		runID, runRecord.Status, runRecord.ReconciliationStatus, stored, len(failed), len(syms), totalRetries)
	return stored, failed
}
