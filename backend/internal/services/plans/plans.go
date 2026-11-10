// Package plans is the trade-plan simulator shared by the automatic paper trader. It is a line-by-line port of the rules the learning loop
// tests in Python (quant breakout_evidence._simulate), and a parity test feeds both the same bars so they cannot drift apart.
//
// Rules: a confirmed breakout on session t is entered at the NEXT session's open. The stop is the invalidation level (optionally trailed
// behind the high by a multiple of ATR), the target is Target 1, Target 2 or none, and a trade not closed by the limit is closed at that
// session's close. A bar touching both stop and target is taken as the stop. Gaps through a level fill at the open.
// Known simplification, shared with the study so results stay comparable: the entry session itself is not checked against the stop.
package plans

import "math"

// Plan is one exit plan of the pre-registered menu.
type Plan struct {
	ID       string
	Target   string  // "T1", "T2" or "" (no target)
	TrailATR float64 // 0 = no trailing stop
	Limit    int     // sessions after entry
}

// Menu mirrors breakout_evidence.PLANS.
var Menu = map[string]Plan{
	"T1_10D":         {ID: "T1_10D", Target: "T1", Limit: 10},
	"T2_20D":         {ID: "T2_20D", Target: "T2", Limit: 20},
	"TRAIL_2ATR_20D": {ID: "TRAIL_2ATR_20D", TrailATR: 2.0, Limit: 20},
	"TRAIL_3ATR_40D": {ID: "TRAIL_3ATR_40D", TrailATR: 3.0, Limit: 40},
	"HOLD_20D":       {ID: "HOLD_20D", Limit: 20},
	"HOLD_40D":       {ID: "HOLD_40D", Limit: 40},
}

// EntryFilters mirrors breakout_evidence.ENTRY_FILTERS: the maximum percent the entry open may sit above the top of the entry zone.
var EntryFilters = map[string]*float64{"ANY_OPEN": nil, "WITHIN_1PCT": ptr(1.0)}

func ptr(v float64) *float64 { return &v }

// Signal is a confirmed breakout's frozen levels.
type Signal struct {
	Stop, T1, T2, EntryHigh, ATR float64
}

// Bar is one completed daily session.
type Bar struct {
	Date                   string
	Open, High, Low, Close float64
}

// Outcome is what the plan did with a signal given the bars available so far.
type Outcome struct {
	Entered    bool
	SkipReason string // set when not entered
	EntryIdx   int    // index into bars
	EntryPx    float64
	Exited     bool
	ExitIdx    int
	ExitPx     float64
	Reason     string // STOP_LOSS | TARGET | TIME_LIMIT
}

// Simulate runs plan p on signal sig. bars[0] must be the first session AFTER the signal day and bars must be ascending and gap-free as
// the exchange recorded them. Only bars supplied are used, so calling it again with more bars never changes an earlier result.
func Simulate(sig Signal, bars []Bar, p Plan, maxExtPct *float64) Outcome {
	if len(bars) == 0 {
		return Outcome{SkipReason: "NO_ENTRY_SESSION_YET"}
	}
	entry := bars[0].Open
	if entry <= sig.Stop {
		return Outcome{SkipReason: "OPENED_AT_OR_BELOW_STOP"}
	}
	if maxExtPct != nil && sig.EntryHigh > 0 {
		if ext := (entry/sig.EntryHigh - 1) * 100; ext > *maxExtPct {
			return Outcome{SkipReason: "OPENED_TOO_FAR_ABOVE_ENTRY_ZONE"}
		}
	}
	target := math.Inf(1)
	switch p.Target {
	case "T1":
		target = sig.T1
	case "T2":
		target = sig.T2
	}
	if target <= entry {
		return Outcome{SkipReason: "OPENED_AT_OR_ABOVE_TARGET"}
	}
	if p.TrailATR > 0 && sig.ATR <= 0 {
		return Outcome{SkipReason: "NO_ATR_FOR_TRAILING_STOP"}
	}
	out := Outcome{Entered: true, EntryIdx: 0, EntryPx: entry}
	hwm := bars[0].High
	last := len(bars) - 1
	for k := 1; k <= p.Limit && k <= last; k++ {
		b := bars[k]
		stop := sig.Stop
		if p.TrailATR > 0 {
			// float64() stops the compiler fusing multiply and subtract (FMA on arm64), which would round differently from the
			// two-step arithmetic Python uses and can move a stop by one ulp, enough to change which session it triggers on.
			stop = math.Max(stop, hwm-float64(p.TrailATR*sig.ATR))
		}
		if b.Low <= stop {
			out.Exited, out.ExitIdx, out.ExitPx, out.Reason = true, k, math.Min(b.Open, stop), "STOP_LOSS"
			return out
		}
		if b.High >= target {
			out.Exited, out.ExitIdx, out.ExitPx, out.Reason = true, k, math.Max(b.Open, target), "TARGET"
			return out
		}
		hwm = math.Max(hwm, b.High)
	}
	if last >= p.Limit {
		out.Exited, out.ExitIdx, out.ExitPx, out.Reason = true, p.Limit, bars[p.Limit].Close, "TIME_LIMIT"
	}
	return out
}
