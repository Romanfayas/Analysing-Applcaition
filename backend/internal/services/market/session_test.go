package market

import (
	"testing"
	"time"
)

func TestGetFreshnessStatus(t *testing.T) {
	loc := Location

	// Case 1: Zero timestamp -> UNAVAILABLE
	if status := GetFreshnessStatus(time.Time{}, time.Now()); status != "UNAVAILABLE" {
		t.Errorf("expected UNAVAILABLE for zero time, got %s", status)
	}

	// Case 2: Market Open (Monday 11:00 AM IST)
	// Candle from 10:55 AM IST (5 mins ago) -> FRESH
	mondayOpen := time.Date(2026, 10, 12, 11, 0, 0, 0, loc)
	candleRecent := time.Date(2026, 10, 12, 10, 55, 0, 0, loc)
	if status := GetFreshnessStatus(candleRecent, mondayOpen); status != "FRESH" {
		t.Errorf("expected FRESH for 5 min old candle during open market, got %s", status)
	}

	// Candle from 10:40 AM IST (20 mins ago) -> STALE
	candleOld := time.Date(2026, 10, 12, 10, 40, 0, 0, loc)
	if status := GetFreshnessStatus(candleOld, mondayOpen); status != "STALE" {
		t.Errorf("expected STALE for 20 min old candle during open market, got %s", status)
	}

	// Candle from previous Friday during Monday Open -> STALE
	fridayCandle := time.Date(2026, 10, 9, 15, 30, 0, 0, loc)
	if status := GetFreshnessStatus(fridayCandle, mondayOpen); status != "STALE" {
		t.Errorf("expected STALE for Friday candle during Monday open, got %s", status)
	}

	// Case 3: Market Closed after trading day (Monday 6:00 PM IST)
	mondayEvening := time.Date(2026, 10, 12, 18, 0, 0, 0, loc)
	mondayCloseCandle := time.Date(2026, 10, 12, 15, 29, 0, 0, loc)
	if status := GetFreshnessStatus(mondayCloseCandle, mondayEvening); status != "LAST_AVAILABLE" {
		t.Errorf("expected LAST_AVAILABLE for today's close candle on Monday evening, got %s", status)
	}

	// If candle was from Friday on Monday evening (missed Monday session) -> STALE
	if status := GetFreshnessStatus(fridayCandle, mondayEvening); status != "STALE" {
		t.Errorf("expected STALE when missing Monday session on Monday evening, got %s", status)
	}

	// Case 4: Weekend (Saturday 2:00 PM IST)
	saturday := time.Date(2026, 10, 17, 14, 0, 0, 0, loc)
	fridayClose := time.Date(2026, 10, 16, 15, 30, 0, 0, loc)
	if status := GetFreshnessStatus(fridayClose, saturday); status != "LAST_AVAILABLE" {
		t.Errorf("expected LAST_AVAILABLE on Saturday for Friday close, got %s", status)
	}

	// 1 week old candle on Saturday -> STALE
	lastWeekCandle := time.Date(2026, 10, 10, 15, 30, 0, 0, loc)
	if status := GetFreshnessStatus(lastWeekCandle, saturday); status != "STALE" {
		t.Errorf("expected STALE for 1 week old candle on Saturday, got %s", status)
	}

	// Case 5: Official Exchange Holiday (Dussehra - Tuesday, Oct 20, 2026 at 11:00 AM IST)
	dussehraNoon := time.Date(2026, 10, 20, 11, 0, 0, 0, loc)
	if session := GetCurrentSession(dussehraNoon); session != SessionClosed {
		t.Errorf("expected SessionClosed during Dussehra holiday, got %s", session)
	}

	// Monday's close candle (Oct 19, 2026 15:30 IST) during Tuesday holiday -> LAST_AVAILABLE
	mondayOct19Close := time.Date(2026, 10, 19, 15, 30, 0, 0, loc)
	if status := GetFreshnessStatus(mondayOct19Close, dussehraNoon); status != "LAST_AVAILABLE" {
		t.Errorf("expected LAST_AVAILABLE during Dussehra holiday for Monday's close, got %s", status)
	}

	// 5-day old candle during holiday -> STALE
	oldHolidayCandle := time.Date(2026, 10, 15, 15, 30, 0, 0, loc)
	if status := GetFreshnessStatus(oldHolidayCandle, dussehraNoon); status != "STALE" {
		t.Errorf("expected STALE for 5-day-old candle on holiday, got %s", status)
	}
}
