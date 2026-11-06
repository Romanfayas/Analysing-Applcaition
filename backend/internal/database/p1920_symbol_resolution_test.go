package database_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/halal-equity/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestP1920_1_RuntimeRoleResolvesCanonicalSymbolsWithoutWriting guards the regression where
// intraday ingestion silently stored zero candles for nine days: UpsertSymbol issued an
// INSERT ... ON CONFLICT DO UPDATE on `symbols`, which the least-privilege runtime role
// (SELECT-only on the canonical universe) is refused with SQLSTATE 42501.
func TestP1920_1_RuntimeRoleResolvesCanonicalSymbolsWithoutWriting(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	var wantID int
	require.NoError(t, appDB.QueryRowContext(ctx,
		"SELECT id FROM symbols WHERE symbol = 'RELIANCE' AND exchange = 'NSE'").Scan(&wantID))

	gotID, err := appDB.UpsertSymbol(ctx, "RELIANCE", "Different Display Name", "NSE")
	require.NoError(t, err, "runtime role must be able to resolve an existing canonical symbol")
	assert.Equal(t, wantID, gotID)

	var name string
	require.NoError(t, appDB.QueryRowContext(ctx, "SELECT name FROM symbols WHERE id = $1", wantID).Scan(&name))
	assert.NotEqual(t, "Different Display Name", name, "canonical symbol metadata must not be ingestion-mutable")
}

// TestP1920_2_RuntimeRoleCannotRegisterNewInstruments proves the canonical universe stays closed:
// an unknown ticker is refused instead of being silently registered.
func TestP1920_2_RuntimeRoleCannotRegisterNewInstruments(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	_, err := appDB.UpsertSymbol(ctx, "NOTAREALTICKER", "Not Real", "NSE")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canonical universe")

	var n int
	require.NoError(t, appDB.QueryRowContext(ctx,
		"SELECT count(*) FROM symbols WHERE symbol = 'NOTAREALTICKER'").Scan(&n))
	assert.Equal(t, 0, n)
}

// The runtime role is least-privilege by design; features that pass under the superuser test
// connection can still fail in production if a grant is missing (signal_weight_configs once was).
func TestP1920_8_RuntimeRoleCanReadFrozenSignalWeights(t *testing.T) {
	appDB := getAppTestDB(t)
	var n int
	require.NoError(t, appDB.QueryRowContext(context.Background(),
		`SELECT count(*) FROM signal_weight_configs WHERE is_active`).Scan(&n))
	assert.GreaterOrEqual(t, n, 1)

	_, err := appDB.ExecContext(context.Background(), `UPDATE signal_weight_configs SET name = name`)
	assert.Error(t, err, "the frozen weights must not be writable by the runtime role")
}

func TestP1920_9_AlertLifecycleWorksUnderTheRuntimeRole(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	u, err := appDB.CreateUser(ctx, "p1920-approle-"+uuid.NewString()+"@example.test", "App Role", "x")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = appDB.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1920_approle'`)
		_, _ = appDB.Exec(`DELETE FROM users WHERE id = $1`, u.ID) // cascades alerts, events, settings, portfolio
	})

	var sid int
	require.NoError(t, appDB.QueryRow(`SELECT id FROM symbols WHERE symbol = 'ONGC' AND exchange = 'NSE'`).Scan(&sid))
	_, err = appDB.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
		VALUES ($1, now(), 5, 100, 100, 100, 100, 1, 'p1920_approle')`, sid)
	require.NoError(t, err)

	a, err := appDB.CreateAlert(ctx, u.ID, sid, "PRICE_ABOVE", 90, "IN_APP")
	require.NoError(t, err)

	fired, err := appDB.EvaluatePriceAlerts(ctx)
	require.NoError(t, err)
	found := false
	for _, f := range fired {
		found = found || f.Event.AlertID == a.ID
	}
	assert.True(t, found, "evaluator (UPDATE user_alerts + INSERT alert_events) must work as halal_app_user")

	events, err := appDB.ListAlertEvents(ctx, u.ID, 5)
	require.NoError(t, err)
	assert.Len(t, events, 1)

	require.NoError(t, appDB.SaveUserSettings(ctx, u.ID, database.DefaultUserSettings))
	_, err = appDB.GetOrCreatePortfolioForUser(ctx, u.ID)
	assert.NoError(t, err, "per-user portfolio creation must work as halal_app_user")

	_, err = appDB.Exec(`DELETE FROM alert_events WHERE user_id = $1`, u.ID)
	assert.Error(t, err, "alert_events is append-only for the runtime role")
}
