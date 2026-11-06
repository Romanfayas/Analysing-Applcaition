package database

import (
	"context"
	"encoding/json"
	"time"

	"github.com/halal-equity/backend/internal/services/breakouts"
)

// BreakoutSignal is a new confirmed breakout to record.
type BreakoutSignal struct {
	SymbolID                                       int
	SignalDate                                     time.Time
	SignalClose, EntryLow, EntryHigh, Stop, T1, T2 float64
	Score                                          *float64
	RVOL                                           float64
	Shariah                                        string
}

// RecordBreakout stores a signal once. A second sighting of the same day's signal changes nothing.
func (db *DB) RecordBreakout(ctx context.Context, s BreakoutSignal) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO breakout_tracking (symbol_id, signal_date, signal_close, entry_low, entry_high, stop_level, target_1, target_2, structure_score, rvol, shariah_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) ON CONFLICT (symbol_id, signal_date) DO NOTHING`,
		s.SymbolID, s.SignalDate.Format("2006-01-02"), s.SignalClose, s.EntryLow, s.EntryHigh, s.Stop, s.T1, s.T2, s.Score, s.RVOL, s.Shariah)
	return err
}

// OpenBreakout is a tracked signal that can still change.
type OpenBreakout struct {
	ID, SymbolID int
	SignalDate   time.Time
	Levels       breakouts.Levels
}

// OpenBreakouts lists unresolved signals.
func (db *DB) OpenBreakouts(ctx context.Context) ([]OpenBreakout, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, symbol_id, signal_date, entry_low::float8, entry_high::float8, stop_level::float8, target_1::float8, target_2::float8
		FROM breakout_tracking WHERE resolved_at IS NULL ORDER BY signal_date`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OpenBreakout
	for rows.Next() {
		var o OpenBreakout
		if err := rows.Scan(&o.ID, &o.SymbolID, &o.SignalDate, &o.Levels.EntryLow, &o.Levels.EntryHigh, &o.Levels.Stop, &o.Levels.Target1, &o.Levels.Target2); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// UpdateBreakout stores what has happened so far.
func (db *DB) UpdateBreakout(ctx context.Context, id int, p breakouts.Progress, niftyPct *float64) error {
	var resolved any
	if p.Resolved() {
		resolved = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		UPDATE breakout_tracking SET status = $2, verdict = $3, entry_at = $4, entry_price = $5, exit_at = $6, exit_price = $7, return_pct = $8,
			best_pct = $9, worst_pct = $10, sessions = $11, target_1_at = $12, target_2_at = $13, stop_at = $14, closed_below_at = $15,
			last_price = $16, last_price_at = $17, nifty_return_pct = $18, resolved_at = $19, updated_at = now() WHERE id = $1`,
		id, p.Status, p.Verdict, p.EntryDate, p.EntryPrice, p.ExitDate, p.ExitPrice, p.ReturnPct, p.BestPct, p.WorstPct, p.Sessions,
		p.Target1At, p.Target2At, p.StopAt, p.ClosedBelow, p.LastPrice, p.LastPriceAt, niftyPct, resolved)
	return err
}

// TrackedBreakout is one row of the monitor.
type TrackedBreakout struct {
	Symbol         string     `json:"symbol"`
	SignalDate     string     `json:"signal_date"`
	Origin         string     `json:"origin"`
	SignalClose    float64    `json:"signal_close"`
	EntryLow       float64    `json:"entry_low"`
	EntryHigh      float64    `json:"entry_high"`
	Stop           float64    `json:"stop_level"`
	Target1        float64    `json:"target_1"`
	Target2        float64    `json:"target_2"`
	Score          *float64   `json:"structure_score"`
	RVOL           *float64   `json:"rvol"`
	Shariah        *string    `json:"shariah_status"`
	Status         string     `json:"status"`
	Verdict        string     `json:"verdict"`
	EntryAt        *time.Time `json:"entry_at"`
	EntryPrice     *float64   `json:"entry_price"`
	ExitAt         *time.Time `json:"exit_at"`
	ExitPrice      *float64   `json:"exit_price"`
	ReturnPct      *float64   `json:"return_pct"`
	BestPct        *float64   `json:"best_pct"`
	WorstPct       *float64   `json:"worst_pct"`
	Sessions       int        `json:"sessions"`
	ClosedBelowAt  *time.Time `json:"closed_below_at"`
	LastPrice      *float64   `json:"last_price"`
	LastPriceAt    *time.Time `json:"last_price_at"`
	NiftyReturnPct *float64   `json:"nifty_return_pct"`
}

// ListTrackedBreakouts returns recent signals, newest first.
func (db *DB) ListTrackedBreakouts(ctx context.Context, limit int) ([]TrackedBreakout, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.symbol, to_char(t.signal_date, 'YYYY-MM-DD'), t.origin, t.signal_close::float8, t.entry_low::float8, t.entry_high::float8, t.stop_level::float8,
		       t.target_1::float8, t.target_2::float8, t.structure_score::float8, t.rvol::float8, t.shariah_status, t.status, t.verdict, t.entry_at, t.entry_price::float8,
		       t.exit_at, t.exit_price::float8, t.return_pct::float8, t.best_pct::float8, t.worst_pct::float8, t.sessions, t.closed_below_at,
		       t.last_price::float8, t.last_price_at, t.nifty_return_pct::float8
		FROM breakout_tracking t JOIN symbols s ON s.id = t.symbol_id
		ORDER BY t.signal_date DESC, s.symbol LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackedBreakout{}
	for rows.Next() {
		var t TrackedBreakout
		if err := rows.Scan(&t.Symbol, &t.SignalDate, &t.Origin, &t.SignalClose, &t.EntryLow, &t.EntryHigh, &t.Stop, &t.Target1, &t.Target2, &t.Score, &t.RVOL, &t.Shariah,
			&t.Status, &t.Verdict, &t.EntryAt, &t.EntryPrice, &t.ExitAt, &t.ExitPrice, &t.ReturnPct, &t.BestPct, &t.WorstPct, &t.Sessions, &t.ClosedBelowAt,
			&t.LastPrice, &t.LastPriceAt, &t.NiftyReturnPct); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// BreakoutScorecard sums up tracked signals.
type BreakoutScorecard struct {
	Tracked       int      `json:"tracked"`
	Open          int      `json:"open"`
	Worked        int      `json:"worked"`
	Failed        int      `json:"failed"`
	Faded         int      `json:"no_follow_through"`
	NotTradeable  int      `json:"not_tradeable"`
	AvgReturnPct  *float64 `json:"avg_return_pct_closed"`
	AvgExcessPct  *float64 `json:"avg_excess_vs_nifty_pct_closed"`
	WorkedRatePct *float64 `json:"worked_rate_pct"`
}

// BreakoutScores computes the scorecard over signals that have a verdict.
func (db *DB) BreakoutScores(ctx context.Context) (BreakoutScorecard, error) {
	var c BreakoutScorecard
	err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE verdict = 'PENDING'), count(*) FILTER (WHERE verdict = 'WORKED'), count(*) FILTER (WHERE verdict = 'FAILED'),
		       count(*) FILTER (WHERE verdict = 'NO_FOLLOW_THROUGH'), count(*) FILTER (WHERE verdict = 'NOT_TRADEABLE'),
		       (avg(return_pct) FILTER (WHERE verdict IN ('WORKED', 'FAILED', 'NO_FOLLOW_THROUGH')))::float8,
		       (avg(return_pct - nifty_return_pct) FILTER (WHERE verdict IN ('WORKED', 'FAILED', 'NO_FOLLOW_THROUGH')))::float8
		FROM breakout_tracking`).Scan(&c.Tracked, &c.Open, &c.Worked, &c.Failed, &c.Faded, &c.NotTradeable, &c.AvgReturnPct, &c.AvgExcessPct)
	if n := c.Worked + c.Failed + c.Faded; err == nil && n > 0 {
		v := float64(c.Worked) / float64(n) * 100
		c.WorkedRatePct = &v
	}
	return c, err
}

// LatestResearchRun returns the newest stored result of a research kind, or nil.
func (db *DB) LatestResearchRun(ctx context.Context, kind string) (map[string]any, *time.Time, error) {
	var raw []byte
	var at time.Time
	err := db.QueryRowContext(ctx, `SELECT result, created_at FROM research_runs WHERE kind = $1 ORDER BY id DESC LIMIT 1`, kind).Scan(&raw, &at)
	if err != nil {
		return nil, nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, nil, err
	}
	return out, &at, nil
}
