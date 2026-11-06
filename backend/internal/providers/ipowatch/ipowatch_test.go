package ipowatch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const listPage = `<table><tr class="row-1"><th>Company</th><th>GMP*</th><th>Trend</th><th>Price Band</th><th>Est. Gain</th><th>Date</th></tr>
<tr class="row-2"><td class="column-1"><a href="https://ipowatch.in/vedant-poly-fabs-ipo-gmp-today/" target="_blank">Vedant Poly Fabs</a> <br> <span style="color: red;">Closed</span></td><td><strong>₹1</strong></td><td>🔴</td><td>₹94</td><td>₹95 (1.06%)</td><td>2-4 Nov</td></tr>
<tr><td><a href="https://ipowatch.in/antala-industries-ipo-gmp-today/">Antala Industries</a> <span>Open</span></td><td><strong>₹0</strong></td><td>🟡</td><td>₹66</td><td>₹66 (0.00%)</td><td>29-5 Nov</td></tr></table>
<table><tr><th>Unrelated</th><th>Table</th></tr><tr><td>a</td><td>b</td></tr></table>`

func TestParseGMP(t *testing.T) {
	rows := ParseGMP(listPage)
	require.Len(t, rows, 2)
	assert.Equal(t, "Vedant Poly Fabs", rows[0].Name)
	assert.Equal(t, "Closed", rows[0].Status)
	assert.InDelta(t, 1, *rows[0].GMP, 1e-9)
	assert.InDelta(t, 94, *rows[0].Price, 1e-9)
	assert.InDelta(t, 1.06, *rows[0].GainPct, 1e-9)
	assert.Equal(t, "https://ipowatch.in/vedant-poly-fabs-ipo/", rows[0].DetailURL)
	assert.Zero(t, *rows[1].GMP, "a published GMP of zero is a real value")
}

const detailPage = `<table><tr><td>IPO Price Band</td><td>₹88 to ₹94 Per Share</td></tr><tr><td>Issue Size</td><td>Approx ₹56.68 Crores</td></tr>
<tr><td>Fresh Issue</td><td>Approx ₹51.01 Crores</td></tr><tr><td>Offer for Sale:</td><td>Approx [.] Equity Shares</td></tr><tr><td>IPO Listing</td><td>NSE SME</td></tr></table>
<table><tr><th>Investor Category</th><th>Share Offered</th><th>Shares (%)</th></tr><tr><td>Anchor Investor</td><td>10,24,000 Shares</td><td>28.07%</td></tr><tr><td>QIB (Ex. Anchor)</td><td>6,83,200 Shares</td><td>18.73%</td></tr></table>
<table><tr><td>Application</td><td>Lot Size</td><td>Shares</td><td>Amount</td></tr><tr><td>Retail Minimum</td><td>2</td><td>1600</td><td>₹2,52,800</td></tr></table>
<table><tr><td>IPO Open Date:</td><td>October 30, 2026</td></tr><tr><td>Basis of Allotment:</td><td>November 4, 2026</td></tr><tr><td>IPO Listing Date:</td><td>November 6, 2026</td></tr></table>`

func TestParseDetail_FindsTablesByContentAndHidesPlaceholders(t *testing.T) {
	d := ParseDetail(detailPage, "u")
	assert.Equal(t, "NSE SME", d.Listing)
	assert.InDelta(t, 56.68, *d.IssueSizeCr, 1e-9)
	assert.InDelta(t, 51.01, *d.FreshCr, 1e-9)
	assert.Empty(t, d.OFS, `"[.]" means not published, so nothing is shown`)
	assert.Nil(t, d.OFSCr)
	require.Len(t, d.Reservations, 2)
	assert.InDelta(t, 683200, *d.Reservations[1].Shares, 1e-9)
	assert.InDelta(t, 18.73, *d.Reservations[1].Percent, 1e-9)
	require.Len(t, d.LotTiers, 1)
	assert.Equal(t, "1600", d.LotTiers[0].Shares)
	require.NotNil(t, d.ListingDate)
	assert.Equal(t, "2026-11-06", d.ListingDate.Format("2006-01-02"))
}

func TestSameCompany(t *testing.T) {
	assert.True(t, SameCompany("HGS (India)", "HGS (India) Limited"))
	assert.True(t, SameCompany("Maxwell Engineering", "Maxwell Engineering Solutions Limited"))
	assert.True(t, SameCompany("Bombay Coated and Special Steels", "Bombay Coated and Special Steels Limited"))
	assert.False(t, SameCompany("Tata Motors", "Tata Steel Limited"))
	assert.False(t, SameCompany("Paras", "Paras Healthcare Limited"), "a single word is too weak to match on")
	assert.False(t, SameCompany("", "x"))
}

func TestDetailURLFrom_HandlesBothLinkStyles(t *testing.T) {
	assert.Equal(t, "https://ipowatch.in/vedant-poly-fabs-ipo/", DetailURLFrom("https://ipowatch.in/vedant-poly-fabs-ipo-gmp-today/"))
	assert.Equal(t, "https://ipowatch.in/bombay-coated-special-steels-ipo/", DetailURLFrom(absolute("/bombay-coated-special-steels-ipo-gmp-grey-market-premium/")))
	assert.Equal(t, "https://ipowatch.in/already-ipo/", DetailURLFrom("https://ipowatch.in/already-ipo/"))
}

func TestParseBand(t *testing.T) {
	lo, hi := ParseBand("₹88 to ₹94 Per Share")
	assert.InDelta(t, 88, *lo, 1e-9)
	assert.InDelta(t, 94, *hi, 1e-9)
	lo, hi = ParseBand("not announced")
	assert.Nil(t, lo)
	assert.Nil(t, hi)
}
