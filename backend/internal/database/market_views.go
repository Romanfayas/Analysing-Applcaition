package database

import (
	"context"
	"fmt"
	"time"
)

// StrictResearchEligible is the read-time research-eligibility rule for an ohlcv_daily row aliased `d`.
//
// adjusted_close IS NOT NULL alone is not enough: the quarantine guard only checks evidence when a bar is written,
// so a bar released while its corporate-action event was verified stays released after that event is demoted.
// A bar is eligible only if it is a nominal vendor bar (adjusted = close) or its adjustment is backed NOW by an
// undisputed, verified factor on a VERIFIED_EXCHANGE_PRIMARY event.
const StrictResearchEligible = `(d.adjusted_close IS NOT NULL AND (
	(d.source = 'yahoo_finance' AND d.adjusted_close = d.close)
	OR EXISTS (
		SELECT 1 FROM corporate_action_adjustment_factors f
		JOIN corporate_action_events e ON e.event_id = f.event_id AND e.symbol_id = f.symbol_id
		WHERE f.symbol_id = d.symbol_id AND e.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
		  AND f.is_verified AND f.dispute_status = 'UNDISPUTED' AND f.price_factor_applicable
		  AND (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date < f.ex_date
		  AND abs(d.adjusted_close - round(d.close * f.total_return_factor, 4)) <= 0.0005)))`

// DailyBar is a stored daily bar together with its research-eligibility flag.
//
// A bar is research-eligible only when its adjusted close has been populated by the
// verified pipeline; quarantined bars (adjusted_close IS NULL) are display-only and must
// never feed calculations, models or backtests.
type DailyBar struct {
	Timestamp        time.Time `json:"timestamp"`
	Open             float64   `json:"open"`
	High             float64   `json:"high"`
	Low              float64   `json:"low"`
	Close            float64   `json:"close"`
	Volume           int64     `json:"volume"`
	AdjustedClose    *float64  `json:"adjusted_close"`
	Source           string    `json:"source"`
	QualityStatus    string    `json:"quality_status"`
	ResearchEligible bool      `json:"research_eligible"`
}

// IntradayBar is a stored intraday bar.
type IntradayBar struct {
	Timestamp       time.Time `json:"timestamp"`
	IntervalMinutes int       `json:"interval_minutes"`
	Open            float64   `json:"open"`
	High            float64   `json:"high"`
	Low             float64   `json:"low"`
	Close           float64   `json:"close"`
	Volume          int64     `json:"volume"`
	Source          string    `json:"source"`
	QualityStatus   string    `json:"quality_status"`
}

// BarCoverage summarises how much stored history exists for a symbol and how much of it is usable.
type BarCoverage struct {
	DailyTotal       int        `json:"daily_total"`
	DailyEligible    int        `json:"daily_research_eligible"`
	DailyQuarantined int        `json:"daily_quarantined"`
	DailyRevoked     int        `json:"daily_eligibility_revoked"` // released earlier, no longer backed by verified evidence
	DailyFirst       *time.Time `json:"daily_first"`
	DailyLast        *time.Time `json:"daily_last"`
	IntradayTotal    int        `json:"intraday_total"`
	IntradayLast     *time.Time `json:"intraday_last"`
}

// GetDailyBars returns the most recent `limit` daily bars in ascending time order.
// When eligibleOnly is true, quarantined bars are excluded.
func (db *DB) GetDailyBars(ctx context.Context, symbolID, limit int, eligibleOnly bool) ([]DailyBar, error) {
	query := `
		SELECT d.timestamp, d.open, d.high, d.low, d.close, d.volume, d.adjusted_close, d.source, d.quality_status::text,
		       ` + StrictResearchEligible + `
		FROM ohlcv_daily d
		WHERE d.symbol_id = $1 AND ($3::bool = false OR ` + StrictResearchEligible + `)
		ORDER BY d.timestamp DESC
		LIMIT $2
	`
	rows, err := db.QueryContext(ctx, query, symbolID, limit, eligibleOnly)
	if err != nil {
		return nil, fmt.Errorf("query daily bars failed: %w", err)
	}
	defer rows.Close()

	bars := make([]DailyBar, 0, limit)
	for rows.Next() {
		var b DailyBar
		if err := rows.Scan(&b.Timestamp, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume,
			&b.AdjustedClose, &b.Source, &b.QualityStatus, &b.ResearchEligible); err != nil {
			return nil, fmt.Errorf("scan daily bar failed: %w", err)
		}
		bars = append(bars, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Newest-first from SQL for LIMIT; callers want chronological order.
	for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
		bars[i], bars[j] = bars[j], bars[i]
	}
	return bars, nil
}

// GetIntradayBars returns the most recent `limit` intraday bars in ascending time order.
func (db *DB) GetIntradayBars(ctx context.Context, symbolID, limit int) ([]IntradayBar, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT timestamp, interval_minutes, open, high, low, close, volume, source, quality_status::text
		FROM ohlcv_intraday
		WHERE symbol_id = $1
		ORDER BY timestamp DESC
		LIMIT $2
	`, symbolID, limit)
	if err != nil {
		return nil, fmt.Errorf("query intraday bars failed: %w", err)
	}
	defer rows.Close()

	bars := make([]IntradayBar, 0, limit)
	for rows.Next() {
		var b IntradayBar
		if err := rows.Scan(&b.Timestamp, &b.IntervalMinutes, &b.Open, &b.High, &b.Low, &b.Close,
			&b.Volume, &b.Source, &b.QualityStatus); err != nil {
			return nil, fmt.Errorf("scan intraday bar failed: %w", err)
		}
		bars = append(bars, b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
		bars[i], bars[j] = bars[j], bars[i]
	}
	return bars, nil
}

// GetBarCoverage reports stored history and research eligibility for a symbol.
func (db *DB) GetBarCoverage(ctx context.Context, symbolID int) (*BarCoverage, error) {
	var c BarCoverage
	err := db.QueryRowContext(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE `+StrictResearchEligible+`),
		       count(*) FILTER (WHERE d.adjusted_close IS NULL),
		       count(*) FILTER (WHERE d.adjusted_close IS NOT NULL AND NOT `+StrictResearchEligible+`),
		       min(d.timestamp), max(d.timestamp)
		FROM ohlcv_daily d WHERE d.symbol_id = $1
	`, symbolID).Scan(&c.DailyTotal, &c.DailyEligible, &c.DailyQuarantined, &c.DailyRevoked, &c.DailyFirst, &c.DailyLast)
	if err != nil {
		return nil, fmt.Errorf("daily coverage failed: %w", err)
	}
	err = db.QueryRowContext(ctx, `
		SELECT count(*), max(timestamp) FROM ohlcv_intraday WHERE symbol_id = $1
	`, symbolID).Scan(&c.IntradayTotal, &c.IntradayLast)
	if err != nil {
		return nil, fmt.Errorf("intraday coverage failed: %w", err)
	}
	return &c, nil
}

// SearchActiveSymbols finds active instruments whose ticker or name contains the query.
// Exact and prefix ticker matches rank first.
func (db *DB) SearchActiveSymbols(ctx context.Context, q string, limit int) ([]SymbolRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, symbol, name, exchange, COALESCE(isin, ''), COALESCE(sector, ''),
		       COALESCE(industry, ''), COALESCE(market_cap_category, ''), is_active, created_at
		FROM symbols
		WHERE is_active = true
		  AND (symbol ILIKE $1 ESCAPE '\' OR name ILIKE $1 ESCAPE '\')
		ORDER BY (upper(symbol) = upper($2)) DESC,
		         (symbol ILIKE $3 ESCAPE '\') DESC,
		         symbol ASC
		LIMIT $4
	`, "%"+escapeLike(q)+"%", q, escapeLike(q)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search symbols failed: %w", err)
	}
	defer rows.Close()

	out := make([]SymbolRecord, 0, limit)
	for rows.Next() {
		var r SymbolRecord
		if err := rows.Scan(&r.ID, &r.Symbol, &r.Name, &r.Exchange, &r.ISIN,
			&r.Sector, &r.Industry, &r.MarketCapCat, &r.IsActive, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan symbol failed: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func escapeLike(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '%', '_':
			out = append(out, '\\')
		}
		out = append(out, s[i])
	}
	return string(out)
}

// GetLatestPrice returns the most recent stored bar across intraday and daily data.
// Preferring intraday unconditionally would pin a symbol to an old intraday bar for as long
// as intraday ingestion is down, even though newer daily closes exist.
func (db *DB) GetLatestPrice(ctx context.Context, symbolID int) (*OHLCVRecord, error) {
	intra, ierr := db.GetLatestIntradayOHLCV(ctx, symbolID)
	daily, derr := db.GetLatestOHLCV(ctx, symbolID)
	switch {
	case ierr == nil && derr == nil:
		if intra.Timestamp.After(daily.Timestamp) {
			return intra, nil
		}
		return daily, nil
	case ierr == nil:
		return intra, nil
	case derr == nil:
		return daily, nil
	default:
		return nil, derr
	}
}

// CountValidIntradayBars counts VALID 1-minute bars of a symbol in [from, to).
func (db *DB) CountValidIntradayBars(ctx context.Context, symbolID int, from, to time.Time) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM ohlcv_intraday
		WHERE symbol_id = $1 AND interval_minutes = 1 AND quality_status = 'VALID' AND timestamp >= $2 AND timestamp < $3`,
		symbolID, from, to).Scan(&n)
	return n, err
}

// IPOSubscriptionSnapshot is one reading of an issue's demand from the exchange.
type IPOSubscriptionSnapshot struct {
	QIB, NII, Retail, Total *float64
	Source                  string
}

// RecordIPOSubscription appends a subscription reading unless it is identical to the latest one stored.
// day_number is the bidding day (1 on the open date, counting working days).
func (db *DB) RecordIPOSubscription(ctx context.Context, ipoID int, openDate *time.Time, now time.Time, s IPOSubscriptionSnapshot) (bool, error) {
	if s.Total == nil && s.QIB == nil && s.NII == nil && s.Retail == nil {
		return false, nil
	}
	day := 1
	if openDate != nil {
		for d := *openDate; d.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())); d = d.AddDate(0, 0, 1) {
			if wd := d.Weekday(); wd != time.Saturday && wd != time.Sunday {
				day++
			}
		}
	}
	res, err := db.ExecContext(ctx, `
		INSERT INTO ipo_subscriptions (ipo_id, day_number, recorded_at, qib_subscription, nii_subscription, retail_subscription, total_subscription, source)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8
		WHERE NOT EXISTS (
			SELECT 1 FROM (SELECT qib_subscription, nii_subscription, retail_subscription, total_subscription
			               FROM ipo_subscriptions WHERE ipo_id = $1 ORDER BY recorded_at DESC, id DESC LIMIT 1) l
			WHERE l.qib_subscription IS NOT DISTINCT FROM $4::numeric AND l.nii_subscription IS NOT DISTINCT FROM $5::numeric
			  AND l.retail_subscription IS NOT DISTINCT FROM $6::numeric AND l.total_subscription IS NOT DISTINCT FROM $7::numeric)`,
		ipoID, day, now, s.QIB, s.NII, s.Retail, s.Total, s.Source)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ScanBar is one daily bar of a breakout-scan window with the exchange cross-check of that bar.
type ScanBar struct {
	TradeDate time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
	// Classification is the vendor-vs-exchange result for this bar (evidence.v_daily_bar_reconciliation), or
	// UNVERIFIED when no exchange file has been loaded for that date.
	Classification string
}

// GetScanWindow returns the latest `limit` daily bars of a symbol in chronological order, each cross-checked against
// the exchange record where one is loaded.
func (db *DB) GetScanWindow(ctx context.Context, symbolID, limit int) ([]ScanBar, error) {
	rows, err := db.QueryContext(ctx, `
		WITH b AS (
			SELECT (d.timestamp AT TIME ZONE 'Asia/Kolkata')::date AS trade_date, d.open, d.high, d.low, d.close, d.volume
			FROM ohlcv_daily d WHERE d.symbol_id = $1 ORDER BY d.timestamp DESC LIMIT $2)
		SELECT b.trade_date, b.open, b.high, b.low, b.close, b.volume, COALESCE(r.classification, 'UNVERIFIED')
		FROM b LEFT JOIN evidence.v_daily_bar_reconciliation r ON r.symbol_id = $1 AND r.trade_date = b.trade_date
		ORDER BY b.trade_date ASC`, symbolID, limit)
	if err != nil {
		return nil, fmt.Errorf("scan window query failed: %w", err)
	}
	defer rows.Close()
	var out []ScanBar
	for rows.Next() {
		var b ScanBar
		if err := rows.Scan(&b.TradeDate, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.Classification); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// BackfillBar is one historical intraday bar loaded after the fact (not observed live).
type BackfillBar struct {
	Timestamp              time.Time
	Open, High, Low, Close float64
	Volume                 int64
}

// BackfillSourceFiveMinute labels 5-minute history loaded from the provider's 60-day window.
const BackfillSourceFiveMinute = "yahoo_finance_5m_backfill"

// InsertBackfillIntradayBars stores historical bars without ever touching existing rows (ON CONFLICT DO NOTHING).
// first_received_at stays NULL: these bars were not received live, so they must not enter latency statistics.
func (db *DB) InsertBackfillIntradayBars(ctx context.Context, symbolID, intervalMinutes int, source string, bars []BackfillBar) (int, error) {
	if len(bars) == 0 {
		return 0, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source, quality_status, retrieved_at, first_received_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'VALID', NOW(), NULL)
		ON CONFLICT (symbol_id, timestamp, interval_minutes) DO NOTHING`)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()
	stored := 0
	for _, b := range bars {
		res, err := stmt.ExecContext(ctx, symbolID, b.Timestamp, intervalMinutes, b.Open, b.High, b.Low, b.Close, b.Volume, source)
		if err != nil {
			return stored, fmt.Errorf("backfill insert failed: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			stored++
		}
	}
	return stored, tx.Commit()
}

// IntradayLastSessions returns every stored intraday bar of the symbol's most recent `sessions` trading days (IST dates), oldest first.
func (db *DB) IntradayLastSessions(ctx context.Context, symbolID, sessions int) ([]IntradayBar, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT timestamp, interval_minutes, open, high, low, close, volume, source, quality_status::text
		FROM ohlcv_intraday
		WHERE symbol_id = $1 AND (timestamp AT TIME ZONE 'Asia/Kolkata')::date >= (
			SELECT min(d) FROM (SELECT DISTINCT (timestamp AT TIME ZONE 'Asia/Kolkata')::date AS d FROM ohlcv_intraday
			                    WHERE symbol_id = $1 ORDER BY d DESC LIMIT $2) x)
		ORDER BY timestamp`, symbolID, sessions)
	if err != nil {
		return nil, fmt.Errorf("query intraday sessions failed: %w", err)
	}
	defer rows.Close()
	var bars []IntradayBar
	for rows.Next() {
		var b IntradayBar
		if err := rows.Scan(&b.Timestamp, &b.IntervalMinutes, &b.Open, &b.High, &b.Low, &b.Close, &b.Volume, &b.Source, &b.QualityStatus); err != nil {
			return nil, fmt.Errorf("scan intraday bar failed: %w", err)
		}
		bars = append(bars, b)
	}
	return bars, rows.Err()
}
