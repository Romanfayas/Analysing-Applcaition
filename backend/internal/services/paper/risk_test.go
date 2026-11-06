package paper

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseState() State {
	return State{IsBuy: true, OrderCost: 100_000, SymbolCost: 0, BookEquity: 1_000_000, InitialCapital: 1_000_000}
}

func TestCheck_SellsAreNeverBlocked(t *testing.T) {
	s := baseState()
	s.IsBuy, s.OrdersToday, s.RealizedToday, s.OrderCost = false, 999, -900_000, 5_000_000
	assert.Nil(t, Check(DefaultLimits, s))
}

func TestCheck_PositionCapCountsWhatIsAlreadyHeld(t *testing.T) {
	s := baseState()
	s.OrderCost = 250_000
	assert.Nil(t, Check(DefaultLimits, s), "exactly 25% is allowed")
	s.OrderCost = 260_000
	b := Check(DefaultLimits, s)
	require.NotNil(t, b)
	assert.Equal(t, "MAX_POSITION", b.Rule)
	s.OrderCost, s.SymbolCost = 100_000, 200_000
	assert.NotNil(t, Check(DefaultLimits, s), "adding to a large position counts the existing cost too")
}

func TestCheck_DailyLossPausesBuysOnly(t *testing.T) {
	s := baseState()
	s.RealizedToday = -49_000
	assert.Nil(t, Check(DefaultLimits, s))
	s.RealizedToday = -50_000
	b := Check(DefaultLimits, s)
	require.NotNil(t, b)
	assert.Equal(t, "MAX_DAILY_LOSS", b.Rule)
}

func TestCheck_OrderFrequencyAndDisabledRules(t *testing.T) {
	s := baseState()
	s.OrdersToday = 50
	assert.Equal(t, "MAX_ORDERS_PER_DAY", Check(DefaultLimits, s).Rule)
	assert.Nil(t, Check(Limits{}, s), "zero disables every rule")
	s.OrderCost = 5_000_000
	assert.Nil(t, Check(Limits{}, s))
}

func TestLimitsFromEnv(t *testing.T) {
	t.Setenv("PAPER_RISK_MAX_POSITION_PCT", "10")
	t.Setenv("PAPER_RISK_MAX_ORDERS_PER_DAY", "0")
	t.Setenv("PAPER_RISK_MAX_DAILY_LOSS_PCT", "junk")
	l := LimitsFromEnv()
	assert.Equal(t, 10.0, l.MaxPositionPct)
	assert.Equal(t, 0, l.MaxOrdersPerDay)
	assert.Equal(t, DefaultLimits.MaxDailyLossPct, l.MaxDailyLossPct, "an unreadable value keeps the default")
}
