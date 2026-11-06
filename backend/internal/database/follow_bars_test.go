package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/breakouts"
	"github.com/halal-equity/backend/internal/services/market"
)

// A verdict reached on intraday bars must survive the session closing. Nothing here is ever replaced by a daily bar, even
// when a daily bar exists, so a target hit on the entry day stays hit.
func TestFollowBars_UsesIntradayEvenAfterTheCloseSoAVerdictCannotFlip(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	var tcs int
	require.NoError(t, db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1927_test'`) })
	for _, b := range []struct {
		ts         string
		o, h, l, c float64
	}{
		{"2001-03-07 09:15:00+05:30", 102, 103, 101, 102},
		{"2001-03-07 10:00:00+05:30", 102, 109, 101, 108}, // Target 1 at 108, on the entry day
		{"2001-03-07 15:00:00+05:30", 108, 108, 103, 104},
	} {
		_, err := db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
			VALUES ($1, $2, 1, $3, $4, $5, $6, 1, 'p1927_test')`, tcs, b.ts, b.o, b.h, b.l, b.c)
		require.NoError(t, err)
	}
	signal := time.Date(2001, 3, 6, 0, 0, 0, 0, time.UTC)
	after := time.Date(2001, 3, 8, 20, 0, 0, 0, market.Location)
	bars, err := db.FollowBars(ctx, tcs, signal, after)
	require.NoError(t, err)
	require.Len(t, bars, 3)
	p := breakouts.Follow(breakouts.Levels{EntryLow: 100, EntryHigh: 101, Stop: 95, Target1: 108, Target2: 115}, bars, breakouts.LimitSessions)
	assert.Equal(t, breakouts.Target1Hit, p.Status)
	assert.Equal(t, breakouts.Worked, p.Verdict)

	// Mid-session only what has finished by then is used.
	during := time.Date(2001, 3, 7, 9, 30, 0, 0, market.Location)
	bars, err = db.FollowBars(ctx, tcs, signal, during)
	require.NoError(t, err)
	assert.Len(t, bars, 1)
}
