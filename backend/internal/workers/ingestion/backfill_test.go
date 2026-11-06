package ingestion

import (
	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/ipowatch"
	"github.com/halal-equity/backend/internal/providers/marketdata/nsedata"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
)

func TestPlausibleBackfillBar(t *testing.T) {
	now := time.Date(2026, 11, 2, 22, 0, 0, 0, market.Location)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 2, h, m, 0, 0, market.Location) }
	good := database.BackfillBar{Timestamp: at(10, 0), Open: 100, High: 101, Low: 99, Close: 100.5, Volume: 1000}
	assert.True(t, plausibleBackfillBar(good, now))

	bad := good
	bad.High = 98
	assert.False(t, plausibleBackfillBar(bad, now), "high below low")
	bad = good
	bad.Close = 102
	assert.False(t, plausibleBackfillBar(bad, now), "close above high")
	bad = good
	bad.Open = 0
	assert.False(t, plausibleBackfillBar(bad, now), "zero price")
	bad = good
	bad.Volume = -1
	assert.False(t, plausibleBackfillBar(bad, now), "negative volume")

	pre, post := good, good
	pre.Timestamp, post.Timestamp = at(9, 10), at(15, 30)
	assert.False(t, plausibleBackfillBar(pre, now), "before the session opens")
	assert.False(t, plausibleBackfillBar(post, now), "a bar starting at the close is outside the session")
	first, last := good, good
	first.Timestamp, last.Timestamp = at(9, 15), at(15, 25)
	assert.True(t, plausibleBackfillBar(first, now))
	assert.True(t, plausibleBackfillBar(last, now))

	sat := good
	sat.Timestamp = time.Date(2026, 10, 31, 10, 0, 0, 0, market.Location)
	assert.False(t, plausibleBackfillBar(sat, now), "weekend")

	forming := good
	forming.Timestamp = at(14, 0)
	assert.False(t, plausibleBackfillBar(forming, at(14, 3)), "a bar that has not finished is not history")
	assert.True(t, plausibleBackfillBar(forming, at(14, 5)))
}

func TestMergeIPODetail_NSEWinsAndSMEMultiplesAreDerivedAndLabelled(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	lot := 800
	o := &nsedata.IPODetail{LotSize: &lot, Structure: nsedata.IssueStructure{FreshShares: f(3648000)}, Categories: []nsedata.CategoryBook{
		{Category: nsedata.CatQIB, Bid: f(177255200), Basis: "NSE_ONLY"},
		{Category: nsedata.CatBHNI, Bid: f(354195200), Basis: "NSE_ONLY"},
		{Category: nsedata.CatTotal, Bid: f(1289407200), Basis: "NSE_ONLY"},
	}}
	u := &ipowatch.Detail{FreshCr: f(999), IssueSizeCr: f(999), Listing: "NSE SME", Reservations: []ipowatch.Reservation{
		{Category: "Anchor Investor", Shares: f(1024000)}, {Category: "QIB (Ex. Anchor)", Shares: f(683200)},
		{Category: "NII Shares Offered", Shares: f(518400)}, {Category: "Retail Shares Offered", Shares: f(1200000)},
	}}
	m := MergeIPODetail(o, u, f(158))
	assert.InDelta(t, 57.6384, *m.FreshIssueCr, 1e-4, "NSE's share count at the upper band, not the secondary figure")
	assert.InDelta(t, 57.6384, *m.IssueSizeCr, 1e-4)
	assert.InDelta(t, 126400, *m.MinInvestment, 1e-9)
	assert.Equal(t, "NSE SME", m.ListingAt)
	assert.Equal(t, "DERIVED", m.Categories[0].Basis)
	assert.InDelta(t, 177255200.0/683200, *m.Categories[0].Times, 1e-9)
	assert.InDelta(t, 518400.0*2/3, *m.Categories[1].Offered, 1e-6, "bHNI is two-thirds of the non-institutional portion")
	assert.InDelta(t, 683200+518400+1200000, *m.Categories[2].Offered, 1e-6)

	assert.Nil(t, MergeIPODetail(nil, nil, nil).IssueSizeCr, "nothing known, nothing claimed")

	// Real case: a source listing 209 QIB shares must not produce a 327x multiple.
	tiny := MergeIPODetail(&nsedata.IPODetail{Categories: []nsedata.CategoryBook{{Category: nsedata.CatQIB, Bid: f(68400), Basis: "NSE_ONLY"}}},
		&ipowatch.Detail{Reservations: []ipowatch.Reservation{{Category: "QIB (Ex. Anchor)", Shares: f(209)}, {Category: "NII Shares Offered", Shares: f(2260800)}, {Category: "Retail Shares Offered", Shares: f(3396000)}}}, f(94))
	assert.Nil(t, tiny.Categories[0].Times)
	assert.InDelta(t, 209, *tiny.Categories[0].Offered, 1e-9)
}

func TestFinishedMinutes_DropsTheMinuteStillForming(t *testing.T) {
	now := time.Date(2026, 11, 6, 14, 6, 14, 0, time.UTC)
	got := finishedMinutes([]models.OHLCV{
		{Timestamp: now.Add(-3 * time.Minute).Truncate(time.Minute)},
		{Timestamp: now.Add(-1 * time.Minute).Truncate(time.Minute)}, // 14:05 ended at 14:06: complete
		{Timestamp: now.Truncate(time.Minute)},                       // 14:06 is still forming
		{Timestamp: now},                                             // the provider's live tick
	}, now)
	require.Len(t, got, 2)
	assert.Equal(t, 5, got[1].Timestamp.Minute())
}
