package database

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"time"

	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
)

const (
	basisIntraday1M      = "INTRADAY_1M"
	basisIntraday5M      = "INTRADAY_5M"
	basisExchangeDaily   = "EXCHANGE_DAILY"
	basisVendorDaily     = "VENDOR_DAILY"
	liquidityLookbackSes = 20
)

func istDay(t time.Time) time.Time {
	t = t.In(market.Location)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, market.Location)
}

// intradayBars returns finished intraday bars with start in [from, to) using 1-minute bars when stored, else 5-minute.
func (db *DB) intradayBars(ctx context.Context, symbolID int, from, to time.Time) ([]paper.Bar, error) {
	for _, c := range []struct {
		interval int
		basis    string
	}{{1, basisIntraday1M}, {5, basisIntraday5M}} {
		rows, err := db.QueryContext(ctx, `
			SELECT timestamp, open::float8, high::float8, low::float8, close::float8 FROM ohlcv_intraday
			WHERE symbol_id = $1 AND interval_minutes = $2 AND quality_status = 'VALID'
			  AND timestamp >= $3 AND timestamp + make_interval(mins => $2) <= $4
			ORDER BY timestamp`, symbolID, c.interval, from, to)
		if err != nil {
			return nil, err
		}
		var out []paper.Bar
		for rows.Next() {
			b := paper.Bar{Basis: c.basis}
			if err := rows.Scan(&b.Start, &b.Open, &b.High, &b.Low, &b.Close); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, b)
		}
		rows.Close()
		if len(out) > 0 {
			return out, rows.Err()
		}
	}
	return nil, nil
}

// dailyBar returns the day's OHLC: the exchange's own bhavcopy when loaded, else the vendor's daily bar.
func (db *DB) dailyBar(ctx context.Context, symbolID int, day time.Time) (*paper.Bar, error) {
	d := day.Format("2006-01-02")
	b := paper.Bar{Basis: basisExchangeDaily, Start: time.Date(day.Year(), day.Month(), day.Day(), 9, 15, 0, 0, market.Location)}
	err := db.QueryRowContext(ctx, `SELECT open::float8, high::float8, low::float8, close::float8 FROM exchange_daily_bars WHERE symbol_id = $1 AND trade_date = $2::date`, symbolID, d).
		Scan(&b.Open, &b.High, &b.Low, &b.Close)
	if err == nil {
		return &b, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	b.Basis = basisVendorDaily
	err = db.QueryRowContext(ctx, `SELECT open::float8, high::float8, low::float8, close::float8 FROM ohlcv_daily WHERE symbol_id = $1 AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date = $2::date`, symbolID, d).
		Scan(&b.Open, &b.High, &b.Low, &b.Close)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

// PathBars returns the price path a position held from `from` to `to` actually lived through, oldest first:
// intraday bars on the entry day after the entry, whole daily bars for the days in between, and intraday bars (or the
// daily bar once the session has closed) on the exit day. Sessions with no stored data are listed in missing.
func (db *DB) PathBars(ctx context.Context, symbolID int, from, to time.Time) (bars []paper.Bar, missing []string, err error) {
	from, to = from.In(market.Location), to.In(market.Location)
	firstDay, lastDay := istDay(from), istDay(to)
	sessionClose := func(d time.Time) time.Time {
		return time.Date(d.Year(), d.Month(), d.Day(), 15, 30, 0, 0, market.Location)
	}
	for d := firstDay; !d.After(lastDay); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if h, _ := market.IsTradingHoliday(d); h {
			continue
		}
		a, b := time.Date(d.Year(), d.Month(), d.Day(), 9, 15, 0, 0, market.Location), sessionClose(d)
		intraday := false
		if d.Equal(firstDay) {
			a, intraday = from, true
		}
		if d.Equal(lastDay) && to.Before(sessionClose(d)) {
			b, intraday = to, true
		}
		if d.Equal(firstDay) && !d.Equal(lastDay) {
			b = sessionClose(d)
		}
		if intraday {
			got, e := db.intradayBars(ctx, symbolID, a, b)
			if e != nil {
				return nil, nil, e
			}
			if len(got) == 0 && b.After(a) {
				missing = append(missing, d.Format("2006-01-02"))
			}
			bars = append(bars, got...)
			continue
		}
		bar, e := db.dailyBar(ctx, symbolID, d)
		if e != nil {
			return nil, nil, e
		}
		if bar == nil {
			missing = append(missing, d.Format("2006-01-02"))
			continue
		}
		bars = append(bars, *bar)
	}
	return bars, missing, nil
}

// LiquidityBefore summarises the average daily volume and daily volatility over the sessions before `at`, from the
// exchange's own daily bars. ok is false when there is not enough history to say anything.
func (db *DB) LiquidityBefore(ctx context.Context, symbolID int, at time.Time) (advShares, dailyVolPct float64, sessions int, ok bool, err error) {
	rows, err := db.QueryContext(ctx, `
		SELECT close::float8, volume::float8 FROM exchange_daily_bars
		WHERE symbol_id = $1 AND trade_date < $2::date ORDER BY trade_date DESC LIMIT $3`,
		symbolID, istDay(at).Format("2006-01-02"), liquidityLookbackSes+1)
	if err != nil {
		return 0, 0, 0, false, err
	}
	defer rows.Close()
	var closes, vols []float64
	for rows.Next() {
		var c, v float64
		if err := rows.Scan(&c, &v); err != nil {
			return 0, 0, 0, false, err
		}
		closes, vols = append(closes, c), append(vols, v)
	}
	if err := rows.Err(); err != nil {
		return 0, 0, 0, false, err
	}
	if len(closes) < 6 { // stocks added by the owner have no exchange history for older days: use the vendor's bars
		vr, err := db.QueryContext(ctx, `
			SELECT close::float8, volume::float8 FROM ohlcv_daily
			WHERE symbol_id = $1 AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date < $2::date ORDER BY timestamp DESC LIMIT $3`,
			symbolID, istDay(at).Format("2006-01-02"), liquidityLookbackSes+1)
		if err != nil {
			return 0, 0, 0, false, err
		}
		defer vr.Close()
		closes, vols = nil, nil
		for vr.Next() {
			var c, v float64
			if err := vr.Scan(&c, &v); err != nil {
				return 0, 0, 0, false, err
			}
			closes, vols = append(closes, c), append(vols, v)
		}
		if len(closes) < 6 {
			return 0, 0, 0, false, vr.Err()
		}
	}
	var rets []float64
	for i := 0; i+1 < len(closes); i++ { // newest first: closes[i] / closes[i+1]
		if closes[i+1] > 0 {
			rets = append(rets, closes[i]/closes[i+1]-1)
		}
	}
	var sumV float64
	for _, v := range vols[:len(vols)-1] {
		sumV += v
	}
	n := float64(len(vols) - 1)
	var mean float64
	for _, r := range rets {
		mean += r
	}
	mean /= float64(len(rets))
	var ss float64
	for _, r := range rets {
		ss += (r - mean) * (r - mean)
	}
	return sumV / n, math.Sqrt(ss/float64(len(rets)-1)) * 100, len(rets), true, nil
}

// FollowBars returns the bars after a signal day for the breakout tracker: intraday wherever stored, so the order of events
// inside each session is known and a result cannot change when a day's bars later become one daily bar. A session
// with no intraday data falls back to its daily bar once it has closed.
func (db *DB) FollowBars(ctx context.Context, symbolID int, signalDay, to time.Time) ([]paper.Bar, error) {
	to = to.In(market.Location)
	d := time.Date(signalDay.Year(), signalDay.Month(), signalDay.Day(), 0, 0, 0, 0, market.Location).AddDate(0, 0, 1)
	var out []paper.Bar
	for ; !d.After(to); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if h, _ := market.IsTradingHoliday(d); h {
			continue
		}
		open := time.Date(d.Year(), d.Month(), d.Day(), 9, 15, 0, 0, market.Location)
		closeT := time.Date(d.Year(), d.Month(), d.Day(), 15, 30, 0, 0, market.Location)
		end := closeT
		if to.Before(closeT) {
			end = to
		}
		bars, err := db.intradayBars(ctx, symbolID, open, end)
		if err != nil {
			return nil, err
		}
		if len(bars) > 0 {
			out = append(out, bars...)
			continue
		}
		if to.After(closeT) {
			bar, err := db.dailyBar(ctx, symbolID, d)
			if err != nil {
				return nil, err
			}
			if bar != nil {
				out = append(out, *bar)
			}
		}
	}
	return out, nil
}
