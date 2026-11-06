package database_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func TestIPO_ExpectedListingIsSEBITPlus3SkippingWeekends(t *testing.T) {
	// Close Wed 2026-11-04 -> Thu, Fri, then Mon (weekend skipped).
	got := database.ExpectedListingDate(time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC))
	require.NotNil(t, got)
	assert.Equal(t, "2026-11-09", got.Format("2006-01-02"))
	// Close Fri 2026-10-30 -> Mon, Tue, Wed.
	got = database.ExpectedListingDate(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	assert.Equal(t, "2026-11-04", got.Format("2006-01-02"))
}

func TestIPO_BoardAndListingDateAreStoredAndServed(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	name := "P1925 Board Test " + uuid.NewString()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM ipo_subscriptions WHERE ipo_id IN (SELECT id FROM ipos WHERE company_name = $1)`, name)
		_, _ = db.Exec(`DELETE FROM ipos WHERE company_name = $1`, name)
	})
	sym, src, board := "P1925B", "NSE_OFFICIAL_IPO_API", "SME"
	open, closeD := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	rec := database.IPORecord{CompanyName: name, Symbol: &sym, Exchange: "NSE", Board: &board, Source: &src, Status: "OPEN", OpenDate: &open, CloseDate: &closeD}
	id, _, err := db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)

	find := func() map[string]any {
		list, err := db.GetEnrichedIPOs(ctx, "ALL")
		require.NoError(t, err)
		for _, it := range list {
			if it["name"] == name {
				return it
			}
		}
		t.Fatal("ipo not served")
		return nil
	}
	it := find()
	assert.Equal(t, "SME", it["board"])
	assert.Equal(t, "NSE_SERIES", it["board_basis"])
	assert.Equal(t, "EXPECTED_SEBI_T_PLUS_3", it["listing_date_basis"], "no announced date yet: the SEBI timeline is shown, labelled as expected")
	require.NotNil(t, it["expected_listing_date"])
	assert.Nil(t, it["listing_date"])

	// The exchange announces the listing date: it replaces the expectation.
	ld := time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)
	rec.ListingDate, rec.Status = &ld, "CLOSED_AWAITING_LISTING"
	_, _, err = db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)
	it = find()
	assert.Equal(t, "ANNOUNCED", it["listing_date_basis"])
	assert.Nil(t, it["expected_listing_date"])

	// A refresh that omits the board must not erase it.
	rec.Board = nil
	_, _, err = db.UpsertIPOReportingInsert(ctx, rec)
	require.NoError(t, err)
	assert.Equal(t, "SME", find()["board"])

	// Subscription snapshots: stored, de-duplicated, and nothing invented for categories SME does not publish.
	total := 72.97
	snap := database.IPOSubscriptionSnapshot{Total: &total, Source: "NSE_IPO_API"}
	wrote, err := db.RecordIPOSubscription(ctx, id, &open, time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC), snap)
	require.NoError(t, err)
	assert.True(t, wrote)
	wrote, err = db.RecordIPOSubscription(ctx, id, &open, time.Date(2026, 11, 2, 11, 0, 0, 0, time.UTC), snap)
	require.NoError(t, err)
	assert.False(t, wrote, "an unchanged reading is not stored again")
	sub := find()["subscription"].(map[string]any)
	assert.InDelta(t, 72.97, sub["total"], 1e-9)
	assert.Nil(t, sub["qib"])
	assert.Nil(t, sub["retail"])

	var day int
	require.NoError(t, db.QueryRow(`SELECT day_number FROM ipo_subscriptions WHERE ipo_id = $1 ORDER BY id LIMIT 1`, id).Scan(&day))
	assert.Equal(t, 2, day, "bidding day counts working days from the open date: Fri 30 Oct = 1, Mon 2 Nov = 2")
}
