package evidence

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassifyRealNSESubjects(t *testing.T) {
	c := Classify(FeedAction{ExDate: "28-Oct-2024", Subject: "Bonus 1:1"})
	assert.Equal(t, ClassBonus, c.Class)
	require.NotNil(t, c.PriceFactor)
	assert.InDelta(t, 0.5, *c.PriceFactor, 1e-12)
	assert.True(t, c.PriceChanging)
	assert.Equal(t, "2024-10-28", c.Ex.Format("2006-01-02"))

	c = Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Bonus 4:1"})
	assert.InDelta(t, 0.2, *c.PriceFactor, 1e-12)

	c = Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Face Value Split (Sub-Division) - From Rs 2/- Per Share To Re 1/- Per Share"})
	assert.Equal(t, ClassSplit, c.Class)
	assert.InDelta(t, 0.5, *c.PriceFactor, 1e-12)

	c = Classify(FeedAction{ExDate: "14-Oct-2026", Subject: "Interim Dividend - Rs 12 Per Share"})
	assert.Equal(t, ClassDividend, c.Class)
	assert.Nil(t, c.PriceFactor, "dividends do not change the nominal price series")
	assert.InDelta(t, 12.0, *c.DividendPerSh, 1e-12)
	assert.False(t, c.PriceChanging)

	c = Classify(FeedAction{ExDate: "06-Jan-2025", Subject: "Demerger"})
	assert.Equal(t, ClassDemerger, c.Class)
	assert.Nil(t, c.PriceFactor, "a demerger's ratio is not in the subject; it is never guessed")
	assert.True(t, c.PriceChanging)

	assert.Equal(t, ClassBuyback, Classify(FeedAction{ExDate: "24-Nov-2023", Subject: "Buy Back"}).Class)
	assert.False(t, Classify(FeedAction{ExDate: "not a date"}).ParseableExDte)
}

func TestParserFragmentsAreRecognisedNarrowly(t *testing.T) {
	for _, s := range []string{"CML", "2026", "70875)", "LTM_RENAME"} {
		assert.True(t, IsParserFragment("OTHER", s, true), s)
	}
	assert.False(t, IsParserFragment("OTHER", "Change in Ticker Symbol from LTIM to LTM", true), "real text is not a fragment")
	assert.False(t, IsParserFragment("OTHER", "CML", false), "needs a sibling event it was split from")
	assert.False(t, IsParserFragment("CASH_DIVIDEND", "CML", true), "only OTHER events are candidates")
}

func TestRatioStepMeasuresTheVendorAdjustmentAtTheExDate(t *testing.T) {
	ex := Classify(FeedAction{ExDate: "28-Oct-2024", Subject: "Bonus 1:1"}).Ex
	var obs []ratioObs
	for i := -15; i < 15; i++ {
		r := 1.0
		if i < 0 {
			r = 0.5 // vendor halved pre-bonus prices; exchange kept the traded price
		}
		obs = append(obs, ratioObs{d: ex.AddDate(0, 0, i), ratio: r})
	}
	step, n := ratioStep(obs, ex)
	require.NotNil(t, step)
	assert.InDelta(t, 0.5, *step, 1e-12)
	assert.Equal(t, 20, n)

	only, _ := ratioStep(obs[:10], ex)
	assert.Nil(t, only, "needs observations on both sides of the ex-date")
}

func TestSelectFilingsPrefersRecordDateAndArchivePDFs(t *testing.T) {
	anns := []Announcement{ // shaped like the real RELIANCE filings of Sep-Oct 2024
		{Desc: "Press Release", Text: "RIL Board Approves 1:1 Bonus Issue", Date: "05-Sep-2024 18:41:16", File: "https://nsearchives.nseindia.com/corporate/a.pdf"},
		{Desc: "Bonus", Text: "The Board of Directors of the Company, at its meeting held today, have recommended bonus", Date: "05-Sep-2024 15:10:57", File: "https://nsearchives.nseindia.com/corporate/RELIANCE_Reg30_OutcomeofBM.pdf"},
		{Desc: "Record Date", Text: "Record date for the purpose of Bonus is 28-Oct-2024", Date: "16-Oct-2024 20:04:08", File: "https://nsearchives.nseindia.com/corporate/rd.pdf"},
		{Desc: "Record Date", Text: "Record date for Bonus", Date: "16-Oct-2024 20:04:08", File: "https://evil.example.com/rd.pdf"},
		{Desc: "Dividend", Text: "Record date for dividend", Date: "05-Aug-2024 20:10:46", File: "https://nsearchives.nseindia.com/corporate/div.pdf"},
	}
	got := SelectFilings(anns, ClassBonus, 2)
	require.Len(t, got, 2)
	assert.Equal(t, "https://nsearchives.nseindia.com/corporate/rd.pdf", got[0].File, "record-date intimation first")
	assert.Contains(t, got[1].File, "OutcomeofBM")
	for _, a := range got {
		assert.NotContains(t, a.File, "evil.example.com", "only the exchange archive is accepted")
	}
	assert.Empty(t, SelectFilings(anns, ClassDemerger, 2))
}

func TestDividendAmountsReadsEveryComponent(t *testing.T) {
	assert.Equal(t, []float64{10, 66}, DividendAmounts("Interim Dividend - Rs 10 Per Share And Special Dividend - Rs 66 Per Share"))
	assert.Equal(t, []float64{1}, DividendAmounts("Interim Dividend - Re 1 Per Share"))
	assert.Empty(t, DividendAmounts("Bonus 1:1"))
}

func TestCombinedFactorMultipliesSameDayActionsAndNeverGuesses(t *testing.T) {
	bonus := Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Bonus 4:1"})
	split := Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Face Value Split (Sub-Division) - From Rs 2/- Per Share To Re 1/- Per Share"})
	div := Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Dividend - Rs 5 Per Share"})
	f := combinedFactor([]Classified{bonus, split, div}, bonus.Ex)
	require.NotNil(t, f)
	assert.InDelta(t, 0.1, *f, 1e-12, "BAJFINANCE: 0.2 x 0.5")
	dem := Classify(FeedAction{ExDate: "16-Jun-2025", Subject: "Demerger"})
	assert.Nil(t, combinedFactor([]Classified{bonus, dem}, bonus.Ex), "an unstated demerger ratio makes the day's factor unknown")
}
