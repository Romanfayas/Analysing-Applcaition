package breakouts

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
)

// The real EICHERMOT plan from the Signals page.
var eicher = Levels{EntryLow: 7569.50, EntryHigh: 7629.29, Stop: 7355.98, Target1: 7889.78, Target2: 8210.06}

func TestLocate_AnEntryZoneThatHasPassedIsCalledOut(t *testing.T) {
	p := Locate(eicher, 7690.10, 7420, time.Now())
	assert.Equal(t, AboveZone, p.Status)
	require.NotNil(t, p.RewardRiskNow)
	assert.InDelta(t, 0.6, *p.RewardRiskNow, 0.05, "the planned 1.5R is about 0.6R from the price it actually reached")
	assert.InDelta(t, 1.5, p.PlannedRewardRisk, 0.01)
	assert.InDelta(t, 3.64, p.ChangeSinceScan, 0.01)
	assert.Contains(t, p.Message, "do not chase")
}

func TestLocate_EveryOtherPosition(t *testing.T) {
	for price, want := range map[float64]string{7600: InZone, 7500: BelowZone, 7300: Invalidated, 7900: T1Reached, 8300: T2Reached} {
		assert.Equal(t, want, Locate(eicher, price, 7420, time.Now()).Status, price)
	}
	assert.Nil(t, Locate(eicher, 7900, 7420, time.Now()).RewardRiskNow, "no reward left to quote")
}

func day(d int) time.Time { return time.Date(2026, 10, d, 9, 15, 0, 0, market.Location) }

func bar(d int, o, h, l, c float64) paper.Bar {
	return paper.Bar{Start: day(d), Open: o, High: h, Low: l, Close: c}
}

var plan = Levels{EntryLow: 100, EntryHigh: 101, Stop: 95, Target1: 108, Target2: 115}

func TestFollow_NothingYet(t *testing.T) {
	p := Follow(plan, nil, 10)
	assert.Equal(t, AwaitingOpen, p.Status)
	assert.Equal(t, Pending, p.Verdict)
}

func TestFollow_EntryIsTheNextOpenAndTheTargetCountsAsWorked(t *testing.T) {
	p := Follow(plan, []paper.Bar{bar(1, 102, 104, 101, 103), bar(2, 103, 109, 102, 108)}, 10)
	assert.InDelta(t, 102, *p.EntryPrice, 1e-9)
	assert.Equal(t, Target1Hit, p.Status)
	assert.Equal(t, Worked, p.Verdict)
	assert.InDelta(t, 108, *p.ExitPrice, 1e-9)
	assert.False(t, p.Resolved(), "still watching for Target 2")
	p = Follow(plan, []paper.Bar{bar(1, 102, 104, 101, 103), bar(2, 103, 109, 102, 108), bar(5, 108, 116, 107, 115)}, 10)
	assert.Equal(t, Target2Hit, p.Status)
	assert.True(t, p.Resolved())
	assert.InDelta(t, 13.73, *p.BestPct, 0.01)
}

func TestFollow_StopAssumedFirstAndGapFillsAtTheOpen(t *testing.T) {
	p := Follow(plan, []paper.Bar{bar(1, 102, 103, 101, 102), bar(2, 102, 110, 94, 100)}, 10)
	assert.Equal(t, Stopped, p.Status)
	assert.Equal(t, Failed, p.Verdict)
	assert.InDelta(t, 95, *p.ExitPrice, 1e-9)
	p = Follow(plan, []paper.Bar{bar(1, 102, 103, 101, 102), bar(2, 90, 92, 88, 91)}, 10)
	assert.InDelta(t, 90, *p.ExitPrice, 1e-9, "a gap through the stop fills at the open")
}

func TestFollow_ClosingBackBelowTheBreakoutLevelIsRecorded(t *testing.T) {
	p := Follow(plan, []paper.Bar{bar(1, 102, 103, 101, 102), bar(2, 101, 102, 98, 99)}, 10)
	require.NotNil(t, p.ClosedBelow)
	assert.Equal(t, Open, p.Status)
}

func TestFollow_ExpiresAfterTheSessionLimitWithoutFollowThrough(t *testing.T) {
	bars := []paper.Bar{}
	for d := 1; d <= 12; d++ {
		bars = append(bars, bar(d, 102, 103, 101, 102))
	}
	p := Follow(plan, bars, 10)
	assert.Equal(t, Expired, p.Status)
	assert.Equal(t, Faded, p.Verdict)
	assert.Equal(t, 10, p.Sessions)
	assert.True(t, p.Resolved())
}

func TestFollow_OpeningAboveTheTargetIsNotATrade(t *testing.T) {
	p := Follow(plan, []paper.Bar{bar(1, 109, 110, 108, 109)}, 10)
	assert.Equal(t, NoRoom, p.Status)
	assert.Equal(t, Skipped, p.Verdict)
}
