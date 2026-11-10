package autopaper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/plans"
)

// fake is an in-memory Store with a ledger that behaves like the real one for cash.
type fake struct {
	sigs     []database.AutoSignal
	bars     map[int][]plans.Bar
	sessions []string
	acts     []database.AutoAction
	nextID   int64
	cash     float64
	cursor   string
	trades   []fakeTrade
	failFill int // fail this many fills (after the decision is stored) to simulate a crash
	uptrend  map[string]bool
}
type fakeTrade struct {
	id     int
	symbol int
	side   string
	shares int
	price  float64
	at     time.Time
}

func newFake() *fake {
	return &fake{bars: map[int][]plans.Bar{}, cash: 1200000, uptrend: map[string]bool{}}
}

func (f *fake) Signals(context.Context) ([]database.AutoSignal, error) { return f.sigs, nil }
func (f *fake) Sessions(_ context.Context, after string) ([]string, error) {
	var out []string
	for _, d := range f.sessions {
		if d > after {
			out = append(out, d)
		}
	}
	return out, nil
}
func (f *fake) Bars(_ context.Context, sym int, after, upTo string) ([]plans.Bar, error) {
	var out []plans.Bar
	for _, b := range f.bars[sym] {
		if b.Date > after && b.Date <= upTo {
			out = append(out, b)
		}
	}
	return out, nil
}
func (f *fake) UptrendAt(_ context.Context, day string) (bool, bool, error) {
	v, ok := f.uptrend[day]
	return v, ok, nil
}
func (f *fake) Actions(context.Context, int) ([]database.AutoAction, error) {
	return append([]database.AutoAction(nil), f.acts...), nil
}
func (f *fake) SaveAction(_ context.Context, a database.AutoAction) (int64, bool, error) {
	for _, x := range f.acts {
		if x.TrackingID == a.TrackingID && x.Leg == a.Leg {
			return 0, false, nil
		}
	}
	f.nextID++
	a.ID = f.nextID
	f.acts = append(f.acts, a)
	return a.ID, true, nil
}
func (f *fake) SetActionTrade(_ context.Context, id int64, tid int) error {
	for i := range f.acts {
		if f.acts[i].ID == id {
			f.acts[i].TradeID = &tid
		}
	}
	return nil
}
func (f *fake) FindTrade(_ context.Context, _, sym int, side string, at time.Time) (int, error) {
	for _, t := range f.trades {
		if t.symbol == sym && t.side == side && t.at.Equal(at) {
			return t.id, nil
		}
	}
	return 0, nil
}
func (f *fake) Fill(_ context.Context, _, sym int, side string, shares int, price float64, at time.Time, _ string) (int, error) {
	if f.failFill > 0 {
		f.failFill--
		return 0, errors.New("simulated crash before the fill")
	}
	gross := float64(shares) * price
	fees := database.CalculateDeliveryTransactionCosts(side, gross).TotalFees
	if side == "BUY" {
		f.cash -= gross + fees
	} else {
		f.cash += gross - fees
	}
	id := len(f.trades) + 1
	f.trades = append(f.trades, fakeTrade{id, sym, side, shares, price, at})
	return id, nil
}
func (f *fake) Cash(context.Context, int) (float64, error)  { return f.cash, nil }
func (f *fake) Cursor(context.Context, int) (string, error) { return f.cursor, nil }
func (f *fake) SetCursor(_ context.Context, _ int, d string) error {
	if d > f.cursor {
		f.cursor = d
	}
	return nil
}

func days(from string, n int) []string {
	t, _ := time.Parse("2006-01-02", from)
	var out []string
	for i := 0; i < n; i++ {
		out = append(out, t.AddDate(0, 0, i).Format("2006-01-02"))
	}
	return out
}

func flat(day string, px float64) plans.Bar {
	return plans.Bar{Date: day, Open: px, High: px + 1, Low: px - 1, Close: px}
}

// scenario: one signal on d0 for symbol 1, target 110, stop 95, then bars.
func scenario(bars []plans.Bar) *fake {
	f := newFake()
	f.sigs = []database.AutoSignal{{ID: 1, SymbolID: 1, Symbol: "AAA", SignalDate: "2026-01-01", Origin: "LIVE", Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 2}}
	f.bars[1] = bars
	for _, b := range bars {
		f.sessions = append(f.sessions, b.Date)
	}
	return f
}

var control = Spec{PortfolioID: 7, Name: "control", Plan: plans.Menu["T1_10D"]}

func TestSignalIsEnteredNextOpenAndExitedAtTargetWithSlippageAndFees(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100), {Date: "2026-01-04", Open: 101, High: 112, Low: 100, Close: 111}})
	res, err := Process(context.Background(), f, control)
	if err != nil || res.Entries != 1 || res.Exits != 1 {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	buy, sell := f.trades[0], f.trades[1]
	if buy.side != "BUY" || buy.price != 100.05 || buy.shares != 999 { // open 100 + 5 bps; floor(100000/100.05)
		t.Fatalf("buy: %+v", buy)
	}
	if sell.side != "SELL" || sell.price != 109.95 { // target 110 - 5 bps
		t.Fatalf("sell: %+v", sell)
	}
	if buy.at.Format("2006-01-02 15:04") != "2026-01-02 09:15" || sell.at.Format("2006-01-02 15:04") != "2026-01-04 15:25" {
		t.Fatalf("times: %v %v", buy.at, sell.at)
	}
	want := 1200000 - (999*100.05 + database.CalculateDeliveryTransactionCosts("BUY", 999*100.05).TotalFees) + (999*109.95 - database.CalculateDeliveryTransactionCosts("SELL", 999*109.95).TotalFees)
	if math.Abs(f.cash-want) > 0.005 {
		t.Fatalf("cash %.2f want %.2f", f.cash, want)
	}
}

func TestRunningAgainChangesNothing(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100), {Date: "2026-01-04", Open: 101, High: 112, Low: 100, Close: 111}})
	Process(context.Background(), f, control)
	trades, acts, cash := len(f.trades), len(f.acts), f.cash
	res, err := Process(context.Background(), f, control)
	if err != nil || res.Entries+res.Exits+res.Skips != 0 || len(f.trades) != trades || len(f.acts) != acts || f.cash != cash {
		t.Fatalf("second pass must be a no-op: %+v %v", res, err)
	}
}

func TestEntryNeverHappensBeforeTheSessionAfterTheSignal(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100)})
	f.sessions = []string{"2026-01-01"} // the exchange has recorded only the signal day so far
	f.bars[1] = nil
	res, _ := Process(context.Background(), f, control)
	if res.Entries != 0 || len(f.trades) != 0 {
		t.Fatalf("must wait for the entry session: %+v", res)
	}
}

func TestOpenPositionStaysOpenAndExitsOnlyWhenTheRuleFires(t *testing.T) {
	bars := []plans.Bar{flat("2026-01-02", 100)}
	for _, d := range days("2026-01-03", 8) {
		bars = append(bars, flat(d, 100))
	}
	f := scenario(bars)
	res, _ := Process(context.Background(), f, control)
	if res.Entries != 1 || res.Exits != 0 {
		t.Fatalf("quiet market: still open, got %+v", res)
	}
	// stop hit later
	f.bars[1] = append(f.bars[1], plans.Bar{Date: "2026-01-11", Open: 94, High: 95, Low: 90, Close: 91})
	f.sessions = append(f.sessions, "2026-01-11")
	res, _ = Process(context.Background(), f, control)
	if res.Exits != 1 || f.trades[1].price != math.Round(94*(1-0.0005)*100)/100 { // gap through the stop fills at the open
		t.Fatalf("gap stop: %+v trades=%+v", res, f.trades)
	}
}

func TestSkipsAreRecordedWithReasonsAndNeverTraded(t *testing.T) {
	cases := map[string]plans.Bar{
		"OPENED_AT_OR_ABOVE_TARGET": {Date: "2026-01-02", Open: 111, High: 112, Low: 110, Close: 111},
		"OPENED_AT_OR_BELOW_STOP":   {Date: "2026-01-02", Open: 94, High: 96, Low: 93, Close: 95},
	}
	for reason, bar := range cases {
		f := scenario([]plans.Bar{bar})
		res, _ := Process(context.Background(), f, control)
		if res.Skips != 1 || len(f.trades) != 0 || f.acts[0].Reason != reason || f.acts[0].Action != "SKIPPED" {
			t.Errorf("%s: %+v acts=%+v", reason, res, f.acts)
		}
	}
}

func TestLateSignalsAreSkippedNotBackdated(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)})
	f.cursor = "2026-01-02" // the worker already processed the entry session before this signal arrived
	f.sessions = []string{"2026-01-02", "2026-01-03"}
	Process(context.Background(), f, control)
	if len(f.trades) != 0 || f.acts[0].Reason != "LATE_SIGNAL_RECORDED_AFTER_ENTRY_SESSION" {
		t.Fatalf("late signal must not trade: %+v", f.acts)
	}
}

func TestOneOpenPositionPerSymbolAndAtMostTenOpen(t *testing.T) {
	f := newFake()
	for i := 1; i <= 12; i++ {
		f.sigs = append(f.sigs, database.AutoSignal{ID: int64(i), SymbolID: i, Symbol: fmt.Sprintf("S%d", i), SignalDate: "2026-01-01", Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 2})
		f.bars[i] = []plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)}
	}
	f.sessions = []string{"2026-01-02", "2026-01-03"}
	res, _ := Process(context.Background(), f, control)
	if res.Entries != MaxOpenPositions || res.Skips != 2 {
		t.Fatalf("cap of %d: %+v", MaxOpenPositions, res)
	}
	reasons := map[string]int{}
	for _, a := range f.acts {
		if a.Action == "SKIPPED" {
			reasons[a.Reason]++
		}
	}
	if reasons["MAX_OPEN_POSITIONS_REACHED"] != 2 {
		t.Fatalf("reasons: %v", reasons)
	}
	// a second signal on a symbol already held
	f2 := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100), flat("2026-01-04", 100)})
	f2.sigs = append(f2.sigs, database.AutoSignal{ID: 2, SymbolID: 1, Symbol: "AAA", SignalDate: "2026-01-02", Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 2})
	Process(context.Background(), f2, control)
	sort.Slice(f2.acts, func(i, j int) bool { return f2.acts[i].TrackingID < f2.acts[j].TrackingID })
	if len(f2.trades) != 1 || f2.acts[len(f2.acts)-1].Reason != "SYMBOL_ALREADY_HELD" {
		t.Fatalf("symbol already held: trades=%d acts=%+v", len(f2.trades), f2.acts)
	}
}

func TestInsufficientCashIsASkipNotAnError(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)})
	f.cash = 5000
	res, err := Process(context.Background(), f, control)
	if err != nil || res.Skips != 1 || f.acts[0].Reason != "INSUFFICIENT_CASH" || len(f.trades) != 0 {
		t.Fatalf("%+v %v %+v", res, err, f.acts)
	}
}

func TestUptrendOnlyPlansSkipWeakMarkets(t *testing.T) {
	sp := control
	sp.UptrendOnly = true
	for up, want := range map[bool]string{false: "MARKET_NOT_IN_UPTREND"} {
		f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)})
		f.uptrend["2026-01-01"] = up
		Process(context.Background(), f, sp)
		if len(f.trades) != 0 || f.acts[0].Reason != want {
			t.Fatalf("up=%v: %+v", up, f.acts)
		}
	}
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)})
	Process(context.Background(), f, sp)
	if f.acts[0].Reason != "MARKET_TREND_UNKNOWN" {
		t.Fatalf("unknown trend must not be assumed up: %+v", f.acts)
	}
	f = scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100)})
	f.uptrend["2026-01-01"] = true
	Process(context.Background(), f, sp)
	if len(f.trades) != 1 {
		t.Fatalf("uptrend should trade: %+v", f.acts)
	}
}

func TestACrashBetweenDecisionAndFillIsRepairedWithoutTradingTwice(t *testing.T) {
	f := scenario([]plans.Bar{flat("2026-01-02", 100), flat("2026-01-03", 100), {Date: "2026-01-04", Open: 101, High: 112, Low: 100, Close: 111}})
	f.failFill = 1
	if _, err := Process(context.Background(), f, control); err == nil {
		t.Fatal("the simulated crash must surface as an error")
	}
	if len(f.acts) != 1 || f.acts[0].TradeID != nil || len(f.trades) != 0 {
		t.Fatalf("decision stored, no trade yet: %+v", f.acts)
	}
	for i := 0; i < 2; i++ { // restart, then restart again
		if _, err := Process(context.Background(), f, control); err != nil {
			t.Fatal(err)
		}
	}
	buys, sells := 0, 0
	for _, tr := range f.trades {
		if tr.side == "BUY" {
			buys++
		} else {
			sells++
		}
	}
	if buys != 1 || sells != 1 {
		t.Fatalf("exactly one round trip expected after recovery, got %d buys %d sells", buys, sells)
	}
}

func TestSpecFromParamsOnlyAcceptsTheMenu(t *testing.T) {
	if sp, ok := SpecFromParams(1, "x", map[string]string{"plan": "HOLD_40D", "entry": "WITHIN_1PCT", "market": "UPTREND"}); !ok || !sp.UptrendOnly || sp.MaxExtPct == nil || sp.Plan.Limit != 40 {
		t.Fatalf("%+v %v", sp, ok)
	}
	for _, bad := range []map[string]string{{"plan": "NOPE", "entry": "ANY_OPEN", "market": "ANY_MARKET"}, {"plan": "T1_10D", "entry": "X", "market": "ANY_MARKET"}, {"plan": "T1_10D", "entry": "ANY_OPEN", "market": "Y"}} {
		if _, ok := SpecFromParams(1, "x", bad); ok {
			t.Fatalf("must reject %v", bad)
		}
	}
}
