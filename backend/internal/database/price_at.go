package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/halal-equity/backend/internal/services/market"
)

// Price sources, ordered from finest to coarsest history.
const (
	PriceSourceIntraday1M  = "INTRADAY_1M"
	PriceSourceIntraday5M  = "INTRADAY_5M"
	PriceSourceExchangeEOD = "EXCHANGE_DAILY_CLOSE"
	PriceSourceVendorEOD   = "VENDOR_DAILY_CLOSE"
	PriceSourceLiveLatest  = "LIVE_LATEST"
)

// maxIntradayPriceAge is how far before the requested time the newest finished bar may be. Anything older means the
// feed had a gap and no honest price exists for that moment.
const maxIntradayPriceAge = 10 * time.Minute

// maxFeedGap is how far back to look across a hole in the feed before saying no price exists.
const maxFeedGap = 30 * time.Minute

// ErrNoPriceAt means no stored price can honestly be assigned to the requested time.
var ErrNoPriceAt = errors.New("no price available for that time")

// PricePoint is the price a paper order would fill at for a requested time, and where it came from.
type PricePoint struct {
	Price      float64    `json:"price"`
	Source     string     `json:"source"`
	BarTime    *time.Time `json:"bar_time"` // start of the bar used; nil for daily closes
	ResolvedAt time.Time  `json:"resolved_at"`
	Note       string     `json:"note,omitempty"`
}

// NextTradingSession returns the next trading day after `after` as a date (IST midnight).
func NextTradingSession(after time.Time) time.Time {
	d := time.Date(after.Year(), after.Month(), after.Day(), 0, 0, 0, 0, market.Location)
	for i := 0; i < 15; i++ {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if h, _ := market.IsTradingHoliday(d); h {
			continue
		}
		return d
	}
	return d
}

func isTradingDay(t time.Time) bool {
	t = t.In(market.Location)
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	h, _ := market.IsTradingHoliday(t)
	return !h
}

// PriceAt returns the price available at `at` with no lookahead: a bar's close only counts once that bar has finished.
//   - during the session: the newest finished 1-minute bar, else 5-minute bar, no older than 10 minutes;
//   - after the close on a trading day: that day's close, taken from the exchange's own record when loaded,
//     else the vendor daily bar;
//   - before the open, on weekends or holidays: ErrNoPriceAt, because there is no market to fill against.
func (db *DB) PriceAt(ctx context.Context, symbolID int, at time.Time) (*PricePoint, error) {
	at = at.In(market.Location)
	if !isTradingDay(at) {
		return nil, fmt.Errorf("%w: %s is not a trading day (weekend or exchange holiday)", ErrNoPriceAt, at.Format("2006-01-02"))
	}
	open := time.Date(at.Year(), at.Month(), at.Day(), 9, 15, 0, 0, market.Location)
	closeT := time.Date(at.Year(), at.Month(), at.Day(), 15, 30, 0, 0, market.Location)
	if at.Before(open) {
		return nil, fmt.Errorf("%w: the market opens at 09:15 IST", ErrNoPriceAt)
	}

	if at.Before(closeT) {
		for _, c := range []struct {
			interval int
			source   string
		}{{1, PriceSourceIntraday1M}, {5, PriceSourceIntraday5M}} {
			var price float64
			var barStart time.Time
			err := db.QueryRowContext(ctx, `
				SELECT close::float8, timestamp FROM ohlcv_intraday
				WHERE symbol_id = $1 AND interval_minutes = $2 AND quality_status = 'VALID'
				  AND timestamp + make_interval(mins => $2) <= $3 AND timestamp >= $4
				ORDER BY timestamp DESC LIMIT 1`, symbolID, c.interval, at, at.Add(-maxIntradayPriceAge)).Scan(&price, &barStart)
			if err == nil {
				return &PricePoint{Price: price, Source: c.source, BarTime: &barStart, ResolvedAt: at,
					Note: fmt.Sprintf("close of the %d-minute bar that finished at %s", c.interval,
						barStart.Add(time.Duration(c.interval)*time.Minute).In(market.Location).Format("15:04"))}, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		// The feed has known holes, chiefly 15:16-15:28 every day. A short hole still has an honest answer: the last valid
		// bar of this session, which is older but was final at that time. The note says how old it is.
		for _, c := range []struct {
			interval int
			source   string
		}{{1, PriceSourceIntraday1M}, {5, PriceSourceIntraday5M}} {
			var price float64
			var barStart time.Time
			err := db.QueryRowContext(ctx, `
				SELECT close::float8, timestamp FROM ohlcv_intraday
				WHERE symbol_id = $1 AND interval_minutes = $2 AND quality_status = 'VALID'
				  AND timestamp + make_interval(mins => $2) <= $3 AND timestamp >= $4
				ORDER BY timestamp DESC LIMIT 1`, symbolID, c.interval, at, at.Add(-maxFeedGap)).Scan(&price, &barStart)
			if err == nil {
				end := barStart.Add(time.Duration(c.interval) * time.Minute)
				return &PricePoint{Price: price, Source: c.source, BarTime: &barStart, ResolvedAt: at,
					Note: fmt.Sprintf("last valid price before the feed gap: the bar that finished at %s, %d minutes earlier",
						end.In(market.Location).Format("15:04"), int(at.Sub(end).Minutes()))}, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		return nil, fmt.Errorf("%w: no valid intraday price within %d minutes before %s (stored intraday history starts 2026-08-06; pick a time after the close for a daily close)",
			ErrNoPriceAt, int(maxFeedGap.Minutes()), at.Format("2006-01-02 15:04"))
	}

	// After the close: the day's official close.
	var price float64
	err := db.QueryRowContext(ctx, `SELECT close::float8 FROM exchange_daily_bars WHERE symbol_id = $1 AND trade_date = $2::date`,
		symbolID, at.Format("2006-01-02")).Scan(&price)
	if err == nil {
		return &PricePoint{Price: price, Source: PriceSourceExchangeEOD, ResolvedAt: at, Note: "official NSE closing price"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	err = db.QueryRowContext(ctx, `SELECT close::float8 FROM ohlcv_daily WHERE symbol_id = $1 AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date = $2::date`,
		symbolID, at.Format("2006-01-02")).Scan(&price)
	if err == nil {
		return &PricePoint{Price: price, Source: PriceSourceVendorEOD, ResolvedAt: at, Note: "vendor daily close (no exchange file loaded for this date)"}, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		// The day's daily bar is loaded after the close. Until then the last finished intraday bar of that session
		// is the honest closing price, and the source says so.
		for _, c := range []struct {
			interval int
			source   string
		}{{1, PriceSourceIntraday1M}, {5, PriceSourceIntraday5M}} {
			var barStart time.Time
			err := db.QueryRowContext(ctx, `
				SELECT close::float8, timestamp FROM ohlcv_intraday
				WHERE symbol_id = $1 AND interval_minutes = $2 AND quality_status = 'VALID'
				  AND timestamp + make_interval(mins => $2) <= $3 AND timestamp >= $4
				ORDER BY timestamp DESC LIMIT 1`, symbolID, c.interval, closeT, open).Scan(&price, &barStart)
			if err == nil {
				return &PricePoint{Price: price, Source: c.source, BarTime: &barStart, ResolvedAt: at,
					Note: "last traded price of the session (the official daily close is not loaded yet)"}, nil
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		return nil, fmt.Errorf("%w: no daily close stored for %s", ErrNoPriceAt, at.Format("2006-01-02"))
	}
	return nil, err
}

// LastTradeTime returns the execution time of the portfolio's newest trade, or nil when it has none.
func (db *DB) LastTradeTime(ctx context.Context, portfolioID int) (*time.Time, error) {
	var t sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT max(trade_date) FROM portfolio_trades WHERE portfolio_id = $1`, portfolioID).Scan(&t); err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	return &t.Time, nil
}
