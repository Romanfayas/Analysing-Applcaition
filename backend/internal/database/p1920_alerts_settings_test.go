package database_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func newTestUser(t *testing.T, db *database.DB) *database.UserRecord {
	t.Helper()
	u, err := db.CreateUser(context.Background(), fmt.Sprintf("p1920-%s@example.test", uuid.NewString()), "Test User", "x")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id = $1`, u.ID) }) // cascades alerts/events/settings
	return u
}

func symbolID(t *testing.T, db *database.DB, code string) int {
	t.Helper()
	var id int
	require.NoError(t, db.QueryRow(`SELECT id FROM symbols WHERE symbol = $1 AND exchange = 'NSE'`, code).Scan(&id))
	return id
}

func TestP1920_3_AlertsAreOwnedByTheirUser(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	a, b := newTestUser(t, db), newTestUser(t, db)
	sid := symbolID(t, db, "TCS")

	alert, err := db.CreateAlert(ctx, a.ID, sid, "PRICE_ABOVE", 4000, "IN_APP")
	require.NoError(t, err)
	assert.Equal(t, "TCS", alert.Symbol)

	mine, _ := db.ListAlerts(ctx, a.ID)
	theirs, _ := db.ListAlerts(ctx, b.ID)
	assert.Len(t, mine, 1)
	assert.Empty(t, theirs)
	assert.NotNil(t, theirs, "no alerts serializes as [] not null")

	deleted, err := db.DeleteAlert(ctx, b.ID, alert.ID)
	require.NoError(t, err)
	assert.False(t, deleted, "another user's alert must be indistinguishable from a missing one")

	deleted, err = db.DeleteAlert(ctx, a.ID, alert.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
}

func TestP1920_4_ActiveAlertCapIsEnforced(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	u := newTestUser(t, db)
	sid := symbolID(t, db, "TCS")

	for i := 0; i < database.MaxActiveAlertsPerUser; i++ {
		_, err := db.CreateAlert(ctx, u.ID, sid, "PRICE_BELOW", float64(100+i), "IN_APP")
		require.NoError(t, err)
	}
	_, err := db.CreateAlert(ctx, u.ID, sid, "PRICE_BELOW", 1, "IN_APP")
	assert.ErrorIs(t, err, database.ErrAlertLimit)
}

func TestP1920_5_AlertsFireOnceOnFreshPricesOnly(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	u := newTestUser(t, db)
	fresh, stale := symbolID(t, db, "INFY"), symbolID(t, db, "WIPRO")

	insertBar := func(sym int, ts time.Time, close float64) {
		_, err := db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
			VALUES ($1, $2, 5, $3, $3, $3, $3, 1, 'p1920_test')`, sym, ts, close)
		require.NoError(t, err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1920_test'`) })
	insertBar(fresh, time.Now().Add(-time.Minute), 100)
	insertBar(stale, time.Now().Add(-10*24*time.Hour), 100)

	hit, _ := db.CreateAlert(ctx, u.ID, fresh, "PRICE_ABOVE", 90, "IN_APP")
	miss, _ := db.CreateAlert(ctx, u.ID, fresh, "PRICE_ABOVE", 110, "IN_APP")
	below, _ := db.CreateAlert(ctx, u.ID, fresh, "PRICE_BELOW", 105, "TELEGRAM")
	staleAlert, _ := db.CreateAlert(ctx, u.ID, stale, "PRICE_ABOVE", 90, "IN_APP")

	fired, err := db.EvaluatePriceAlerts(ctx)
	require.NoError(t, err)

	ids := map[int64]string{}
	for _, f := range fired {
		ids[f.Event.AlertID] = f.Channel
	}
	assert.Contains(t, ids, hit.ID)
	assert.Equal(t, "TELEGRAM", ids[below.ID])
	assert.NotContains(t, ids, miss.ID, "threshold not crossed")
	assert.NotContains(t, ids, staleAlert.ID, "a 10-day-old price must not fire as if live")

	again, err := db.EvaluatePriceAlerts(ctx)
	require.NoError(t, err)
	for _, f := range again {
		assert.NotEqual(t, hit.ID, f.Event.AlertID, "an alert fires at most once")
	}

	events, err := db.ListAlertEvents(ctx, u.ID, 10)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, 100.0, events[0].ObservedValue)
}

func TestP1920_6_SettingsDefaultThenPersist(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	u := newTestUser(t, db)

	s, err := db.GetUserSettings(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, database.DefaultUserSettings, s)

	want := database.UserSettings{DefaultBenchmark: "SENSEX", DefaultRiskPct: 2.5, DisplayTimezone: "UTC"}
	require.NoError(t, db.SaveUserSettings(ctx, u.ID, want))
	got, err := db.GetUserSettings(ctx, u.ID)
	require.NoError(t, err)
	assert.Equal(t, want, got)

	// The database itself refuses out-of-range values, independent of API validation.
	err = db.SaveUserSettings(ctx, u.ID, database.UserSettings{DefaultBenchmark: "SENSEX", DefaultRiskPct: 50, DisplayTimezone: "UTC"})
	assert.Error(t, err)
}

func TestP1920_7_IPOUpsertReportsInsertRefreshesAndNeverDowngrades(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	name := "P1920 Test IPO " + uuid.NewString()
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM ipos WHERE company_name = $1`, name) })

	sym, src := "P1920T", "TEST"
	lo := 78.0
	rec := database.IPORecord{CompanyName: name, Symbol: &sym, Exchange: "NSE", Status: "UPCOMING", Source: &src}

	_, inserted, err := db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)
	assert.True(t, inserted)

	rec.Status, rec.PriceBandLow = "OPEN", &lo
	_, inserted, err = db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)
	assert.False(t, inserted, "a refresh must not be reported as a new IPO")

	var status string
	var gotLo *float64
	require.NoError(t, db.QueryRow(`SELECT status, price_band_low::float8 FROM ipos WHERE company_name = $1`, name).Scan(&status, &gotLo))
	assert.Equal(t, "OPEN", status)
	require.NotNil(t, gotLo)
	assert.Equal(t, 78.0, *gotLo)

	// Every lifecycle status the ingestion can emit must fit the column (CLOSED_AWAITING_LISTING once overflowed it).
	for _, st := range []string{"UPCOMING", "OPEN", "CLOSED_AWAITING_LISTING"} {
		rec.Status = st
		_, _, err = db.UpsertIPOReportingInsert(ctx, rec)
		require.NoError(t, err, st)
	}

	_, err = db.Exec(`UPDATE ipos SET status = 'HISTORICAL_LISTED' WHERE company_name = $1`, name)
	require.NoError(t, err)
	rec.Status = "UPCOMING"
	_, _, err = db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)
	require.NoError(t, db.QueryRow(`SELECT status FROM ipos WHERE company_name = $1`, name).Scan(&status))
	assert.Equal(t, "HISTORICAL_LISTED", status, "a listed IPO must not be reopened by the live feed")

	_, _, err = db.UpsertIPOReportingInsert(ctx, database.IPORecord{CompanyName: "  ", Exchange: "NSE", Status: "UPCOMING"})
	assert.Error(t, err, "blank company names are refused by the database")
}
