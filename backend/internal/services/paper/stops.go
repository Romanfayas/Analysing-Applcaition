// Package paper holds the pure, data-free rules the paper-trading simulator applies: how a protective stop, a profit
// target or a trailing stop fills against a sequence of bars, and how large an order's own market impact is.
//
// The fill rules are deliberately conservative and need no information from the future:
//   - a stop level is known before each bar opens, so the bar can only trigger it from what it actually printed;
//   - a bar that opens beyond the level fills at its open (a gap goes through the stop and the loss is bigger than the
//     stop distance; a gap through a target is a better price);
//   - if one bar's range touches both the stop and the target the order of events inside the bar is unknowable, so the
//     stop is assumed to have happened first;
//   - a trailing stop ratchets only from bars that have already completed.
package paper

import (
	"fmt"
	"math"
	"time"
)

// Bar is one OHLC bar. Basis says where it came from (e.g. "INTRADAY_1M", "EXCHANGE_DAILY") and is reported back.
type Bar struct {
	Start                  time.Time
	Open, High, Low, Close float64
	Basis                  string
}

// StopConfig are the protective orders to test; zero means "not set". Percentages are of the entry price.
type StopConfig struct {
	StopLossPct     float64
	TargetPct       float64
	TrailingStopPct float64
}

// Validate rejects values that cannot describe a long position's protective orders.
func (c StopConfig) Validate() error {
	for name, v := range map[string]float64{"stop_loss_pct": c.StopLossPct, "target_pct": c.TargetPct, "trailing_stop_pct": c.TrailingStopPct} {
		if math.IsNaN(v) || v < 0 || v >= 100 {
			return fmt.Errorf("%s must be between 0 and 100 (percent of the entry price)", name)
		}
	}
	return nil
}

// Any reports whether at least one protective order is set.
func (c StopConfig) Any() bool { return c.StopLossPct > 0 || c.TargetPct > 0 || c.TrailingStopPct > 0 }

// Exit reasons.
const (
	ExitStop     = "STOP_LOSS"
	ExitTrailing = "TRAILING_STOP"
	ExitTarget   = "TARGET"
)

// Outcome is what happened to a long position that was protected with the given orders.
type Outcome struct {
	Triggered bool      `json:"triggered"`
	Reason    string    `json:"reason,omitempty"`
	Price     float64   `json:"price,omitempty"`
	BarStart  time.Time `json:"bar_start,omitempty"`
	Basis     string    `json:"basis,omitempty"`
	// Gap is true when the bar opened beyond the level, so the fill is the open rather than the level.
	Gap bool `json:"gap,omitempty"`
	// Ambiguous is true when one bar touched both the stop and the target; the stop is assumed first.
	Ambiguous bool    `json:"ambiguous,omitempty"`
	StopLevel float64 `json:"stop_level,omitempty"`
	// HighWater is the highest high seen up to the trigger (or the end of the bars).
	HighWater float64 `json:"high_water"`
}

// Evaluate walks the bars (which must start after the entry) and returns the first protective order that fills.
func Evaluate(entry float64, bars []Bar, c StopConfig) Outcome {
	hwm := entry
	out := Outcome{HighWater: entry}
	if entry <= 0 || !c.Any() {
		return out
	}
	fixedStop := 0.0
	if c.StopLossPct > 0 {
		fixedStop = entry * (1 - c.StopLossPct/100)
	}
	target := math.Inf(1)
	if c.TargetPct > 0 {
		target = entry * (1 + c.TargetPct/100)
	}
	for _, b := range bars {
		stop, reason := fixedStop, ExitStop
		if c.TrailingStopPct > 0 {
			if trail := hwm * (1 - c.TrailingStopPct/100); trail > stop {
				stop, reason = trail, ExitTrailing
			}
		}
		hitStop := stop > 0 && b.Low <= stop
		hitTarget := b.High >= target
		switch {
		case stop > 0 && b.Open <= stop:
			return trigger(out, hwm, reason, b.Open, b, stop, true, false)
		case b.Open >= target:
			return trigger(out, hwm, ExitTarget, b.Open, b, stop, true, false)
		case hitStop && hitTarget:
			return trigger(out, hwm, reason, stop, b, stop, false, true)
		case hitStop:
			return trigger(out, hwm, reason, stop, b, stop, false, false)
		case hitTarget:
			return trigger(out, hwm, ExitTarget, target, b, stop, false, false)
		}
		if b.High > hwm {
			hwm = b.High
		}
	}
	out.HighWater = hwm
	return out
}

func trigger(o Outcome, hwm float64, reason string, price float64, b Bar, stop float64, gap, ambiguous bool) Outcome {
	o.Triggered, o.Reason, o.Price, o.BarStart, o.Basis = true, reason, price, b.Start, b.Basis
	o.Gap, o.Ambiguous, o.StopLevel, o.HighWater = gap, ambiguous, stop, hwm
	return o
}
