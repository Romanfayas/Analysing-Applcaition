package database

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestDailyOHLCVQueryAggregation verifies that GetRecentCandlesForSymbol strictly returns
// 1 clean bar per distinct calendar session, aggregating any duplicate intraday timestamps.
func TestDailyOHLCVQueryAggregation(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "192.168.18.43"
		}
		dbURL = "postgres://halal_user:devpassword@" + host + ":5432/halal_equity?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := NewDB(dbURL)
	if err != nil {
		t.Skipf("PostgreSQL database unavailable on host: %v", err)
		return
	}
	defer db.Close()

	// Query symbol 1 (ADANIENT)
	records, err := db.GetRecentCandlesForSymbol(ctx, 1, 40)
	if err != nil {
		t.Fatalf("GetRecentCandlesForSymbol failed: %v", err)
	}

	if len(records) == 0 {
		t.Fatalf("Expected at least 1 record, got 0")
	}

	// 1. Verify chronological ordering (oldest to newest)
	for i := 1; i < len(records); i++ {
		if !records[i].Timestamp.After(records[i-1].Timestamp) {
			t.Errorf("Records out of order: idx %d (%v) not after idx %d (%v)",
				i, records[i].Timestamp, i-1, records[i-1].Timestamp)
		}
	}

	// 2. Verify all records have distinct calendar dates (zero intraday 1-minute duplicates)
	dateSet := make(map[string]bool)
	for i, r := range records {
		dStr := r.Timestamp.Format("2006-01-02")
		if dateSet[dStr] {
			t.Errorf("Duplicate session date %s detected at index %d! 1-minute contamination present.", dStr, i)
		}
		dateSet[dStr] = true

		// 3. Verify valid OHLC invariants
		if r.High < r.Low {
			t.Errorf("Invalid OHLC at index %d: High %f < Low %f", i, r.High, r.Low)
		}
		if r.Open < r.Low || r.Open > r.High {
			t.Errorf("Invalid Open at index %d: Open %f outside [%f, %f]", i, r.Open, r.Low, r.High)
		}
		if r.Close < r.Low || r.Close > r.High {
			t.Errorf("Invalid Close at index %d: Close %f outside [%f, %f]", i, r.Close, r.Low, r.High)
		}
	}
}

// TestNoSilentCoalesceFallback verifies that GetOHLCV does NOT silently fall back to close
// when adjusted_close is NULL in the database.
func TestNoSilentCoalesceFallback(t *testing.T) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "192.168.18.43"
		}
		dbURL = "postgres://halal_user:devpassword@" + host + ":5432/halal_equity?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := NewDB(dbURL)
	if err != nil {
		t.Skipf("PostgreSQL database unavailable on host: %v", err)
		return
	}
	defer db.Close()

	// Query historical date range in 2024 where adjusted_close IS NULL
	from := time.Date(2024, 10, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 10, 15, 0, 0, 0, 0, time.UTC)

	records, err := db.GetOHLCV(ctx, 1, from, to)
	if err != nil {
		t.Fatalf("GetOHLCV failed: %v", err)
	}

	if len(records) == 0 {
		t.Skip("No historical records found for symbol 1 in 2024 range")
		return
	}

	// Historical records must have AdjClose == nil, proving no silent fallback
	hasNullAdj := false
	for _, r := range records {
		if r.AdjClose == nil {
			hasNullAdj = true
			break
		}
	}

	if !hasNullAdj {
		t.Errorf("Expected historical bars to have AdjClose == nil, but all had non-nil values. Silent fallback detected!")
	}
}

