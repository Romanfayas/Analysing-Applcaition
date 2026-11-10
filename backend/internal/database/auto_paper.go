package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"time"
)

// AutoSignal is a counted confirmed breakout the automatic paper trader may act on.
type AutoSignal struct {
	ID                           int64
	SymbolID                     int
	Symbol                       string
	SignalDate                   string
	Origin                       string
	Stop, T1, T2, EntryHigh, ATR float64
}

// AutoSignals lists counted signals (repeat signals are excluded), oldest first.
func (db *DB) AutoSignals(ctx context.Context) ([]AutoSignal, error) {
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.symbol_id, s.symbol, to_char(t.signal_date, 'YYYY-MM-DD'), t.origin, t.stop_level::float8, t.target_1::float8, t.target_2::float8,
		t.entry_high::float8, COALESCE(t.atr, 0)::float8 FROM breakout_tracking t JOIN symbols s ON s.id = t.symbol_id WHERE t.counts_in_stats ORDER BY t.signal_date, s.symbol, t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutoSignal
	for rows.Next() {
		var a AutoSignal
		if err := rows.Scan(&a.ID, &a.SymbolID, &a.Symbol, &a.SignalDate, &a.Origin, &a.Stop, &a.T1, &a.T2, &a.EntryHigh, &a.ATR); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ExchangeBar is one official exchange session.
type ExchangeBar struct {
	Date                   string
	Open, High, Low, Close float64
}

// ExchangeSessions returns the distinct session dates after the given date (empty = all), ascending.
func (db *DB) ExchangeSessions(ctx context.Context, after string) ([]string, error) {
	if after == "" {
		after = "1900-01-01"
	}
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT to_char(trade_date, 'YYYY-MM-DD') d FROM exchange_daily_bars WHERE trade_date > $1 ORDER BY d`, after)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// ExchangeBarsBetween returns a symbol's official daily bars with after < date <= upTo, ascending.
func (db *DB) ExchangeBarsBetween(ctx context.Context, symbolID int, after, upTo string) ([]ExchangeBar, error) {
	rows, err := db.QueryContext(ctx, `SELECT to_char(trade_date, 'YYYY-MM-DD'), open::float8, high::float8, low::float8, close::float8 FROM exchange_daily_bars
		WHERE symbol_id = $1 AND trade_date > $2 AND trade_date <= $3 AND series = 'EQ' ORDER BY trade_date`, symbolID, after, upTo)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExchangeBar
	for rows.Next() {
		var b ExchangeBar
		if err := rows.Scan(&b.Date, &b.Open, &b.High, &b.Low, &b.Close); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// NiftyUptrendAt reports whether NIFTY 50 closed above its 50-day average on or before the date. known is false with too little history.
func (db *DB) NiftyUptrendAt(ctx context.Context, day string) (up, known bool, err error) {
	var n int
	var last, avg float64
	err = db.QueryRowContext(ctx, `WITH w AS (SELECT close::float8 c FROM index_daily_bars WHERE index_name = $1 AND trade_date <= $2 ORDER BY trade_date DESC LIMIT 50)
		SELECT count(*), COALESCE((SELECT c FROM w LIMIT 1), 0), COALESCE(avg(c), 0) FROM w`, IndexNifty50, day).Scan(&n, &last, &avg)
	if err != nil || n < 50 {
		return false, false, err
	}
	return last > avg, true, nil
}

// ActivePaperStrategy returns the plan the learner promoted to active paper trading, if any.
func (db *DB) ActivePaperStrategy(ctx context.Context) (name string, params map[string]string, ok bool, err error) {
	var raw []byte
	err = db.QueryRowContext(ctx, `SELECT name, params FROM strategy_current WHERE status = 'ACTIVE_PAPER' ORDER BY status_at DESC LIMIT 1`).Scan(&name, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, false, nil
	}
	if err != nil {
		return "", nil, false, err
	}
	if err := json.Unmarshal(raw, &params); err != nil {
		return "", nil, false, err
	}
	return name, params, true, nil
}

// AutoPaperCapital funds ten Rs 1 lakh positions plus their charges, so the position cap and the trade size are consistent.
const AutoPaperCapital = 1200000

// EnsureAutoPortfolio returns the id of the automatic paper portfolio with this label, creating it (Rs 12 lakh, no owner) on first use.
func (db *DB) EnsureAutoPortfolio(ctx context.Context, label string, plan map[string]string) (int, error) {
	var id int
	err := db.QueryRowContext(ctx, `SELECT id FROM portfolios WHERE kind = 'AUTO_STRATEGY' AND label = $1`, label).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, `INSERT INTO portfolios (cash_balance, initial_capital, kind, label) VALUES ($1, $1, 'AUTO_STRATEGY', $2) RETURNING id`, float64(AutoPaperCapital), label).Scan(&id); err != nil {
		return 0, err
	}
	pj, _ := json.Marshal(plan)
	if _, err := tx.ExecContext(ctx, `INSERT INTO auto_paper_state (portfolio_id, strategy_name, plan) VALUES ($1, $2, $3)`, id, label, string(pj)); err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

// AutoCursor is the last session the portfolio has fully processed ("" if none).
func (db *DB) AutoCursor(ctx context.Context, pid int) (string, error) {
	var d sql.NullString
	err := db.QueryRowContext(ctx, `SELECT to_char(processed_through, 'YYYY-MM-DD') FROM auto_paper_state WHERE portfolio_id = $1`, pid).Scan(&d)
	return d.String, err
}

// SetAutoCursor moves the cursor forward (never back).
func (db *DB) SetAutoCursor(ctx context.Context, pid int, day string) error {
	_, err := db.ExecContext(ctx, `UPDATE auto_paper_state SET processed_through = $2, updated_at = now() WHERE portfolio_id = $1 AND (processed_through IS NULL OR processed_through < $2)`, pid, day)
	return err
}

// AutoAction is one decision on a signal.
type AutoAction struct {
	ID          int64
	PortfolioID int
	TrackingID  int64
	Leg         string // ENTRY | EXIT
	SessionDate string
	Action      string // FILLED | SKIPPED
	Reason      string
	TradeID     *int
	Price       *float64
	Shares      *int
}

// AutoActions loads every action of a portfolio.
func (db *DB) AutoActions(ctx context.Context, pid int) ([]AutoAction, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, portfolio_id, tracking_id, leg, to_char(session_date, 'YYYY-MM-DD'), action, COALESCE(reason, ''), trade_id, price::float8, shares
		FROM auto_paper_actions WHERE portfolio_id = $1 ORDER BY id`, pid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AutoAction
	for rows.Next() {
		var a AutoAction
		if err := rows.Scan(&a.ID, &a.PortfolioID, &a.TrackingID, &a.Leg, &a.SessionDate, &a.Action, &a.Reason, &a.TradeID, &a.Price, &a.Shares); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAutoAction inserts a decision once; inserted is false if it already existed (the unique key), so a retry never decides twice.
func (db *DB) SaveAutoAction(ctx context.Context, a AutoAction) (id int64, inserted bool, err error) {
	err = db.QueryRowContext(ctx, `INSERT INTO auto_paper_actions (portfolio_id, tracking_id, leg, session_date, action, reason, trade_id, price, shares)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7, $8, $9) ON CONFLICT (portfolio_id, tracking_id, leg) DO NOTHING RETURNING id`,
		a.PortfolioID, a.TrackingID, a.Leg, a.SessionDate, a.Action, a.Reason, a.TradeID, a.Price, a.Shares).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return id, err == nil, err
}

// SetAutoActionTrade links a decision to the ledger trade it produced.
func (db *DB) SetAutoActionTrade(ctx context.Context, actionID int64, tradeID int) error {
	_, err := db.ExecContext(ctx, `UPDATE auto_paper_actions SET trade_id = $2 WHERE id = $1 AND trade_id IS NULL`, actionID, tradeID)
	return err
}

// FindTradeAt finds a ledger trade by its deterministic execution time, used to finish a fill that was interrupted.
func (db *DB) FindTradeAt(ctx context.Context, pid, symbolID int, side string, at time.Time) (int, error) {
	var id int
	err := db.QueryRowContext(ctx, `SELECT id FROM portfolio_trades WHERE portfolio_id = $1 AND symbol_id = $2 AND trade_type = $3 AND trade_date = $4 ORDER BY id LIMIT 1`, pid, symbolID, side, at).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// PortfolioCash reads the cash balance.
func (db *DB) PortfolioCash(ctx context.Context, pid int) (float64, error) {
	var c float64
	err := db.QueryRowContext(ctx, `SELECT cash_balance::float8 FROM portfolios WHERE id = $1`, pid).Scan(&c)
	return c, err
}

// ---------------------------------------------------------------------------------------------- summary for the page

// AutoClosed is one finished automatic paper trade.
type AutoClosed struct {
	Symbol     string  `json:"symbol"`
	Origin     string  `json:"origin"`
	SignalDate string  `json:"signal_date"`
	EntryDate  string  `json:"entry_date"`
	ExitDate   string  `json:"exit_date"`
	Shares     int     `json:"shares"`
	EntryPrice float64 `json:"entry_price"`
	ExitPrice  float64 `json:"exit_price"`
	Fees       float64 `json:"fees"`
	NetPnl     float64 `json:"net_pnl"`
	NetPct     float64 `json:"net_pct"`
	ExitReason string  `json:"exit_reason"`
}

// AutoOpen is a position still held.
type AutoOpen struct {
	Symbol     string  `json:"symbol"`
	Shares     int     `json:"shares"`
	AvgCost    float64 `json:"avg_cost"`
	EntryDate  string  `json:"entry_date"`
	SignalDate string  `json:"signal_date"`
}

// AutoSkipped is a signal the trader decided not to trade, with the reason.
type AutoSkipped struct {
	Symbol     string `json:"symbol"`
	SignalDate string `json:"signal_date"`
	Leg        string `json:"leg"`
	Reason     string `json:"reason"`
}

// AutoSummary is one automatic portfolio with everything needed to verify it.
type AutoSummary struct {
	PortfolioID     int               `json:"portfolio_id"`
	Label           string            `json:"label"`
	Plan            map[string]string `json:"plan"`
	InitialCapital  float64           `json:"initial_capital"`
	Cash            float64           `json:"cash"`
	ProcessedTo     string            `json:"processed_through"`
	Open            []AutoOpen        `json:"open_positions"`
	Closed          []AutoClosed      `json:"closed_trades"`
	Skipped         []AutoSkipped     `json:"skipped"`
	ClosedCount     int               `json:"closed_count"`
	Wins            int               `json:"wins"`
	NetPnl          float64           `json:"net_pnl_closed"`
	TotalFees       float64           `json:"total_fees"`
	ReturnOnCapital float64           `json:"return_on_capital_pct"`
	MaxDrawdownPct  float64           `json:"max_drawdown_realized_pct"`
	ExpectedCash    float64           `json:"expected_cash_from_trades"`
	Reconciled      bool              `json:"cash_reconciles_to_ledger"`
	Note            string            `json:"note"`
}

// AutoSummaries returns every automatic portfolio's results.
func (db *DB) AutoSummaries(ctx context.Context) ([]AutoSummary, error) {
	prow, err := db.QueryContext(ctx, `SELECT p.id, COALESCE(p.label, ''), p.initial_capital::float8, p.cash_balance::float8, COALESCE(to_char(a.processed_through, 'YYYY-MM-DD'), ''), a.plan
		FROM portfolios p JOIN auto_paper_state a ON a.portfolio_id = p.id WHERE p.kind = 'AUTO_STRATEGY' ORDER BY p.id`)
	if err != nil {
		return nil, err
	}
	var out []AutoSummary
	for prow.Next() {
		var s AutoSummary
		var plan []byte
		if err := prow.Scan(&s.PortfolioID, &s.Label, &s.InitialCapital, &s.Cash, &s.ProcessedTo, &plan); err != nil {
			prow.Close()
			return nil, err
		}
		_ = json.Unmarshal(plan, &s.Plan)
		out = append(out, s)
	}
	prow.Close()
	for i := range out {
		if err := db.fillAutoSummary(ctx, &out[i]); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []AutoSummary{}
	}
	return out, nil
}

func (db *DB) fillAutoSummary(ctx context.Context, s *AutoSummary) error {
	s.Open, s.Closed, s.Skipped = []AutoOpen{}, []AutoClosed{}, []AutoSkipped{}
	rows, err := db.QueryContext(ctx, `
		SELECT sy.symbol, t.origin, to_char(t.signal_date, 'YYYY-MM-DD'), e.session_date::text, x.session_date::text, e.shares, e.price::float8, x.price::float8,
		       et.fees_paid::float8, xt.fees_paid::float8, xt.realized_pnl::float8, COALESCE(x.reason, ''), x.trade_id IS NOT NULL, e.trade_id IS NOT NULL
		FROM auto_paper_actions e
		JOIN breakout_tracking t ON t.id = e.tracking_id JOIN symbols sy ON sy.id = t.symbol_id
		LEFT JOIN auto_paper_actions x ON x.portfolio_id = e.portfolio_id AND x.tracking_id = e.tracking_id AND x.leg = 'EXIT' AND x.action = 'FILLED'
		LEFT JOIN portfolio_trades et ON et.id = e.trade_id LEFT JOIN portfolio_trades xt ON xt.id = x.trade_id
		WHERE e.portfolio_id = $1 AND e.leg = 'ENTRY' AND e.action = 'FILLED' AND e.trade_id IS NOT NULL ORDER BY e.session_date, sy.symbol`, s.PortfolioID)
	if err != nil {
		return err
	}
	var pnls []float64
	for rows.Next() {
		var sym, origin, sig, ed string
		var xd sql.NullString
		var shares int
		var ep float64
		var xp, ef, xf, rp sql.NullFloat64
		var reason string
		var hasX, hasE bool
		if err := rows.Scan(&sym, &origin, &sig, &ed, &xd, &shares, &ep, &xp, &ef, &xf, &rp, &reason, &hasX, &hasE); err != nil {
			rows.Close()
			return err
		}
		if !xd.Valid || !hasX || !rp.Valid {
			s.Open = append(s.Open, AutoOpen{Symbol: sym, Shares: shares, AvgCost: ep, EntryDate: ed, SignalDate: sig})
			continue
		}
		fees := ef.Float64 + xf.Float64
		pct := rp.Float64 / (float64(shares) * ep) * 100
		s.Closed = append(s.Closed, AutoClosed{Symbol: sym, Origin: origin, SignalDate: sig, EntryDate: ed, ExitDate: xd.String, Shares: shares, EntryPrice: ep, ExitPrice: xp.Float64,
			Fees: math.Round(fees*100) / 100, NetPnl: math.Round(rp.Float64*100) / 100, NetPct: math.Round(pct*100) / 100, ExitReason: reason})
		s.NetPnl += rp.Float64
		s.TotalFees += fees
		s.ClosedCount++
		if rp.Float64 > 0 {
			s.Wins++
		}
		pnls = append(pnls, rp.Float64)
	}
	rows.Close()
	sort.SliceStable(s.Closed, func(i, j int) bool { return s.Closed[i].ExitDate > s.Closed[j].ExitDate })
	s.NetPnl = math.Round(s.NetPnl*100) / 100
	s.TotalFees = math.Round(s.TotalFees*100) / 100
	if s.InitialCapital > 0 {
		s.ReturnOnCapital = math.Round(s.NetPnl/s.InitialCapital*10000) / 100
	}
	// realized drawdown from the chronological sequence of closed trades (oldest first)
	eq, peak, worst := s.InitialCapital, s.InitialCapital, 0.0
	chron := append([]AutoClosed(nil), s.Closed...)
	sort.SliceStable(chron, func(i, j int) bool { return chron[i].ExitDate < chron[j].ExitDate })
	for _, c := range chron {
		eq += c.NetPnl
		peak = math.Max(peak, eq)
		worst = math.Min(worst, (eq-peak)/peak*100)
	}
	s.MaxDrawdownPct = math.Round(worst*100) / 100

	sk, err := db.QueryContext(ctx, `SELECT sy.symbol, to_char(t.signal_date, 'YYYY-MM-DD'), a.leg, COALESCE(a.reason, '') FROM auto_paper_actions a JOIN breakout_tracking t ON t.id = a.tracking_id
		JOIN symbols sy ON sy.id = t.symbol_id WHERE a.portfolio_id = $1 AND a.action = 'SKIPPED' ORDER BY t.signal_date DESC, sy.symbol LIMIT 200`, s.PortfolioID)
	if err != nil {
		return err
	}
	for sk.Next() {
		var k AutoSkipped
		if err := sk.Scan(&k.Symbol, &k.SignalDate, &k.Leg, &k.Reason); err != nil {
			sk.Close()
			return err
		}
		s.Skipped = append(s.Skipped, k)
	}
	sk.Close()

	// The ledger identity, computed straight from the trades table: cash must equal capital minus buys plus sells, fees included.
	var buys, sells float64
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(sum(shares * price + fees_paid) FILTER (WHERE trade_type = 'BUY'), 0)::float8,
		COALESCE(sum(shares * price - fees_paid) FILTER (WHERE trade_type = 'SELL'), 0)::float8 FROM portfolio_trades WHERE portfolio_id = $1`, s.PortfolioID).Scan(&buys, &sells); err != nil {
		return err
	}
	s.ExpectedCash = math.Round((s.InitialCapital-buys+sells)*100) / 100
	s.Reconciled = math.Abs(s.ExpectedCash-s.Cash) < 0.02
	s.Note = "Fills use the official NSE daily bar of the session, the plan's stop/target levels (gaps fill at the open), 5 bps assumed slippage per side and the ledger's real delivery charges. Rs 1 lakh per trade, at most 10 open positions, Rs 12 lakh account."
	if s.Plan == nil {
		s.Plan = map[string]string{}
	}
	return nil
}
