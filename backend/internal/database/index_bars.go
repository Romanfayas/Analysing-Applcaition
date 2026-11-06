package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/halal-equity/backend/internal/providers/exchange/nseindex"
)

// Benchmark index names as NSE writes them in its files.
const (
	IndexNifty50  = "Nifty 50"
	IndexNifty500 = "Nifty 500"
)

// IndexPoint is one stored index close.
type IndexPoint struct {
	Date  time.Time `json:"date"`
	Close float64   `json:"close"`
}

// IndexSnapshot is an index's latest stored day with valuation context.
type IndexSnapshot struct {
	Name          string    `json:"name"`
	Date          time.Time `json:"date"`
	Close         float64   `json:"close"`
	PrevClose     *float64  `json:"prev_close"`
	ChangePercent *float64  `json:"change_percent"`
	PE            *float64  `json:"pe"`
	PB            *float64  `json:"pb"`
	DivYield      *float64  `json:"div_yield"`
}

// UpsertIndexBars stores one day's index rows with the SHA-256 of the file they came from.
func (db *DB) UpsertIndexBars(ctx context.Context, rows []nseindex.Row, sourceURL, sha string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck
	n := 0
	for _, r := range rows {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO index_daily_bars (index_name, trade_date, open, high, low, close, points_change, volume, turnover_cr,
				pe, pb, div_yield, source_url, source_sha256)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (index_name, trade_date) DO UPDATE SET open = EXCLUDED.open, high = EXCLUDED.high, low = EXCLUDED.low,
				close = EXCLUDED.close, points_change = EXCLUDED.points_change, volume = EXCLUDED.volume,
				turnover_cr = EXCLUDED.turnover_cr, pe = EXCLUDED.pe, pb = EXCLUDED.pb, div_yield = EXCLUDED.div_yield,
				source_url = EXCLUDED.source_url, source_sha256 = EXCLUDED.source_sha256, retrieved_at = now()`,
			r.Name, r.Date, r.Open, r.High, r.Low, r.Close, r.PointsChange, r.Volume, r.TurnoverCr, r.PE, r.PB, r.DY, sourceURL, sha)
		if err != nil {
			return 0, err
		}
		n++
	}
	return n, tx.Commit()
}

// IndexDatesStored returns the trading dates (YYYY-MM-DD) already stored for the benchmark index in [from, to].
func (db *DB) IndexDatesStored(ctx context.Context, from, to time.Time) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `SELECT trade_date FROM index_daily_bars WHERE index_name = $1 AND trade_date BETWEEN $2 AND $3`,
		IndexNifty50, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var d time.Time
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out[d.Format("2006-01-02")] = true
	}
	return out, rows.Err()
}

// IndexCloseOnOrBefore returns the index's close on `day`, or the latest close before it (e.g. over a holiday).
func (db *DB) IndexCloseOnOrBefore(ctx context.Context, name string, day time.Time) (IndexPoint, bool, error) {
	var p IndexPoint
	err := db.QueryRowContext(ctx, `SELECT trade_date, close::float8 FROM index_daily_bars WHERE index_name = $1 AND trade_date <= $2::date ORDER BY trade_date DESC LIMIT 1`,
		name, day.Format("2006-01-02")).Scan(&p.Date, &p.Close)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	return p, err == nil, err
}

// IndexSeries returns the closes of an index between two dates, oldest first.
func (db *DB) IndexSeries(ctx context.Context, name string, from, to time.Time) ([]IndexPoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT trade_date, close::float8 FROM index_daily_bars WHERE index_name = $1 AND trade_date BETWEEN $2 AND $3 ORDER BY trade_date`,
		name, from.Format("2006-01-02"), to.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []IndexPoint
	for rows.Next() {
		var p IndexPoint
		if err := rows.Scan(&p.Date, &p.Close); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// LatestIndexSnapshots returns the newest stored day of each named index, with the previous close for the change.
func (db *DB) LatestIndexSnapshots(ctx context.Context, names []string) ([]IndexSnapshot, error) {
	var out []IndexSnapshot
	for _, n := range names {
		var s IndexSnapshot
		var prev sql.NullFloat64
		err := db.QueryRowContext(ctx, `
			SELECT index_name, trade_date, close::float8, pe::float8, pb::float8, div_yield::float8,
			       (SELECT close::float8 FROM index_daily_bars p WHERE p.index_name = b.index_name AND p.trade_date < b.trade_date ORDER BY p.trade_date DESC LIMIT 1)
			FROM index_daily_bars b WHERE index_name = $1 ORDER BY trade_date DESC LIMIT 1`, n).
			Scan(&s.Name, &s.Date, &s.Close, &s.PE, &s.PB, &s.DivYield, &prev)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if prev.Valid && prev.Float64 > 0 {
			pc := (s.Close/prev.Float64 - 1) * 100
			s.PrevClose, s.ChangePercent = &prev.Float64, &pc
		}
		out = append(out, s)
	}
	return out, nil
}
