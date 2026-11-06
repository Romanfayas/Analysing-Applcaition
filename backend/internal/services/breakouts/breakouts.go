// Package breakouts answers two questions about a breakout signal, using only prices that already exist: where is the
// price now relative to the plan, and what actually happened after the signal.
package breakouts

import (
	"fmt"
	"time"

	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
)

// Levels are the plan frozen at the signal.
type Levels struct {
	EntryLow, EntryHigh, Stop, Target1, Target2 float64
}

// Live position statuses.
const (
	InZone      = "IN_ENTRY_ZONE"
	AboveZone   = "ABOVE_ENTRY_ZONE"
	BelowZone   = "BELOW_ENTRY_ZONE"
	Invalidated = "INVALIDATED"
	T1Reached   = "TARGET_1_REACHED"
	T2Reached   = "TARGET_2_REACHED"
)

// Position compares the latest price with the plan.
type Position struct {
	Price             float64   `json:"price"`
	AsOf              time.Time `json:"as_of"`
	Status            string    `json:"status"`
	Message           string    `json:"message"`
	FromZonePct       float64   `json:"from_zone_pct"`         // distance from the nearest edge of the entry zone
	RewardRiskNow     *float64  `json:"reward_risk_now"`       // to Target 1 versus to the stop, from the current price
	PlannedRewardRisk float64   `json:"planned_reward_risk"`   // the same ratio from the entry zone
	ChangeSinceScan   float64   `json:"change_since_scan_pct"` // since the close the levels were computed from
}

// Locate places a price against the plan. It says plainly when a level has already passed, because a plan drawn on
// yesterday's close is no longer the trade on offer once the price has moved.
func Locate(l Levels, price, scanClose float64, asOf time.Time) Position {
	p := Position{Price: price, AsOf: asOf}
	if scanClose > 0 {
		p.ChangeSinceScan = (price/scanClose - 1) * 100
	}
	if risk := l.EntryLow - l.Stop; risk > 0 {
		p.PlannedRewardRisk = (l.Target1 - l.EntryLow) / risk
	}
	if price > l.Stop && l.Target1 > price {
		v := (l.Target1 - price) / (price - l.Stop)
		p.RewardRiskNow = &v
	}
	switch {
	case price <= l.Stop:
		p.Status, p.Message = Invalidated, "The price is at or below the invalidation level: this setup has failed."
	case price >= l.Target2:
		p.Status, p.Message = T2Reached, "Target 2 has already been reached: the move this plan described is over."
	case price >= l.Target1:
		p.Status, p.Message = T1Reached, "Target 1 has already been reached: entering now has no planned reward left."
	case price > l.EntryHigh:
		p.FromZonePct = (price/l.EntryHigh - 1) * 100
		p.Status = AboveZone
		if p.RewardRiskNow != nil {
			p.Message = fmt.Sprintf("Already %.1f%% above the entry zone. From here the reward to Target 1 is only %.1f times the risk to the stop, against %.1f planned: do not chase.",
				p.FromZonePct, *p.RewardRiskNow, p.PlannedRewardRisk)
		}
	case price >= l.EntryLow:
		p.Status, p.Message = InZone, "Inside the entry zone now."
	default:
		p.FromZonePct = (price/l.EntryLow - 1) * 100
		p.Status, p.Message = BelowZone, fmt.Sprintf("%.1f%% below the entry zone: the level has not been reached.", -p.FromZonePct)
	}
	return p
}

// Tracking statuses.
const (
	AwaitingOpen = "AWAITING_OPEN"
	Open         = "OPEN"
	Target1Hit   = "TARGET_1_HIT"
	Target2Hit   = "TARGET_2_HIT"
	Stopped      = "STOPPED"
	Expired      = "EXPIRED"
	NoRoom       = "OPENED_ABOVE_TARGET"
)

// Verdicts: did the breakout follow through?
const (
	Pending = "PENDING"
	Worked  = "WORKED"
	Failed  = "FAILED"
	Faded   = "NO_FOLLOW_THROUGH"
	Skipped = "NOT_TRADEABLE"
)

// LimitSessions matches the historical study, so live results and history are comparable.
const LimitSessions = 10

// Progress is everything known about a signal so far.
type Progress struct {
	Status      string
	Verdict     string
	EntryDate   *time.Time
	EntryPrice  *float64
	ExitDate    *time.Time
	ExitPrice   *float64
	ReturnPct   *float64 // gross, entry to exit, or entry to the last price while open
	BestPct     *float64
	WorstPct    *float64
	Sessions    int
	Target1At   *time.Time
	Target2At   *time.Time
	StopAt      *time.Time
	ClosedBelow *time.Time // first close back under the breakout level
	LastPrice   *float64
	LastPriceAt *time.Time
}

func sessionOf(t time.Time) string { return t.In(market.Location).Format("2006-01-02") }

// Follow replays the bars after a signal the same way the historical study does: entry at the next session's open, then
// the stop and Target 1, with the stop assumed first when one bar touches both. After Target 1 it keeps watching for Target 2
// until the session limit. Bars must be oldest first and all after the signal day.
func Follow(l Levels, bars []paper.Bar, limit int) Progress {
	p := Progress{Status: AwaitingOpen, Verdict: Pending}
	if len(bars) == 0 {
		return p
	}
	entry := bars[0].Open
	t0 := bars[0].Start
	p.EntryDate, p.EntryPrice, p.Status = &t0, &entry, Open
	if l.Target1 <= entry {
		p.Status, p.Verdict = NoRoom, Skipped
	}
	high, low := bars[0].High, bars[0].Low
	sessions := map[string]bool{sessionOf(bars[0].Start): true}
	last := bars[0]
	for _, b := range bars[1:] {
		sessions[sessionOf(b.Start)] = true
		if len(sessions) > limit {
			break
		}
		last = b
		if b.High > high {
			high = b.High
		}
		if b.Low < low {
			low = b.Low
		}
		t := b.Start
		if p.Status == Open {
			switch {
			case b.Low <= l.Stop:
				px := l.Stop
				if b.Open < px {
					px = b.Open
				}
				p.StopAt, p.Status, p.Verdict, p.ExitDate, p.ExitPrice = &t, Stopped, Failed, &t, &px
			case b.High >= l.Target1:
				px := l.Target1
				if b.Open > px {
					px = b.Open
				}
				p.Target1At, p.Status, p.Verdict, p.ExitDate, p.ExitPrice = &t, Target1Hit, Worked, &t, &px
			}
		}
		if p.Status == Target1Hit && p.Target2At == nil && b.High >= l.Target2 {
			p.Target2At, p.Status = &t, Target2Hit
		}
		if p.ClosedBelow == nil && b.Close < l.EntryLow {
			p.ClosedBelow = &t
		}
	}
	p.Sessions = len(sessions)
	if p.Sessions > limit {
		p.Sessions = limit
	}
	lastPx, lastAt := last.Close, last.Start
	p.LastPrice, p.LastPriceAt = &lastPx, &lastAt
	if p.Status == Open && len(sessions) > limit {
		t := last.Start
		px := last.Close
		p.Status, p.Verdict, p.ExitDate, p.ExitPrice = Expired, Faded, &t, &px
	}
	best, worst := (high/entry-1)*100, (low/entry-1)*100
	p.BestPct, p.WorstPct = &best, &worst
	end := lastPx
	if p.ExitPrice != nil {
		end = *p.ExitPrice
	}
	ret := (end/entry - 1) * 100
	p.ReturnPct = &ret
	return p
}

// Resolved reports whether nothing further can change.
func (p Progress) Resolved() bool {
	switch p.Status {
	case Stopped, Expired, NoRoom, Target2Hit:
		return true
	}
	return false
}
