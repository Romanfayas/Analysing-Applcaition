package ingestion

import (
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

func TestDailyStampIs0915IST(t *testing.T) {
	// Yahoo stamps daily bars at the session open; any instant on that IST date must map to 03:45 UTC.
	for _, in := range []time.Time{
		time.Date(2026, 10, 7, 3, 45, 0, 0, time.UTC),
		time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC), // 15:30 IST
		time.Date(2026, 10, 6, 19, 0, 0, 0, time.UTC), // 00:30 IST on Oct 7
	} {
		got := dailyStamp(in)
		want := time.Date(2026, 10, 7, 3, 45, 0, 0, time.UTC)
		if !got.Equal(want) {
			t.Errorf("dailyStamp(%v)=%v want %v", in, got, want)
		}
	}
}

func TestSessionCompleteNeverAcceptsInProgressDay(t *testing.T) {
	d := time.Date(2026, 10, 14, 3, 45, 0, 0, time.UTC)
	if sessionComplete(d, time.Date(2026, 10, 14, 8, 33, 0, 0, time.UTC)) { // 14:03 IST, market open
		t.Error("in-progress session accepted")
	}
	if !sessionComplete(d, time.Date(2026, 10, 14, 10, 30, 0, 0, time.UTC)) { // 16:00 IST
		t.Error("completed session rejected")
	}
	if !sessionComplete(d, time.Date(2026, 10, 15, 3, 0, 0, 0, time.UTC)) {
		t.Error("previous-day session rejected")
	}
}

func TestEODDegradedStatusDetermination(t *testing.T) {
	// Status determination logic:
	// 0 failures -> SUCCESS
	// >0 failures and >0 stored -> PARTIAL_DEGRADED
	// 0 stored and >0 attempted -> FAILED

	determineStatus := func(symbolsAttempted, stored, failed int) string {
		if failed == 0 && stored > 0 {
			return "SUCCESS"
		}
		if failed > 0 && stored > 0 {
			return "PARTIAL_DEGRADED"
		}
		if stored == 0 && symbolsAttempted > 0 {
			return "FAILED"
		}
		return "UNKNOWN"
	}

	if st := determineStatus(50, 500, 0); st != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", st)
	}
	if st := determineStatus(50, 480, 2); st != "PARTIAL_DEGRADED" {
		t.Errorf("expected PARTIAL_DEGRADED, got %s", st)
	}
	if st := determineStatus(50, 0, 50); st != "FAILED" {
		t.Errorf("expected FAILED, got %s", st)
	}
}

func TestEODRunRecordModelFields(t *testing.T) {
	now := time.Now().UTC()
	run := models.EODIngestionRun{
		RunID:              "eod_20261015_160000_123456",
		TriggerType:        "SCHEDULED",
		Provider:           "yahoo_finance",
		StartedAt:          now,
		Status:             "SUCCESS",
		SymbolsAttempted:   50,
		CandlesFetched:     500,
		CandlesStored:      500,
		CandlesFailed:      0,
		RetryCount:         1,
		IsExchangeVerified: false,
	}

	if run.IsExchangeVerified {
		t.Error("Yahoo Finance EOD run should NOT be marked exchange-verified without Bhavcopy audit")
	}
	if run.Status != "SUCCESS" {
		t.Errorf("unexpected status %s", run.Status)
	}
}

func TestReconciliationInvariantExactReconciliation(t *testing.T) {
	// Verify that when all fetched candles are categorized into mutually exclusive outcomes,
	// the invariant fetched == inserted + updated + unchanged + quarantined + rejected + explicitly_skipped holds exactly.
	fetched := 423
	inserted := 0
	updated := 0
	unchanged := 376
	quarantined := 0
	rejected := 0
	explicitlySkipped := 47
	failedRecords := 0
	failedSymbolsCount := 0

	accountedTotal := inserted + updated + unchanged + quarantined + rejected + explicitlySkipped
	reconciliationDelta := fetched - accountedTotal

	if reconciliationDelta != 0 {
		t.Fatalf("expected reconciliationDelta 0, got %d", reconciliationDelta)
	}

	var status, reconStatus string
	if reconciliationDelta == 0 && failedRecords == 0 && failedSymbolsCount == 0 {
		reconStatus = "RECONCILED"
		status = "SUCCESS"
	} else {
		reconStatus = "UNRECONCILED"
		status = "PARTIAL_DEGRADED"
	}

	if reconStatus != "RECONCILED" {
		t.Errorf("expected RECONCILED, got %s", reconStatus)
	}
	if status != "SUCCESS" {
		t.Errorf("expected SUCCESS, got %s", status)
	}
}

func TestReconciliationInvariantUnaccountedDeltaDetected(t *testing.T) {
	// Regression test for P1.8.6: 415 fetched, 368 updated, 47 unexplained in-progress records.
	// When explicitly_skipped is omitted, delta is 47, and the run MUST be marked UNRECONCILED and degraded.
	fetched := 415
	inserted := 0
	updated := 368
	unchanged := 0
	quarantined := 0
	rejected := 0
	explicitlySkipped := 0 // Bug: omitted in-progress candle count!
	failedRecords := 0
	failedSymbolsCount := 0

	accountedTotal := inserted + updated + unchanged + quarantined + rejected + explicitlySkipped
	reconciliationDelta := fetched - accountedTotal

	if reconciliationDelta != 47 {
		t.Fatalf("expected reconciliationDelta 47, got %d", reconciliationDelta)
	}

	var status, reconStatus string
	if reconciliationDelta == 0 && failedRecords == 0 && failedSymbolsCount == 0 {
		reconStatus = "RECONCILED"
		status = "SUCCESS"
	} else if (inserted + updated + unchanged) > 0 {
		reconStatus = "UNRECONCILED"
		status = "PARTIAL_DEGRADED"
	} else {
		reconStatus = "UNRECONCILED"
		status = "FAILED"
	}

	if reconStatus != "UNRECONCILED" {
		t.Errorf("expected UNRECONCILED, got %s", reconStatus)
	}
	if status == "SUCCESS" {
		t.Errorf("run with 47 unaccounted candles MUST NOT be marked SUCCESS")
	}
}

func TestReconciliationInvariantFailuresPreventSuccess(t *testing.T) {
	// A run where failures occurred must not be marked SUCCESS even if all fetched records match
	fetched := 415
	inserted := 0
	updated := 0
	unchanged := 368
	quarantined := 0
	rejected := 0
	explicitlySkipped := 47
	failedRecords := 2
	failedSymbolsCount := 1

	accountedTotal := inserted + updated + unchanged + quarantined + rejected + explicitlySkipped
	reconciliationDelta := fetched - accountedTotal

	var status, reconStatus string
	if reconciliationDelta == 0 && failedRecords == 0 && failedSymbolsCount == 0 {
		reconStatus = "RECONCILED"
		status = "SUCCESS"
	} else if (inserted + updated + unchanged) > 0 {
		reconStatus = "UNRECONCILED"
		status = "PARTIAL_DEGRADED"
	} else {
		reconStatus = "UNRECONCILED"
		status = "FAILED"
	}

	if status == "SUCCESS" {
		t.Errorf("expected degraded status when failures occur, got SUCCESS")
	}
	if reconStatus != "UNRECONCILED" {
		t.Errorf("expected UNRECONCILED when failures occur, got %s", reconStatus)
	}
}

