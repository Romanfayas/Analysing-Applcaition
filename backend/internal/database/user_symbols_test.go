package database_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func TestUserSymbols_AddIsIdempotentAndNeverTouchesCanonicalRows(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	ticker := "ZZTEST" + uuid.NewString()[:6]
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM symbols WHERE symbol = $1`, ticker) })
	owner := uuid.New()

	id, created, err := db.AddUserSymbol(ctx, ticker, "Test Co", owner)
	require.NoError(t, err)
	assert.True(t, created)
	require.NoError(t, db.SetOnboardStatus(ctx, id, database.OnboardStatus{State: database.OnboardRunning,
		Steps: []database.OnboardStep{{Name: "daily_history", OK: true, Detail: "10 daily bars"}}}))

	again, created, err := db.AddUserSymbol(ctx, ticker, "Different Name", owner)
	require.NoError(t, err)
	assert.False(t, created, "adding twice must not create a second row")
	assert.Equal(t, id, again)
	var name string
	require.NoError(t, db.QueryRow(`SELECT name FROM symbols WHERE id = $1`, id).Scan(&name))
	assert.Equal(t, "Test Co", name, "the first registration wins")

	st, userAdded, err := db.GetOnboardStatus(ctx, ticker)
	require.NoError(t, err)
	assert.True(t, userAdded)
	require.NotNil(t, st)
	assert.Equal(t, database.OnboardRunning, st.State)
	require.Len(t, st.Steps, 1)

	added, err := db.UserAddedSymbols(ctx)
	require.NoError(t, err)
	assert.Equal(t, "Test Co", added[ticker])

	// A canonical instrument is reported as already tracked and is left exactly as it was.
	var before string
	require.NoError(t, db.QueryRow(`SELECT name FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&before))
	_, created, err = db.AddUserSymbol(ctx, "TCS", "Hijacked", owner)
	require.NoError(t, err)
	assert.False(t, created)
	var after string
	require.NoError(t, db.QueryRow(`SELECT name FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&after))
	assert.Equal(t, before, after)
	_, userAddedTCS, err := db.GetOnboardStatus(ctx, "TCS")
	require.NoError(t, err)
	assert.False(t, userAddedTCS)
}
