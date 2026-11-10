package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// WatchFlag is an upcoming-breakout (BREAKOUT_CANDIDATE) flag raised by the live scan, frozen on the day it is raised.
type WatchFlag struct {
	SymbolID                                                       int
	FlaggedDate                                                    time.Time
	Close, Resistance, Stop, EntryLow, EntryHigh, Target1, Target2 float64
	Score                                                          *float64
	RVOL, ATR                                                      float64
	Reasons                                                        []string
	Breakdown                                                      map[string]float64
	Shariah                                                        string
}

// RecordWatchFlag stores a candidate once; a second sighting of the same day changes nothing. A flag raised while an earlier one for the same
// stock is still being watched (under 10 sessions old) is stored but does not count in statistics, so each setup counts once.
func (db *DB) RecordWatchFlag(ctx context.Context, f WatchFlag) error {
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM breakout_watch WHERE symbol_id = $1 AND counts_in_stats AND flagged_date < $2 AND flagged_date > $2::date - 14`,
		f.SymbolID, f.FlaggedDate.Format("2006-01-02")).Scan(&n); err != nil {
		return err
	}
	reasons, _ := json.Marshal(f.Reasons)
	breakdown, _ := json.Marshal(f.Breakdown)
	_, err := db.ExecContext(ctx, `
		INSERT INTO breakout_watch (symbol_id, flagged_date, origin, flagged_close, resistance_level, stop_level, entry_low, entry_high, target_1, target_2,
			structure_score, rvol, atr, reasons, score_breakdown, shariah_status, counts_in_stats)
		VALUES ($1, $2, 'LIVE', $3, $4, $5, NULLIF($6, 0), NULLIF($7, 0), NULLIF($8, 0), NULLIF($9, 0), $10, $11, NULLIF($12, 0), $13, $14, $15, $16)
		ON CONFLICT (symbol_id, flagged_date) DO NOTHING`,
		f.SymbolID, f.FlaggedDate.Format("2006-01-02"), f.Close, f.Resistance, f.Stop, f.EntryLow, f.EntryHigh, f.Target1, f.Target2, f.Score, f.RVOL, f.ATR,
		string(reasons), string(breakdown), f.Shariah, n == 0)
	return err
}

// WatchRow is one upcoming-breakout flag and what happened to it.
type WatchRow struct {
	Symbol           string             `json:"symbol"`
	FlaggedDate      string             `json:"flagged_date"`
	Origin           string             `json:"origin"`
	FlaggedClose     float64            `json:"flagged_close"`
	Resistance       float64            `json:"resistance_level"`
	Stop             float64            `json:"stop_level"`
	EntryLow         *float64           `json:"entry_low"`
	EntryHigh        *float64           `json:"entry_high"`
	Target1          *float64           `json:"target_1"`
	Target2          *float64           `json:"target_2"`
	Score            *float64           `json:"structure_score"`
	RVOL             *float64           `json:"rvol"`
	Reasons          []string           `json:"reasons"`
	ScoreBreakdown   map[string]float64 `json:"score_breakdown"`
	Shariah          *string            `json:"shariah_status"`
	Status           string             `json:"status"`
	SessionsElapsed  int                `json:"sessions_elapsed"`
	LastClose        *float64           `json:"last_close"`
	BrokeOutSession  *string            `json:"broke_out_session"`
	SessionsToBreak  *int               `json:"sessions_to_breakout"`
	Ret5             *float64           `json:"ret_5d_pct"`
	Ret10            *float64           `json:"ret_10d_pct"`
	MaxGain          *float64           `json:"max_gain_pct"`
	MaxDrawdown      *float64           `json:"max_drawdown_pct"`
	CountsInStats    bool               `json:"counts_in_stats"`
	DistanceToResPct *float64           `json:"distance_to_resistance_pct"`
}

// ListWatch returns flags newest first, optionally for one symbol and/or one status.
func (db *DB) ListWatch(ctx context.Context, symbol, status string, limit int) ([]WatchRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.symbol, to_char(w.flagged_date, 'YYYY-MM-DD'), w.origin, w.flagged_close::float8, w.resistance_level::float8, w.stop_level::float8,
		       w.entry_low::float8, w.entry_high::float8, w.target_1::float8, w.target_2::float8, w.structure_score::float8, w.rvol::float8, w.reasons, w.score_breakdown,
		       w.shariah_status, w.status, w.sessions_elapsed, w.last_close::float8, to_char(w.broke_out_session, 'YYYY-MM-DD'), w.sessions_to_breakout,
		       w.ret_5d_pct::float8, w.ret_10d_pct::float8, w.max_gain_pct::float8, w.max_drawdown_pct::float8, w.counts_in_stats
		FROM breakout_watch w JOIN symbols s ON s.id = w.symbol_id
		WHERE ($1 = '' OR s.symbol = $1) AND ($2 = '' OR w.status = $2)
		ORDER BY w.flagged_date DESC, s.symbol LIMIT $3`, symbol, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WatchRow{}
	for rows.Next() {
		var w WatchRow
		var reasons, breakdown []byte
		if err := rows.Scan(&w.Symbol, &w.FlaggedDate, &w.Origin, &w.FlaggedClose, &w.Resistance, &w.Stop, &w.EntryLow, &w.EntryHigh, &w.Target1, &w.Target2, &w.Score, &w.RVOL,
			&reasons, &breakdown, &w.Shariah, &w.Status, &w.SessionsElapsed, &w.LastClose, &w.BrokeOutSession, &w.SessionsToBreak, &w.Ret5, &w.Ret10, &w.MaxGain, &w.MaxDrawdown,
			&w.CountsInStats); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reasons, &w.Reasons)
		_ = json.Unmarshal(breakdown, &w.ScoreBreakdown)
		if w.Reasons == nil {
			w.Reasons = []string{}
		}
		if w.LastClose != nil && *w.LastClose > 0 && w.Resistance > 0 {
			d := (w.Resistance / *w.LastClose - 1) * 100
			w.DistanceToResPct = &d
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WatchScorecard summarises counted flags that have an outcome.
type WatchScorecard struct {
	Watching    int      `json:"watching"`
	BrokeOut    int      `json:"broke_out"`
	Invalidated int      `json:"invalidated"`
	Expired     int      `json:"expired"`
	HitRatePct  *float64 `json:"hit_rate_pct"`
	Avg10dPct   *float64 `json:"avg_10d_move_pct"`
	Excluded    int      `json:"repeat_flags_excluded"`
	Note        string   `json:"note"`
}

// WatchScores computes the scorecard for the live and the replayed flags separately plus together.
func (db *DB) WatchScores(ctx context.Context, origin string) (WatchScorecard, error) {
	var c WatchScorecard
	err := db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE status = 'WATCHING'), count(*) FILTER (WHERE status = 'BROKE_OUT'), count(*) FILTER (WHERE status = 'INVALIDATED'),
		       count(*) FILTER (WHERE status = 'EXPIRED'), (avg(ret_10d_pct) FILTER (WHERE status <> 'WATCHING'))::float8
		FROM breakout_watch WHERE counts_in_stats AND ($1 = '' OR origin = $1)`, origin).Scan(&c.Watching, &c.BrokeOut, &c.Invalidated, &c.Expired, &c.Avg10dPct)
	if err != nil {
		return c, err
	}
	if n := c.BrokeOut + c.Invalidated + c.Expired; n > 0 {
		v := float64(c.BrokeOut) / float64(n) * 100
		c.HitRatePct = &v
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM breakout_watch WHERE NOT counts_in_stats AND ($1 = '' OR origin = $1)`, origin).Scan(&c.Excluded)
	c.Note = "Hit = the engine's own CONFIRMED_BREAKOUT followed within 10 sessions. It measures whether the engine foresees its own label, not profit: see the 10-day move."
	return c, nil
}

// StockStats returns the newest per-stock test result stored by the nightly run, or nil.
func (db *DB) StockStats(ctx context.Context, symbol string) (map[string]any, *string, error) {
	var raw []byte
	var asOf string
	err := db.QueryRowContext(ctx, `SELECT l.result, to_char(l.as_of, 'YYYY-MM-DD') FROM stock_signal_stats_latest l JOIN symbols s ON s.id = l.symbol_id WHERE s.symbol = $1`, symbol).Scan(&raw, &asOf)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, err
	}
	return out, &asOf, nil
}
