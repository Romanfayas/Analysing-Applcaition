package nsedata

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Trimmed from the real NSE responses for BCSSL (mainboard) and MAXENGS (SME) on 2026-11-02.
const mainboardBids = `{"bidDetails":[
 {"category":"Qualified Institutional Buyers(QIBs)","noOfSharesOffered":"4798588","noOfTime":"16.428095514763925","noOfsharesBid":"78831662","srNo":"1"},
 {"category":"Mutual funds","noOfsharesBid":"121940","srNo":"1(c)"},
 {"category":"Non Institutional Investors","noOfSharesOffered":"3520327","noOfTime":"82.33571767622723","noOfsharesBid":"289848650","srNo":"2"},
 {"category":"Retail Individual Investors(RIIs)","noOfSharesOffered":"8214097","noOfTime":"21.001076320379465","noOfsharesBid":"172504878","srNo":"3"},
 {"category":"Total","noOfSharesOffered":"1.6533012E7","noOfTime":"32.733611395189214","noOfsharesBid":"5.4118519E8"}]}`

const smeBids = `{"bidDetails":[
 {"category":"Qualified Institutional Buyers(QIBs)","noOfshareBid":"0","noofapplication":"0","srNo":"1"},
 {"category":"Non Institutional Investors","noOfshareBid":"60809600","noofapplication":"12867","srNo":"2"},
 {"category":"Individual Investors (IND category bidding for 2 Lots)","noOfshareBid":"130652800","noofapplication":"81658","srNo":"3"},
 {"category":"Total","noOfshareBid":"191462400","noofapplication":"94525"}]}`

func TestParseBidDetailsMainboardHasCategoryMultiples(t *testing.T) {
	s, err := ParseBidDetails([]byte(mainboardBids))
	require.NoError(t, err)
	assert.InDelta(t, 16.428, *s.QIB, 1e-3)
	assert.InDelta(t, 82.336, *s.NII, 1e-3)
	assert.InDelta(t, 21.001, *s.Retail, 1e-3)
	assert.InDelta(t, 32.734, *s.Total, 1e-3)
	assert.InDelta(t, 5.4118519e8, *s.SharesBid, 1)
}

func TestParseBidDetailsSMEPublishesNoMultiplesSoNoneAreInvented(t *testing.T) {
	s, err := ParseBidDetails([]byte(smeBids))
	require.NoError(t, err)
	assert.Nil(t, s.QIB)
	assert.Nil(t, s.NII)
	assert.Nil(t, s.Retail)
	assert.Nil(t, s.Total, "the multiple comes from the list feed for SME issues")
	assert.InDelta(t, 191462400, *s.SharesBid, 1)

	_, err = ParseBidDetails([]byte(`{"bidDetails":[]}`))
	assert.Error(t, err)
}

func TestParsePastIssuesKeepsRecentEquityAndSMEOnly(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	got := ParsePastIssues([]PastIssue{
		{Company: "EAAA India Alternatives Limited", Symbol: "EDELALTS", SecurityType: "EQ", StartDate: "26-OCT-2026", EndDate: "28-OCT-2026", ListingDate: "02-NOV-2026", PriceRange: "Rs.1585 to Rs.1665", IssuePrice: "  1665"},
		{Company: "M. K. Sons Fine Jewels Limited", Symbol: "MKSONS", SecurityType: "EQ", StartDate: "28-OCT-2026", EndDate: "30-OCT-2026", ListingDate: "-", PriceRange: "Rs.153 to Rs.162", IssuePrice: "-"},
		{Company: "Old Co", Symbol: "OLD", SecurityType: "EQ", StartDate: "01-JAN-2024", EndDate: "03-JAN-2024", ListingDate: "08-JAN-2024"},
		{Company: "A Bond", Symbol: "BOND", SecurityType: "DEBT", StartDate: "28-OCT-2026", EndDate: "30-OCT-2026", ListingDate: "-"},
	}, now, 45*24*time.Hour)
	require.Len(t, got, 2)
	assert.Equal(t, "MAINBOARD", got[0].Board)
	assert.Equal(t, "2026-11-02", got[0].ListingDate.Format("2006-01-02"))
	assert.InDelta(t, 1665, *got[0].IssuePrice, 1e-9)
	assert.Nil(t, got[1].ListingDate, "'-' means the exchange has not announced a listing date")
	assert.Nil(t, got[1].IssuePrice)
}

func TestParseIssueStructure_RealNSESentences(t *testing.T) {
	s := ParseIssueStructure(`Initial Public Offer comprising of Fresh Issue of up to 23,26,000 equity shares and Offer for Sale of up to 5,54,000 Equity Shares (including Market Maker portion of 1,45,200 Equity Shares and Anchor Allocation of 8,13,600 Equity shares)`)
	require.NotNil(t, s.FreshShares)
	assert.InDelta(t, 2326000, *s.FreshShares, 0.1)
	require.NotNil(t, s.OFSShares)
	assert.InDelta(t, 554000, *s.OFSShares, 0.1)
	assert.InDelta(t, 813600, *s.AnchorShares, 0.1)
	assert.InDelta(t, 145200, *s.MarketMakerShares, 0.1)

	m := ParseIssueStructure(`Initial Public Offer comprising of Fresh Issue aggregating upto Rs. 1,830.57 million (including Anchor reservation portion of 65,97,500 Equity Shares)`)
	require.NotNil(t, m.FreshAmountCr)
	assert.InDelta(t, 183.057, *m.FreshAmountCr, 0.001)
	assert.Nil(t, m.OFSShares, "no offer for sale in this issue, and none is invented")
	assert.Nil(t, m.OFSAmountCr)
	assert.InDelta(t, 6597500, *m.AnchorShares, 0.1)

	f := ParseIssueStructure(`Initial Public Offer comprising of Fresh issue of up to 36,48,000 equity shares (including Market Maker portion of 2,22,400 Equity Shares and Anchor Allocation 10,24,000 Equity Shares)`)
	assert.InDelta(t, 3648000, *f.FreshShares, 0.1, "the anchor bracket must not be read as the fresh issue")
}

func TestParseIPODetail_PrefersTheConsolidatedBookAndSplitsHNI(t *testing.T) {
	body := []byte(`{"metaInfo":{"industry":"Iron & Steel Products"},
	 "issueInfo":{"dataList":[{"title":null,"value":"x"},{"title":"Issue Size","value":"\"Initial Public Offer comprising of Fresh Issue aggregating upto Rs. 1,830.57 million\""},
	  {"title":"Bid Lot","value":"182 Equity Shares and in multiples thereof"},{"title":"Name of the Registrar","value":"KFin Technologies Limited"},
	  {"title":"Red Herring Prospectus","value":"https://nsearchives.nseindia.com/content/ipo/RHP_BCSSL.zip"}]},
	 "activeCat":{"updateTime":"Updated as on 02-Nov-2026 19:00:00","dataList":[
	  {"category":"Category","noOfShareOffered":"No.of shares","noOfSharesBid":"x","noOfTotalMeant":"y","srNo":"Sr.No."},
	  {"category":"QIBs","noOfShareOffered":"4798588","noOfSharesBid":"170330524","noOfTotalMeant":"35.49","srNo":"1"},
	  {"category":"FII","noOfShareOffered":"","noOfSharesBid":"36768550","noOfTotalMeant":"","srNo":"1(a)"},
	  {"category":"NII","noOfShareOffered":"3520327","noOfSharesBid":"557336598","noOfTotalMeant":"158.31","srNo":"2"},
	  {"category":"NII>10L","noOfShareOffered":"2346885","noOfSharesBid":"430709916","noOfTotalMeant":"183.52","srNo":"2.1"},
	  {"category":"NII<10L","noOfShareOffered":"1173442","noOfSharesBid":"126626682","noOfTotalMeant":"107.91","srNo":"2.2"},
	  {"category":"RII","noOfShareOffered":"8214097","noOfSharesBid":"277083898","noOfTotalMeant":"33.73","srNo":"3"},
	  {"category":"Total","noOfShareOffered":"1.6533012E7","noOfSharesBid":"1.00475102E9","noOfTotalMeant":"60.77","srNo":null}]},
	 "bidDetails":[{"category":"QIB","noOfSharesOffered":"4798588","noOfTime":"16.42","noOfsharesBid":"78831662","srNo":"1"},
	  {"category":"Total","noOfTime":"32.73","noOfsharesBid":"5.4E8"}]}`)
	d, err := ParseIPODetail(body)
	require.NoError(t, err)
	byCat := map[string]CategoryBook{}
	for _, c := range d.Categories {
		byCat[c.Category] = c
	}
	require.Len(t, d.Categories, 6)
	assert.InDelta(t, 183.52, *byCat[CatBHNI].Times, 1e-9)
	assert.InDelta(t, 107.91, *byCat[CatSHNI].Times, 1e-9)
	assert.InDelta(t, 60.77, *d.Subscription.Total, 1e-9, "the consolidated total, not NSE's own 32.73x window")
	assert.Equal(t, "NSE_CONSOLIDATED", byCat[CatQIB].Basis)
	assert.Equal(t, 182, *d.LotSize)
	assert.Equal(t, "Iron & Steel Products", d.Industry)
	assert.Equal(t, "KFin Technologies Limited", d.Registrar)
	assert.Equal(t, "02-Nov-2026 19:00:00", d.BookUpdatedAt)
}

func TestParseIPODetail_SMEPublishesBidsButNoSharesOffered(t *testing.T) {
	d, err := ParseIPODetail([]byte(`{"activeCat":{"dataList":[{"category":"QIB","noOfShareOffered":"0","noOfSharesBid":"177255200","noOfTotalMeant":"0.00","srNo":"1"}]},
	 "bidDetails":[{"category":"QIBs","noOfshareBid":"177255200","noofapplication":"178","srNo":"1"},
	  {"category":"II","noOfshareBid":"667324800","srNo":"3"},{"category":"Total","noOfshareBid":"1289407200"}]}`))
	require.NoError(t, err)
	require.Len(t, d.Categories, 3)
	assert.Nil(t, d.Categories[0].Offered, "NSE gives no shares offered for SME; zero is not a real value")
	assert.Nil(t, d.Categories[0].Times)
	assert.Equal(t, "NSE_ONLY", d.Categories[0].Basis)
	assert.InDelta(t, 667324800, *d.Categories[1].Bid, 0.1)
}
