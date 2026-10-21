package nse_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/providers/corporateactions/nse"
)

func TestParseNSEDate(t *testing.T) {
	d, err := nse.ParseNSEDate("14-Oct-2026")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, 2026, d.Year())
	assert.Equal(t, time.Month(10), d.Month())
	assert.Equal(t, 14, d.Day())

	// Empty and dash strings should return nil without error
	d2, err2 := nse.ParseNSEDate("-")
	require.NoError(t, err2)
	assert.Nil(t, d2)

	d3, err3 := nse.ParseNSEDate("")
	require.NoError(t, err3)
	assert.Nil(t, d3)

	// Malformed date
	_, err4 := nse.ParseNSEDate("invalid-date")
	assert.Error(t, err4)
}

func TestParseRawCorporateAction_SingleDividend(t *testing.T) {
	raw := nse.RawNSECorporateAction{
		Symbol:  "TCS",
		Comp:    "Tata Consultancy Services Limited",
		Series:  "EQ",
		FaceVal: "1",
		ISIN:    "INE467B01029",
		Subject: "Interim Dividend - Rs 12 Per Share",
		ExDate:  "14-Oct-2026",
		RecDate: "14-Oct-2026",
	}

	artID := 10
	events, err := nse.ParseRawCorporateAction(raw, 9, &artID)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "TCS", ev.Symbol)
	assert.Equal(t, 9, ev.SymbolID)
	assert.Equal(t, "CASH_DIVIDEND", ev.ActionType)
	assert.Equal(t, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1", ev.EventID)
	require.NotNil(t, ev.DividendAmount)
	assert.Equal(t, 12.0, *ev.DividendAmount)
	assert.Equal(t, "VERIFIED_EXCHANGE_PRIMARY", ev.VerificationStatus)
	assert.Equal(t, "PARSED_EVENT", ev.CoverageStatus)
	assert.Equal(t, "INR", ev.Currency)
}

func TestParseRawCorporateAction_CompositeDividend(t *testing.T) {
	// A single subject declaring both an interim dividend and a special dividend on the same ex-date
	raw := nse.RawNSECorporateAction{
		Symbol:  "TCS",
		Comp:    "Tata Consultancy Services Limited",
		Series:  "EQ",
		FaceVal: "1",
		ISIN:    "INE467B01029",
		Subject: "Interim Dividend Rs 11 Per Share/ Special Dividend Rs 46 Per Share",
		ExDate:  "16-Jan-2026",
		RecDate: "17-Jan-2026",
	}

	events, err := nse.ParseRawCorporateAction(raw, 9, nil)
	require.NoError(t, err)
	require.Len(t, events, 2, "Composite dividend must produce 2 distinct events")

	// Event 1: Interim Rs 11
	ev1 := events[0]
	assert.Equal(t, "NSE:TCS:2026-01-16:CASH_DIVIDEND:1", ev1.EventID)
	require.NotNil(t, ev1.DividendAmount)
	assert.Equal(t, 11.0, *ev1.DividendAmount)

	// Event 2: Special Rs 46
	ev2 := events[1]
	assert.Equal(t, "NSE:TCS:2026-01-16:CASH_DIVIDEND:2", ev2.EventID)
	require.NotNil(t, ev2.DividendAmount)
	assert.Equal(t, 46.0, *ev2.DividendAmount)

	// Event IDs must never collide
	assert.NotEqual(t, ev1.EventID, ev2.EventID)
}

func TestParseRawCorporateAction_StockSplit(t *testing.T) {
	raw := nse.RawNSECorporateAction{
		Symbol:  "TESTSPLIT",
		Subject: "Sub-Division/Stock Split From Rs 10/- To Re 1/-",
		ExDate:  "20-Nov-2025",
	}

	events, err := nse.ParseRawCorporateAction(raw, 100, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "STOCK_SPLIT", ev.ActionType)
	assert.Equal(t, "NSE:TESTSPLIT:2025-11-20:STOCK_SPLIT:1", ev.EventID)
	require.NotNil(t, ev.SplitRatioOld)
	require.NotNil(t, ev.SplitRatioNew)
	assert.Equal(t, 10.0, *ev.SplitRatioOld)
	assert.Equal(t, 1.0, *ev.SplitRatioNew)
	assert.Equal(t, "VERIFIED_EXCHANGE_PRIMARY", ev.VerificationStatus)
}

func TestParseRawCorporateAction_BonusIssue(t *testing.T) {
	raw := nse.RawNSECorporateAction{
		Symbol:  "TESTBONUS",
		Subject: "Bonus Issue 1:2",
		ExDate:  "15-Aug-2025",
	}

	events, err := nse.ParseRawCorporateAction(raw, 101, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "BONUS_ISSUE", ev.ActionType)
	assert.Equal(t, "NSE:TESTBONUS:2025-08-15:BONUS_ISSUE:1", ev.EventID)
	require.NotNil(t, ev.BonusRatioBonus)
	require.NotNil(t, ev.BonusRatioHeld)
	assert.Equal(t, 1.0, *ev.BonusRatioBonus)
	assert.Equal(t, 2.0, *ev.BonusRatioHeld)
}

func TestParseRawCorporateAction_RightsIssue(t *testing.T) {
	raw := nse.RawNSECorporateAction{
		Symbol:  "TESTRIGHTS",
		Subject: "Rights 1:4 @ Rs 1257",
		ExDate:  "10-Sep-2025",
	}

	events, err := nse.ParseRawCorporateAction(raw, 102, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "RIGHTS_ISSUE", ev.ActionType)
	assert.Equal(t, "NSE:TESTRIGHTS:2025-09-10:RIGHTS_ISSUE:1", ev.EventID)
	require.NotNil(t, ev.RightsRatioNew)
	require.NotNil(t, ev.RightsRatioOld)
	require.NotNil(t, ev.RightsSubscriptionPrice)
	assert.Equal(t, 1.0, *ev.RightsRatioNew)
	assert.Equal(t, 4.0, *ev.RightsRatioOld)
	assert.Equal(t, 1257.0, *ev.RightsSubscriptionPrice)
}

func TestParseRawCorporateAction_MalformedSubject(t *testing.T) {
	raw := nse.RawNSECorporateAction{
		Symbol:  "TESTOTHER",
		Subject: "Annual General Meeting / Miscellaneous Notice",
		ExDate:  "01-Jul-2025",
	}

	events, err := nse.ParseRawCorporateAction(raw, 103, nil)
	require.NoError(t, err)
	require.Len(t, events, 1)

	ev := events[0]
	assert.Equal(t, "OTHER", ev.ActionType)
	assert.Equal(t, "AMBIGUOUS", ev.VerificationStatus)
	assert.Equal(t, "SECONDARY_EVIDENCE_REQUIRED", ev.CoverageStatus)
}
