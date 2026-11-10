package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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
	// Why the engine called it, frozen with the signal.
	Reasons    []string
	Stage      string
	Resistance float64
	ATR        float64
	Breakdown  map[string]float64
}

// RepeatCooldownDays is the quiet period (about 5 sessions) after a counted signal during which the same stock's further signals are
// treated as the same move, matching the cool-down the research study uses.
const RepeatCooldownDays = 7

// RecordBreakout stores a signal once. A second sighting of the same day's signal changes nothing. A signal for a stock whose earlier
// signal is still open, or under RepeatCooldownDays old, is stored but flagged so it counts in no statistic and no automatic paper trade.
func (db *DB) RecordBreakout(ctx context.Context, s BreakoutSignal) error {
	var lastDate *time.Time
	var lastResolved *time.Time
	err := db.QueryRowContext(ctx, `SELECT signal_date, resolved_at FROM breakout_tracking
		WHERE symbol_id = $1 AND counts_in_stats AND signal_date < $2 ORDER BY signal_date DESC LIMIT 1`, s.SymbolID, s.SignalDate.Format("2006-01-02")).Scan(&lastDate, &lastResolved)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	counts, reason := true, (*string)(nil)
	if lastDate != nil && (lastResolved == nil || s.SignalDate.Sub(*lastDate) < RepeatCooldownDays*24*time.Hour) {
		counts = false
		r := fmt.Sprintf("REPEAT_SIGNAL: the signal of %s is still open or under 5 sessions old", lastDate.Format("2006-01-02"))
		reason = &r
	}
	reasons, _ := json.Marshal(s.Reasons)
	breakdown, _ := json.Marshal(s.Breakdown)
	stage := s.Stage
	if stage == "" {
		stage = "CONFIRMED_BREAKOUT"
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO breakout_tracking (symbol_id, signal_date, signal_close, entry_low, entry_high, stop_level, target_1, target_2, structure_score, rvol, shariah_status,
			reasons, stage, resistance_level, atr, score_breakdown, counts_in_stats, excluded_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NULLIF($14, 0), NULLIF($15, 0), $16, $17, $18) ON CONFLICT (symbol_id, signal_date) DO NOTHING`,
		s.SymbolID, s.SignalDate.Format("2006-01-02"), s.SignalClose, s.EntryLow, s.EntryHigh, s.Stop, s.T1, s.T2, s.Score, s.RVOL, s.Shariah,
		string(reasons), stage, s.Resistance, s.ATR, string(breakdown), counts, reason)
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
	// Why it was called, and whether it counts in statistics.
	Reasons        []string           `json:"reasons"`
	Resistance     *float64           `json:"resistance_level"`
	ATR            *float64           `json:"atr"`
	ScoreBreakdown map[string]float64 `json:"score_breakdown"`
	CountsInStats  bool               `json:"counts_in_stats"`
	ExcludedReason *string            `json:"excluded_reason"`
	NetReturnPct   *float64           `json:"net_return_pct"` // after costs and assumed slippage, for closed trades
}

// ListTrackedBreakouts returns recent signals, newest first.
func (db *DB) ListTrackedBreakouts(ctx context.Context, limit int, symbol string) ([]TrackedBreakout, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.symbol, to_char(t.signal_date, 'YYYY-MM-DD'), t.origin, t.signal_close::float8, t.entry_low::float8, t.entry_high::float8, t.stop_level::float8,
		       t.target_1::float8, t.target_2::float8, t.structure_score::float8, t.rvol::float8, t.shariah_status, t.status, t.verdict, t.entry_at, t.entry_price::float8,
		       t.exit_at, t.exit_price::float8, t.return_pct::float8, t.best_pct::float8, t.worst_pct::float8, t.sessions, t.closed_below_at,
		       t.last_price::float8, t.last_price_at, t.nifty_return_pct::float8, t.reasons, t.resistance_level::float8, t.atr::float8, t.score_breakdown, t.counts_in_stats, t.excluded_reason
		FROM breakout_tracking t JOIN symbols s ON s.id = t.symbol_id
		WHERE ($2 = '' OR s.symbol = $2)
		ORDER BY t.signal_date DESC, s.symbol LIMIT $1`, limit, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrackedBreakout{}
	for rows.Next() {
		var t TrackedBreakout
		var reasons, breakdown []byte
		if err := rows.Scan(&t.Symbol, &t.SignalDate, &t.Origin, &t.SignalClose, &t.EntryLow, &t.EntryHigh, &t.Stop, &t.Target1, &t.Target2, &t.Score, &t.RVOL, &t.Shariah,
			&t.Status, &t.Verdict, &t.EntryAt, &t.EntryPrice, &t.ExitAt, &t.ExitPrice, &t.ReturnPct, &t.BestPct, &t.WorstPct, &t.Sessions, &t.ClosedBelowAt,
			&t.LastPrice, &t.LastPriceAt, &t.NiftyReturnPct, &reasons, &t.Resistance, &t.ATR, &breakdown, &t.CountsInStats, &t.ExcludedReason); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(reasons, &t.Reasons)
		_ = json.Unmarshal(breakdown, &t.ScoreBreakdown)
		if t.Reasons == nil {
			t.Reasons = []string{}
		}
		if t.EntryPrice != nil && t.ExitPrice != nil && (t.Verdict == "WORKED" || t.Verdict == "FAILED" || t.Verdict == "NO_FOLLOW_THROUGH") {
			v := NetReturnPct(*t.EntryPrice, *t.ExitPrice)
			t.NetReturnPct = &v
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// NetReturnPct is a trade's return after statutory and broker charges and 5 bps assumed slippage per side on a Rs 1 lakh position,
// the same basis the learner and the automatic paper trader use. Entry and exit are the traded prices.
func NetReturnPct(entry, exit float64) float64 {
	if entry <= 0 || exit <= 0 {
		return 0
	}
	qty := math.Max(1, math.Floor(100000/entry))
	buyFill, sellFill := entry*(1+0.0005), exit*(1-0.0005)
	buy, sell := qty*buyFill, qty*sellFill
	costs := CalculateDeliveryTransactionCosts("BUY", buy).TotalFees + CalculateDeliveryTransactionCosts("SELL", sell).TotalFees
	return (sell - buy - costs) / (qty * entry) * 100
}

// ScoreGroup summarises closed trades of one origin.
type ScoreGroup struct {
	Closed      int      `json:"closed"`
	Wins        int      `json:"wins"`
	AvgGrossPct *float64 `json:"avg_gross_pct"`
	AvgNetPct   *float64 `json:"avg_net_pct"`
	NetWinRate  *float64 `json:"net_win_rate_pct"`
	TotalNetInr float64  `json:"total_net_inr_on_1l_each"`
}

// BreakoutScorecard sums up tracked signals. Repeat signals (counts_in_stats = false) are excluded everywhere.
type BreakoutScorecard struct {
	Tracked       int      `json:"tracked"`
	Open          int      `json:"open"`
	Worked        int      `json:"worked"`
	Failed        int      `json:"failed"`
	Faded         int      `json:"no_follow_through"`
	NotTradeable  int      `json:"not_tradeable"`
	AvgReturnPct  *float64 `json:"avg_return_pct_closed"` // gross, kept for compatibility: before costs
	AvgExcessPct  *float64 `json:"avg_excess_vs_nifty_pct_closed"`
	WorkedRatePct *float64 `json:"worked_rate_pct"`
	// Added in P1.9.29
	RepeatExcluded int        `json:"repeat_signals_excluded"`
	Live           ScoreGroup `json:"live"`
	Replay         ScoreGroup `json:"history_replay"`
	All            ScoreGroup `json:"all"`
	Note           string     `json:"note"`
}

// BreakoutScores computes the scorecard over counted signals that have a verdict.
func (db *DB) BreakoutScores(ctx context.Context) (BreakoutScorecard, error) {
	var c BreakoutScorecard
	err := db.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE verdict = 'PENDING'), count(*) FILTER (WHERE verdict = 'WORKED'), count(*) FILTER (WHERE verdict = 'FAILED'),
		       count(*) FILTER (WHERE verdict = 'NO_FOLLOW_THROUGH'), count(*) FILTER (WHERE verdict = 'NOT_TRADEABLE'),
		       (avg(return_pct) FILTER (WHERE verdict IN ('WORKED', 'FAILED', 'NO_FOLLOW_THROUGH')))::float8,
		       (avg(return_pct - nifty_return_pct) FILTER (WHERE verdict IN ('WORKED', 'FAILED', 'NO_FOLLOW_THROUGH')))::float8
		FROM breakout_tracking WHERE counts_in_stats`).Scan(&c.Tracked, &c.Open, &c.Worked, &c.Failed, &c.Faded, &c.NotTradeable, &c.AvgReturnPct, &c.AvgExcessPct)
	if err != nil {
		return c, err
	}
	if n := c.Worked + c.Failed + c.Faded; n > 0 {
		v := float64(c.Worked) / float64(n) * 100
		c.WorkedRatePct = &v
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM breakout_tracking WHERE NOT counts_in_stats`).Scan(&c.RepeatExcluded)

	rows, err := db.QueryContext(ctx, `SELECT origin, entry_price::float8, exit_price::float8 FROM breakout_tracking
		WHERE counts_in_stats AND verdict IN ('WORKED', 'FAILED', 'NO_FOLLOW_THROUGH') AND entry_price IS NOT NULL AND exit_price IS NOT NULL`)
	if err != nil {
		return c, err
	}
	defer rows.Close()
	type acc struct {
		n, wins           int
		gross, net, total float64
	}
	groups := map[string]*acc{"LIVE": {}, "HISTORY_REPLAY": {}, "ALL": {}}
	for rows.Next() {
		var origin string
		var en, ex float64
		if err := rows.Scan(&origin, &en, &ex); err != nil {
			return c, err
		}
		g, net := (ex/en-1)*100, NetReturnPct(en, ex)
		for _, k := range []string{origin, "ALL"} {
			a := groups[k]
			a.n++
			a.gross += g
			a.net += net
			a.total += net / 100 * 100000
			if net > 0 {
				a.wins++
			}
		}
	}
	fill := func(a *acc) ScoreGroup {
		out := ScoreGroup{Closed: a.n, Wins: a.wins, TotalNetInr: math.Round(a.total)}
		if a.n > 0 {
			g, n, w := a.gross/float64(a.n), a.net/float64(a.n), float64(a.wins)/float64(a.n)*100
			out.AvgGrossPct, out.AvgNetPct, out.NetWinRate = &g, &n, &w
		}
		return out
	}
	c.Live, c.Replay, c.All = fill(groups["LIVE"]), fill(groups["HISTORY_REPLAY"]), fill(groups["ALL"])
	c.Note = "Net = after statutory and broker charges and 5 bps assumed slippage per side on Rs 1 lakh. Repeat signals of the same stock are excluded. 'History replay' signals were found by running the engine over past data, not announced live."
	return c, rows.Err()
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
