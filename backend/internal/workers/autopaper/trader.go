// Package autopaper trades the platform's own breakout signals on automatic paper portfolios, so the strategy is judged by real ledger
// results and not only by simulations. It never touches live trading and never uses a price the exchange did not record:
//
//   - a signal recorded after session t is entered at session t+1's official open, never earlier;
//   - the stop, target and time limit come from the plan under test (plans package, parity-tested against the learner's Python simulator);
//   - every decision, including every skip and its reason, is stored once (unique key), so a restart can never trade a signal twice;
//   - fills carry 5 bps assumed slippage per side and the ledger's real delivery charges.
package autopaper

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/plans"
)

const (
	// NotionalPerTrade is the position size in rupees.
	NotionalPerTrade = 100000.0
	// MaxOpenPositions caps concurrent holdings in one portfolio.
	MaxOpenPositions = 10
	// SlippageBps is the assumed slippage per side (the same assumption the learner's cost model makes).
	SlippageBps = 5.0
	entryClock  = "09:15:30"
	exitClock   = "15:25:00"
	// Control is the frozen current plan; it always gets a portfolio.
	ControlPlan   = "T1_10D"
	ControlEntry  = "ANY_OPEN"
	ControlMarket = "ANY_MARKET"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

// Store is everything the trader needs from the world; the real one is backed by PostgreSQL.
type Store interface {
	Signals(ctx context.Context) ([]database.AutoSignal, error)
	Sessions(ctx context.Context, after string) ([]string, error)
	Bars(ctx context.Context, symbolID int, after, upTo string) ([]plans.Bar, error)
	UptrendAt(ctx context.Context, day string) (up, known bool, err error)
	Actions(ctx context.Context, pid int) ([]database.AutoAction, error)
	SaveAction(ctx context.Context, a database.AutoAction) (id int64, inserted bool, err error)
	SetActionTrade(ctx context.Context, actionID int64, tradeID int) error
	FindTrade(ctx context.Context, pid, symbolID int, side string, at time.Time) (int, error)
	Fill(ctx context.Context, pid, symbolID int, side string, shares int, price float64, at time.Time, barDate string) (tradeID int, err error)
	Cash(ctx context.Context, pid int) (float64, error)
	Cursor(ctx context.Context, pid int) (string, error)
	SetCursor(ctx context.Context, pid int, day string) error
}

// Spec is one automatic portfolio's plan.
type Spec struct {
	PortfolioID int
	Name        string
	Plan        plans.Plan
	MaxExtPct   *float64
	UptrendOnly bool
}

// SpecFromParams builds a Spec from a learner version's params {plan, entry, market}; ok is false if the menu does not know them.
func SpecFromParams(pid int, name string, p map[string]string) (Spec, bool) {
	pl, ok := plans.Menu[p["plan"]]
	if !ok {
		return Spec{}, false
	}
	ext, ok := plans.EntryFilters[p["entry"]]
	if !ok {
		return Spec{}, false
	}
	switch p["market"] {
	case "ANY_MARKET", "UPTREND":
	default:
		return Spec{}, false
	}
	return Spec{PortfolioID: pid, Name: name, Plan: pl, MaxExtPct: ext, UptrendOnly: p["market"] == "UPTREND"}, true
}

// Result counts what one pass did.
type Result struct{ Entries, Exits, Skips, Sessions int }

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func clock(day, hhmmss string) time.Time {
	t, _ := time.ParseInLocation("2006-01-02 15:04:05", day+" "+hhmmss, ist)
	return t
}

// Process brings one portfolio up to date with every session the exchange has recorded. It is idempotent and resumable.
func Process(ctx context.Context, st Store, sp Spec) (Result, error) {
	var res Result
	sigs, err := st.Signals(ctx)
	if err != nil {
		return res, err
	}
	if len(sigs) == 0 {
		return res, nil
	}
	cursor, err := st.Cursor(ctx, sp.PortfolioID)
	if err != nil {
		return res, err
	}
	acts, err := st.Actions(ctx, sp.PortfolioID)
	if err != nil {
		return res, err
	}
	type key struct {
		id  int64
		leg string
	}
	have := map[key]database.AutoAction{}
	for _, a := range acts {
		have[key{a.TrackingID, a.Leg}] = a
	}
	// Finish anything a previous run decided but did not complete (decision stored, ledger fill or link missing).
	bySignal := map[int64]database.AutoSignal{}
	for _, sg := range sigs {
		bySignal[sg.ID] = sg
	}
	for _, a := range acts {
		sg, known := bySignal[a.TrackingID]
		if !known || a.Action != "FILLED" || a.TradeID != nil || a.Price == nil || a.Shares == nil {
			continue
		}
		side, clk := "BUY", entryClock
		if a.Leg == "EXIT" {
			side, clk = "SELL", exitClock
		}
		fixed, err := place(ctx, st, sp.PortfolioID, sg, a.Leg, side, a.SessionDate, clock(a.SessionDate, clk), *a.Price, *a.Shares, a.Reason)
		if err != nil {
			return res, err
		}
		have[key{a.TrackingID, a.Leg}] = fixed
	}
	after := cursor
	if after == "" {
		after = sigs[0].SignalDate // nothing can happen before the first signal
	}
	sessions, err := st.Sessions(ctx, after)
	if err != nil {
		return res, err
	}
	sort.Strings(sessions)

	// repeat reads state fresh each session so same-day exits free cash and slots for same-day entries
	for _, day := range sessions {
		res.Sessions++
		// ---- 1. exits for positions held
		for _, sg := range sigs {
			e, held := have[key{sg.ID, "ENTRY"}]
			if !held || e.Action != "FILLED" || e.TradeID == nil {
				continue
			}
			if _, done := have[key{sg.ID, "EXIT"}]; done {
				continue
			}
			bars, err := st.Bars(ctx, sg.SymbolID, sg.SignalDate, day)
			if err != nil {
				return res, err
			}
			out := plans.Simulate(plans.Signal{Stop: sg.Stop, T1: sg.T1, T2: sg.T2, EntryHigh: sg.EntryHigh, ATR: sg.ATR}, bars, sp.Plan, sp.MaxExtPct)
			if !out.Entered || !out.Exited {
				continue
			}
			exitDay := bars[out.ExitIdx].Date
			px := round2(out.ExitPx * (1 - SlippageBps/10000))
			a, err := place(ctx, st, sp.PortfolioID, sg, "EXIT", "SELL", exitDay, clock(exitDay, exitClock), px, *e.Shares, out.Reason)
			if err != nil {
				return res, err
			}
			have[key{sg.ID, "EXIT"}] = a
			res.Exits++
			log.Info().Str("portfolio", sp.Name).Str("symbol", sg.Symbol).Str("day", exitDay).Str("why", out.Reason).Float64("price", px).Msg("auto paper exit")
		}
		// ---- 2. entries for signals whose entry session is today
		for _, sg := range sigs {
			if _, done := have[key{sg.ID, "ENTRY"}]; done {
				continue
			}
			bars, err := st.Bars(ctx, sg.SymbolID, sg.SignalDate, day)
			if err != nil {
				return res, err
			}
			if len(bars) == 0 {
				continue // its entry session has not happened yet
			}
			entryDay := bars[0].Date
			skip := func(reason string) error {
				a, err := skipAction(ctx, st, sp.PortfolioID, sg.ID, "ENTRY", entryDay, reason)
				if err != nil {
					return err
				}
				have[key{sg.ID, "ENTRY"}] = a
				res.Skips++
				return nil
			}
			if entryDay < day { // the signal reached us after its entry session had already passed
				if err := skip("LATE_SIGNAL_RECORDED_AFTER_ENTRY_SESSION"); err != nil {
					return res, err
				}
				continue
			}
			if sp.UptrendOnly {
				up, known, err := st.UptrendAt(ctx, sg.SignalDate)
				if err != nil {
					return res, err
				}
				if !known {
					if err := skip("MARKET_TREND_UNKNOWN"); err != nil {
						return res, err
					}
					continue
				}
				if !up {
					if err := skip("MARKET_NOT_IN_UPTREND"); err != nil {
						return res, err
					}
					continue
				}
			}
			out := plans.Simulate(plans.Signal{Stop: sg.Stop, T1: sg.T1, T2: sg.T2, EntryHigh: sg.EntryHigh, ATR: sg.ATR}, bars, sp.Plan, sp.MaxExtPct)
			if !out.Entered {
				if err := skip(out.SkipReason); err != nil {
					return res, err
				}
				continue
			}
			held, open := 0, 0
			for _, o := range sigs {
				if e2, ok := have[key{o.ID, "ENTRY"}]; ok && e2.Action == "FILLED" {
					if _, closed := have[key{o.ID, "EXIT"}]; !closed {
						open++
						if o.SymbolID == sg.SymbolID {
							held++
						}
					}
				}
			}
			if held > 0 {
				if err := skip("SYMBOL_ALREADY_HELD"); err != nil {
					return res, err
				}
				continue
			}
			if open >= MaxOpenPositions {
				if err := skip("MAX_OPEN_POSITIONS_REACHED"); err != nil {
					return res, err
				}
				continue
			}
			fill := round2(out.EntryPx * (1 + SlippageBps/10000))
			shares := int(math.Floor(NotionalPerTrade / fill))
			if shares < 1 {
				if err := skip("PRICE_ABOVE_POSITION_SIZE"); err != nil {
					return res, err
				}
				continue
			}
			cash, err := st.Cash(ctx, sp.PortfolioID)
			if err != nil {
				return res, err
			}
			if need := float64(shares)*fill + database.CalculateDeliveryTransactionCosts("BUY", float64(shares)*fill).TotalFees; need > cash {
				if err := skip("INSUFFICIENT_CASH"); err != nil {
					return res, err
				}
				continue
			}
			a, err := place(ctx, st, sp.PortfolioID, sg, "ENTRY", "BUY", entryDay, clock(entryDay, entryClock), fill, shares, "")
			if err != nil {
				return res, err
			}
			have[key{sg.ID, "ENTRY"}] = a
			res.Entries++
			log.Info().Str("portfolio", sp.Name).Str("symbol", sg.Symbol).Str("day", entryDay).Float64("price", fill).Int("shares", shares).Msg("auto paper entry")
		}
		if err := st.SetCursor(ctx, sp.PortfolioID, day); err != nil {
			return res, err
		}
	}
	return res, nil
}

func skipAction(ctx context.Context, st Store, pid int, tracking int64, leg, day, reason string) (database.AutoAction, error) {
	a := database.AutoAction{PortfolioID: pid, TrackingID: tracking, Leg: leg, SessionDate: day, Action: "SKIPPED", Reason: reason}
	if _, _, err := st.SaveAction(ctx, a); err != nil {
		return a, err
	}
	return a, nil
}

// place records the decision FIRST (unique key), then fills the ledger at a deterministic time, then links the two. If the process dies
// between the steps, the next run finds the decision without a trade, looks the trade up by its fixed execution time, and finishes the job
// instead of trading again.
func place(ctx context.Context, st Store, pid int, sg database.AutoSignal, leg, side, day string, at time.Time, price float64, shares int, reason string) (database.AutoAction, error) {
	p, n := price, shares
	a := database.AutoAction{PortfolioID: pid, TrackingID: sg.ID, Leg: leg, SessionDate: day, Action: "FILLED", Reason: reason, Price: &p, Shares: &n}
	id, inserted, err := st.SaveAction(ctx, a)
	if err != nil {
		return a, err
	}
	if !inserted {
		// the decision exists from an interrupted earlier run: reuse it
		acts, err := st.Actions(ctx, pid)
		if err != nil {
			return a, err
		}
		for _, x := range acts {
			if x.TrackingID == sg.ID && x.Leg == leg {
				if x.TradeID != nil {
					return x, nil
				}
				id = x.ID
			}
		}
	}
	tid, err := st.FindTrade(ctx, pid, sg.SymbolID, side, at)
	if err != nil {
		return a, err
	}
	if tid == 0 {
		if tid, err = st.Fill(ctx, pid, sg.SymbolID, side, shares, price, at, day); err != nil {
			return a, fmt.Errorf("%s %s %s: %w", side, sg.Symbol, day, err)
		}
	}
	if err := st.SetActionTrade(ctx, id, tid); err != nil {
		return a, err
	}
	a.ID, a.TradeID = id, &tid
	return a, nil
}
