package nsedata

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Captured from https://www.nseindia.com/api/ipo-current-issue (camelCase keys, share-count issueSize).
const nseSample = `[
 {"companyName":"Bombay Coated and Special Steels Limited","issueEndDate":"02-Nov-2026","issuePrice":"Rs.78 to Rs.82","issueSize":"16533012","issueStartDate":"29-Oct-2026","series":"EQ","status":"Active","symbol":"BCSSL","category":"Total","srNo":null},
 {"companyName":"SFC Environmental Technologies Limited","issueEndDate":"29-Oct-2026","issuePrice":"Rs.313 to Rs.330","issueSize":"14159744","issueStartDate":"27-Oct-2026","series":"EQ","status":"Closed","symbol":"SFCTECH","category":"Total"},
 {"companyName":"Max Engineering Ltd","issueEndDate":"","issuePrice":"","issueSize":"","issueStartDate":"","series":"EQ","status":"Upcoming","symbol":"MAXENGS"}
]`

var now = time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)

func TestParseIPOIssues_RealFeedShapeDecodesFully(t *testing.T) {
	var raw []IPOData
	require.NoError(t, json.Unmarshal([]byte(nseSample), &raw))
	// Regression: snake_case tags once decoded only symbol/series, leaving every name empty.
	require.Equal(t, "Bombay Coated and Special Steels Limited", raw[0].CompanyName)

	ipos, rejected := ParseIPOIssues(raw, now)
	require.Empty(t, rejected)
	require.Len(t, ipos, 3)

	b := ipos[0]
	assert.Equal(t, "BCSSL", b.Symbol)
	assert.Equal(t, "OPEN", b.Status)
	assert.Equal(t, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC), *b.CloseDate)
	assert.Equal(t, 78.0, *b.PriceBandLow)
	assert.Equal(t, 82.0, *b.PriceBandHigh)
	// 16,533,012 shares x Rs.82 = Rs.1,355,707,000 = 135.57 crore: never the raw share count.
	assert.InDelta(t, 135.5707, *b.IssueSizeCrore, 1e-4)

	assert.Equal(t, "CLOSED_AWAITING_LISTING", ipos[1].Status)

	m := ipos[2]
	assert.Equal(t, "UPCOMING", m.Status)
	assert.Nil(t, m.OpenDate)
	assert.Nil(t, m.PriceBandHigh)
	assert.Nil(t, m.IssueSizeCrore, "no price band means no derived size; it is not guessed")
}

func TestParseIPOIssues_RejectsNamelessAndBadDates(t *testing.T) {
	ipos, rejected := ParseIPOIssues([]IPOData{
		{CompanyName: "", Symbol: "X"},
		{CompanyName: "Y Ltd", Symbol: "Y", IssueStartDate: "31/10/2026"},
	}, now)
	assert.Empty(t, ipos)
	assert.Len(t, rejected, 2)
}

func TestParseIPOIssues_DuplicateSymbolKeepsTotalRow(t *testing.T) {
	ipos, _ := ParseIPOIssues([]IPOData{
		{CompanyName: "A Ltd", Symbol: "A", IssuePrice: "Rs.10", Category: "QIB", IssueSize: "100"},
		{CompanyName: "A Ltd", Symbol: "A", IssuePrice: "Rs.10", Category: "Total", IssueSize: "900"},
	}, now)
	require.Len(t, ipos, 1)
	assert.Equal(t, 900.0, *ipos[0].SharesOffered)
}

func TestParsePriceBand(t *testing.T) {
	lo, hi := parsePriceBand("Rs.1,865 to Rs.1,960")
	assert.Equal(t, 1865.0, *lo)
	assert.Equal(t, 1960.0, *hi)
	lo, hi = parsePriceBand("Rs.100")
	assert.Equal(t, 100.0, *lo)
	assert.Equal(t, 100.0, *hi)
	lo, hi = parsePriceBand("To be announced")
	assert.Nil(t, lo)
	assert.Nil(t, hi)
}

func TestIPOStatusFallsBackToDates(t *testing.T) {
	d := func(day int) *time.Time { t := time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC); return &t }
	assert.Equal(t, "CLOSED_AWAITING_LISTING", ipoStatus("", d(20), d(25), now))
	assert.Equal(t, "OPEN", ipoStatus("", d(30), nil, now))
	assert.Equal(t, "UPCOMING", ipoStatus("", nil, nil, now))
}

func TestBoardAndSubscriptionFromTheListFeed(t *testing.T) {
	raw := []IPOData{
		{CompanyName: "Mainboard Co", Symbol: "MB", Series: "EQ", Status: "Active", IssueStartDate: "29-Oct-2026", IssueEndDate: "02-Nov-2026",
			IssuePrice: "Rs.78 to Rs.82", IssueSize: "1000000", NoOfTime: "32.73", NoOfSharesBid: "5.4118519E8"},
		{CompanyName: "Sme Co", Symbol: "SM", Series: "SME", Status: "Active", IssueStartDate: "30-Oct-2026", IssueEndDate: "03-Nov-2026", NoOfTime: "72.97"},
		{CompanyName: "Odd Co", Symbol: "OD", Series: "N1", Status: "Active", IssueStartDate: "30-Oct-2026", IssueEndDate: "03-Nov-2026"},
	}
	ipos, rej := ParseIPOIssues(raw, now)
	require.Empty(t, rej)
	assert.Equal(t, "MAINBOARD", ipos[0].Board)
	assert.Equal(t, "SME", ipos[1].Board)
	assert.Equal(t, "", ipos[2].Board, "an unknown series is not guessed")
	assert.InDelta(t, 32.73, *ipos[0].SubscriptionTimes, 1e-9)
	assert.InDelta(t, 72.97, *ipos[1].SubscriptionTimes, 1e-9)
	assert.Nil(t, ipos[2].SubscriptionTimes)
}
