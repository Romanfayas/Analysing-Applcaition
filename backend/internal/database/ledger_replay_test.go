package database_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 10, 0, 0, 0, time.UTC) }

func TestReplayLedger_RebuildsCashCostAndRealisedPnLInDateOrder(t *testing.T) {
	rows := []database.LedgerRow{
		{ID: 2, SymbolID: 1, Symbol: "X", Type: "SELL", Shares: 10, Price: 120, Fees: 5, Date: day(5)},
		{ID: 1, SymbolID: 1, Symbol: "X", Type: "BUY", Shares: 10, Price: 100, Fees: 10, Date: day(3)},
	}
	cash, pos, realized, err := database.ReplayLedger(rows, 2000)
	require.NoError(t, err)
	assert.InDelta(t, 2000-1010+1195, cash, 1e-9)
	assert.Equal(t, 0, pos[1].Shares)
	assert.InDelta(t, 1195-1010, realized[2], 1e-9, "average cost includes the buy charges")
}

func TestReplayLedger_RefusesWhatCouldNotHaveHappened(t *testing.T) {
	_, _, _, err := database.ReplayLedger([]database.LedgerRow{
		{ID: 1, SymbolID: 1, Symbol: "X", Type: "SELL", Shares: 1, Price: 100, Date: day(1)},
		{ID: 2, SymbolID: 1, Symbol: "X", Type: "BUY", Shares: 1, Price: 100, Date: day(2)},
	}, 1000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "short selling")

	_, _, _, err = database.ReplayLedger([]database.LedgerRow{
		{ID: 1, SymbolID: 1, Symbol: "X", Type: "BUY", Shares: 10, Price: 100, Date: day(1)},
		{ID: 2, SymbolID: 2, Symbol: "Y", Type: "BUY", Shares: 1, Price: 100, Date: day(2)},
	}, 1000)
	require.Error(t, err, "the second buy has no cash left at its date")
	assert.Contains(t, err.Error(), "short of cash")
}
