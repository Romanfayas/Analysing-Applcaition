package handlers

import (
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/database"
)

func bar(day string, hh, mm int, o, c float64, vol int64) database.IntradayBar {
	d, _ := time.ParseInLocation("2006-01-02", day, istZone)
	return database.IntradayBar{Timestamp: time.Date(d.Year(), d.Month(), d.Day(), hh, mm, 0, 0, istZone), Open: o, High: max(o, c), Low: min(o, c), Close: c, Volume: vol}
}

func TestAggregateIntradayMakesFifteenMinuteCandlesInsideTheSession(t *testing.T) {
	var bars []database.IntradayBar
	for m := 0; m < 30; m++ {
		bars = append(bars, bar("2026-11-11", 9, 15+m, 100+float64(m), 101+float64(m), 10))
	}
	bars = append(bars, bar("2026-11-11", 15, 40, 1, 1, 999)) // after the session: ignored
	c := aggregateIntraday(bars, 15)
	if len(c) != 2 {
		t.Fatalf("want 2 candles, got %d", len(c))
	}
	if c[0].O != 100 || c[0].C != 115 || c[0].V != 150 || c[0].H != 115 || c[0].L != 100 {
		t.Errorf("first candle wrong: %+v", c[0])
	}
}

func TestFlagLargeMinutesNeedsHistoryAndBothThresholds(t *testing.T) {
	var bars []database.IntradayBar
	for d := 1; d <= 5; d++ { // five earlier sessions, steady 1,000 shares a minute
		day := time.Date(2026, 11, d, 0, 0, 0, 0, istZone).Format("2006-01-02")
		for m := 0; m < 60; m++ {
			bars = append(bars, bar(day, 10, m%60, 100, 100.5, 1000))
		}
	}
	for m := 0; m < 60; m++ { // the latest session: steady, with one big up minute and one big minute that is too small in rupees
		v := int64(1000)
		o, c := 100.0, 100.5
		switch m {
		case 10:
			v, c = 50_000, 101 // 50x normal, ₹50 lakh
		case 20:
			v, o, c = 6_000, 100, 99 // 6x normal but only ₹6 lakh
		}
		bars = append(bars, bar("2026-11-06", 10, m, o, c, v))
	}
	sum, large := flagLargeMinutes(bars, 99, 2_000_000)
	if sum == nil || sum.BaselineSessions != 5 || len(large) != 1 {
		t.Fatalf("want one flagged minute over 5 baseline sessions, got %+v %+v", sum, large)
	}
	if large[0].Time != "10:10" || large[0].Side != "UP" || large[0].Times < 49 {
		t.Errorf("wrong flag: %+v", large[0])
	}
	_, none := flagLargeMinutes(bars[len(bars)-60:], 99, 2_000_000) // no earlier sessions: nothing can be called unusual
	if len(none) != 0 {
		t.Errorf("flags without a baseline: %+v", none)
	}
}

func TestSummarizeRangeMeasuresAgainstTheCloseBeforeTheRange(t *testing.T) {
	c := []ChartCandle{{Date: "2026-11-09", O: 100, H: 104, L: 99, C: 103, V: 10}, {Date: "2026-11-10", O: 103, H: 108, L: 101, C: 105, V: 20}}
	sm := summarizeRange(c, 100, "close before the range")
	if sm.Close != 105 || sm.High != 108 || sm.Low != 99 || sm.Volume != 30 || sm.Change != 5 || sm.ChangePct != 5 || sm.FirstLabel != "2026-11-09" || sm.LastLabel != "2026-11-10" {
		t.Fatalf("unexpected summary %+v", sm)
	}
	if got := summarizeRange(c, 0, ""); got.BasePrice != 100 || got.BaseLabel != "first candle's open" {
		t.Fatalf("with no base the first open must be used, got %+v", got)
	}
	if summarizeRange(nil, 100, "x") != nil {
		t.Fatal("no candles, no summary")
	}
}
