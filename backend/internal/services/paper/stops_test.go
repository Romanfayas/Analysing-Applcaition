package paper

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bar(day int, o, h, l, c float64) Bar {
	return Bar{Start: time.Date(2026, 10, day, 9, 15, 0, 0, time.UTC), Open: o, High: h, Low: l, Close: c, Basis: "TEST"}
}

func TestEvaluate_NoProtectiveOrdersNeverTriggers(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 100, 200, 1, 100)}, StopConfig{})
	assert.False(t, out.Triggered)
}

func TestEvaluate_StopFillsAtTheLevelWhenTheBarTradesThroughIt(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 99, 101, 94, 96)}, StopConfig{StopLossPct: 5})
	require.True(t, out.Triggered)
	assert.Equal(t, ExitStop, out.Reason)
	assert.InDelta(t, 95, out.Price, 1e-9)
	assert.False(t, out.Gap)
}

func TestEvaluate_GapDownFillsAtTheOpenNotTheStop(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 90, 92, 88, 91)}, StopConfig{StopLossPct: 5})
	require.True(t, out.Triggered)
	assert.InDelta(t, 90, out.Price, 1e-9, "a gap through the stop loses more than the stop distance")
	assert.True(t, out.Gap)
}

func TestEvaluate_TargetAndGapUp(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 100, 111, 99, 108)}, StopConfig{StopLossPct: 5, TargetPct: 10})
	require.True(t, out.Triggered)
	assert.Equal(t, ExitTarget, out.Reason)
	assert.InDelta(t, 110, out.Price, 1e-9)

	out = Evaluate(100, []Bar{bar(1, 115, 120, 114, 118)}, StopConfig{TargetPct: 10})
	assert.InDelta(t, 115, out.Price, 1e-9, "a gap through the target fills at the better open")
	assert.True(t, out.Gap)
}

func TestEvaluate_BarTouchingBothAssumesTheStopFirst(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 100, 112, 93, 100)}, StopConfig{StopLossPct: 5, TargetPct: 10})
	require.True(t, out.Triggered)
	assert.Equal(t, ExitStop, out.Reason, "the order inside the bar is unknowable, so the pessimistic outcome is used")
	assert.True(t, out.Ambiguous)
}

func TestEvaluate_TrailingStopRatchetsOnlyFromCompletedBars(t *testing.T) {
	bars := []Bar{
		bar(1, 100, 110, 99, 109),  // runs up: high water 110, stop becomes 99 (10% trail)
		bar(2, 109, 125, 108, 124), // runs up: high water 125, stop becomes 112.5
		bar(3, 124, 124, 112, 113), // low 112 <= 112.5 -> trailing stop fills at 112.5
	}
	out := Evaluate(100, bars, StopConfig{TrailingStopPct: 10})
	require.True(t, out.Triggered)
	assert.Equal(t, ExitTrailing, out.Reason)
	assert.InDelta(t, 112.5, out.Price, 1e-9)
	assert.InDelta(t, 125, out.HighWater, 1e-9)

	// The bar that sets a new high cannot also be stopped out by the stop that high creates.
	out = Evaluate(100, []Bar{bar(1, 100, 120, 100, 101)}, StopConfig{TrailingStopPct: 10})
	assert.False(t, out.Triggered)
}

func TestEvaluate_FixedStopStillProtectsWhileTrailingIsLower(t *testing.T) {
	out := Evaluate(100, []Bar{bar(1, 100, 101, 94, 95)}, StopConfig{StopLossPct: 5, TrailingStopPct: 20})
	require.True(t, out.Triggered)
	assert.Equal(t, ExitStop, out.Reason)
}

func TestStopConfigValidation(t *testing.T) {
	assert.NoError(t, StopConfig{StopLossPct: 5, TargetPct: 10}.Validate())
	assert.Error(t, StopConfig{StopLossPct: -1}.Validate())
	assert.Error(t, StopConfig{TargetPct: 100}.Validate())
}

func TestImpactGrowsWithSizeAndVolatility(t *testing.T) {
	small := ImpactBps(2, 100, 1_000_000)
	big := ImpactBps(2, 10_000, 1_000_000)
	assert.Less(t, small, big)
	assert.InDelta(t, big/small, 10, 1e-9, "square-root law: 100x the size is 10x the impact")
	assert.Greater(t, ImpactBps(4, 10_000, 1_000_000), big)
	assert.Zero(t, ImpactBps(2, 0, 1_000_000))
	assert.Zero(t, ImpactBps(0, 100, 1_000_000))
	assert.Nil(t, NewLiquidity(0, 2, 0, 100), "no volume history means no claim")
	l := NewLiquidity(1_000_000, 2, 20, 10_000)
	require.NotNil(t, l)
	assert.InDelta(t, 1.0, l.Participation, 1e-9)
}
