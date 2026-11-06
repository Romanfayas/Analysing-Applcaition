package handlers

import (
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func TestLatency_NotMeasuredWithoutLiveObservationsAndBackfillsExcluded(t *testing.T) {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	db := database.GetIsolatedTestDB(t)
	h := &DataQualityHandler{db: db}
	req := httptest.NewRequest("GET", "/intraday-feed", nil)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1924_latency'`) })
	_, _ = db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1924_latency'`)

	// Only measure if the isolated DB has no other live receipts.
	var other int
	require.NoError(t, db.QueryRow(`SELECT count(*) FROM ohlcv_intraday WHERE first_received_at IS NOT NULL`).Scan(&other))
	if other == 0 {
		got := measureLatency(req, h, false)
		assert.Equal(t, "NOT_MEASURED", got["status"])
		assert.Nil(t, got["median_seconds"], "never an estimate")
		assert.Nil(t, got["p99_seconds"])
	}

	var tcs int
	require.NoError(t, db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS'`).Scan(&tcs))
	ins := func(barIST, recvIST string) {
		_, err := db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source, first_received_at)
			VALUES ($1, ($2::timestamp AT TIME ZONE 'Asia/Kolkata'), 1, 1, 1, 1, 1, 1, 'p1924_latency', ($3::timestamp AT TIME ZONE 'Asia/Kolkata'))`,
			tcs, barIST, recvIST)
		require.NoError(t, err)
	}
	// First poll of the day at 10:15:30 catches up an old bar (excluded from bar receipt: it measures downtime).
	ins("2026-09-01 09:30:00", "2026-09-01 10:15:30")
	ins("2026-09-01 10:14:00", "2026-09-01 10:15:30")
	// Next poll at 10:20:30 receives three bars closing 10:20, 10:19 and 10:18: 30 s, 90 s and 150 s old.
	ins("2026-09-01 10:19:00", "2026-09-01 10:20:30")
	ins("2026-09-01 10:18:00", "2026-09-01 10:20:30")
	ins("2026-09-01 10:17:00", "2026-09-01 10:20:30")
	// The still-forming 10:20 minute, also returned at 10:20:30, closes at 10:21: excluded from both metrics.
	ins("2026-09-01 10:20:00", "2026-09-01 10:20:30")
	// A recovery backfill the next morning must not count as a live observation.
	ins("2026-09-01 11:00:00", "2026-09-02 10:00:00")

	if other == 0 {
		bar := measureLatency(req, h, false)
		assert.Equal(t, 3, bar["observations"], "the first poll (and its 45-minute-old catch-up bar) is excluded")
		assert.Equal(t, "MEASURED_LOW_SAMPLE", bar["status"])
		assert.InDelta(t, 90.0, bar["median_seconds"], 1e-6)
		poll := measureLatency(req, h, true)
		assert.Equal(t, 2, poll["observations"], "two polls")
		assert.InDelta(t, 30.0, poll["median_seconds"], 1e-6, "freshness is the newest bar's age at each poll")
	}
}
