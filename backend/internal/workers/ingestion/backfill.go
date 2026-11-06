package ingestion

import (
	"context"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
)

// fiveMinuteBarLength is the length of one stored history bar.
const fiveMinuteBarLength = 5 * time.Minute

// plausibleBackfillBar applies the same sanity rules the platform uses for stored bars: positive prices,
// high/low consistent with open/close, non-negative volume, and the bar must lie inside the NSE session and be complete.
func plausibleBackfillBar(b database.BackfillBar, now time.Time) bool {
	if b.Open <= 0 || b.Close <= 0 || b.Low <= 0 || b.High < b.Low || b.High < b.Open || b.High < b.Close ||
		b.Low > b.Open || b.Low > b.Close || b.Volume < 0 {
		return false
	}
	t := b.Timestamp.In(market.Location)
	mins := t.Hour()*60 + t.Minute()
	if mins < 9*60+15 || mins >= 15*60+30 {
		return false
	}
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	return !b.Timestamp.Add(fiveMinuteBarLength).After(now) // the still-forming bar is not history
}

// BackfillFiveMinute loads ~60 trading days of 5-minute bars for every active instrument. It never modifies rows that
// already exist, so it is safe to run repeatedly. Yahoo's 5-minute bars were verified equal to the platform's
// 1-minute-derived 5-minute bars (OHLC to float rounding, volume exactly) on the days both exist.
func (p *Pipeline) BackfillFiveMinute(ctx context.Context) (stored int, failed []string) {
	symbols, err := p.db.GetAllActiveSymbols(ctx)
	if err != nil {
		log.Printf("[BACKFILL] cannot list symbols: %v", err)
		return 0, []string{"*"}
	}
	now := time.Now()
	for _, s := range symbols {
		n, err := p.backfillSymbolFiveMinute(ctx, s, now)
		if err != nil {
			failed = append(failed, s.Symbol)
			time.Sleep(2 * time.Second)
			continue
		}
		stored += n
		time.Sleep(800 * time.Millisecond) // polite pacing; the free feed rate-limits bursts
	}
	return stored, failed
}

// backfillSymbolFiveMinute loads up to 60 sessions of 5-minute bars for one instrument and returns how many were new.
func (p *Pipeline) backfillSymbolFiveMinute(ctx context.Context, s database.SymbolRecord, now time.Time) (int, error) {
	return p.backfillSymbolRange(ctx, s, "60d", now)
}

func (p *Pipeline) backfillSymbolRange(ctx context.Context, s database.SymbolRecord, rng string, now time.Time) (int, error) {
	candles, err := p.yahoo.GetHistoricalRange(s.Symbol, rng, "5m")
	if err != nil {
		log.Printf("[BACKFILL] %s: fetch failed: %v", s.Symbol, err)
		return 0, err
	}
	var bars []database.BackfillBar
	skipped := 0
	for _, c := range candles {
		b := database.BackfillBar{Timestamp: c.Timestamp, Open: c.Open, High: c.High, Low: c.Low, Close: c.Close, Volume: c.Volume}
		if plausibleBackfillBar(b, now) {
			bars = append(bars, b)
		} else {
			skipped++
		}
	}
	n, err := p.db.InsertBackfillIntradayBars(ctx, s.ID, 5, database.BackfillSourceFiveMinute, bars)
	if err != nil {
		log.Printf("[BACKFILL] %s: store failed: %v", s.Symbol, err)
		return 0, err
	}
	log.Printf("[BACKFILL] %s: fetched=%d usable=%d new=%d skipped=%d", s.Symbol, len(candles), len(bars), n, skipped)
	return n, nil
}

// RefreshRecentFiveMinute reloads the last few sessions of 5-minute bars for every active instrument. Existing rows are
// never modified, so this only fills holes, chiefly the minutes before the close that the 1-minute feed omits.
func (p *Pipeline) RefreshRecentFiveMinute(ctx context.Context) (stored int, failed []string) {
	symbols, err := p.db.GetAllActiveSymbols(ctx)
	if err != nil {
		return 0, []string{"*"}
	}
	now := time.Now()
	for _, s := range symbols {
		n, err := p.backfillSymbolRange(ctx, s, "5d", now)
		if err != nil {
			failed = append(failed, s.Symbol)
			continue
		}
		stored += n
		time.Sleep(400 * time.Millisecond)
	}
	return stored, failed
}
