package nsebhav

import (
	"archive/zip"
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func zipOf(t *testing.T, name, body string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	require.NoError(t, err)
	_, _ = w.Write([]byte(body))
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// Rows copied from the real NSE files for these dates.
const udiff = `TradDt,BizDt,Sgmt,Src,FinInstrmTp,FinInstrmId,ISIN,TckrSymb,SctySrs,XpryDt,FininstrmActlXpryDt,StrkPric,OptnTp,FinInstrmNm,OpnPric,HghPric,LwPric,ClsPric,LastPric,PrvsClsgPric,UndrlygPric,SttlmPric,OpnIntrst,ChngInOpnIntrst,TtlTradgVol,TtlTrfVal,TtlNbOfTxsExctd,SsnId,NewBrdLotQty,Rmks,Rsvd1,Rsvd2,Rsvd3,Rsvd4
2026-10-30,2026-10-30,CM,NSE,STK,11536,INE467B01029,TCS,EQ,,,,,TATA CONSULTANCY SERV LT,2495.20,2527.70,2494.20,2502.60,2503.00,2478.00,,2502.60,,,3016983,7578000000.00,150000,F1,1,,,,,
`

const legacy = `SYMBOL,SERIES,OPEN,HIGH,LOW,CLOSE,LAST,PREVCLOSE,TOTTRDQTY,TOTTRDVAL,TIMESTAMP,TOTALTRADES,ISIN,
TCS,EQ,3580,3600,3560,3590.5,3591,3575,1500000,5385750000,03-OCT-2023,90000,INE467B01029,
`

func TestParseUDiFF(t *testing.T) {
	rows, total, err := Parse(zipOf(t, "x.csv", udiff), DocUDiFF, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, 1, total)
	r := rows[0]
	assert.Equal(t, "TCS", r.Ticker)
	assert.Equal(t, "EQ", r.Series)
	assert.Equal(t, 2502.60, r.Close)
	assert.Equal(t, int64(3016983), r.Volume)
	require.NotNil(t, r.PrevClose)
	assert.Equal(t, 2478.00, *r.PrevClose)
	assert.True(t, r.Plausible())
}

func TestParseLegacy(t *testing.T) {
	rows, _, err := Parse(zipOf(t, "cm03OCT2023bhav.csv", legacy), DocLegacy, time.Date(2023, 10, 3, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, 3590.5, rows[0].Close)
	assert.Equal(t, "INE467B01029", rows[0].ISIN)
}

func TestParseRejectsFileForAnotherDay(t *testing.T) {
	_, _, err := Parse(zipOf(t, "x.csv", udiff), DocUDiFF, time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC))
	assert.True(t, errors.Is(err, ErrWrongTradeDate))
}

func TestParseRejectsMalformedInput(t *testing.T) {
	d := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	_, _, err := Parse([]byte("not a zip"), DocUDiFF, d)
	assert.Error(t, err)
	_, _, err = Parse(zipOf(t, "x.csv", "TradDt,TckrSymb\n2026-10-30,TCS\n"), DocUDiFF, d)
	assert.ErrorContains(t, err, "missing column")
	bad := `TradDt,ISIN,TckrSymb,SctySrs,OpnPric,HghPric,LwPric,ClsPric,PrvsClsgPric,TtlTradgVol,TtlTrfVal,TtlNbOfTxsExctd
2026-10-30,INE1,TCS,EQ,abc,1,1,1,1,1,1,1
`
	_, _, err = Parse(zipOf(t, "x.csv", bad), DocUDiFF, d)
	assert.ErrorContains(t, err, "bad open")
}

func TestURLsChooseFormatByDate(t *testing.T) {
	after := URLs(time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC))
	require.Len(t, after, 1)
	assert.Equal(t, "https://nsearchives.nseindia.com/content/cm/BhavCopy_NSE_CM_0_0_0_20261030_F_0000.csv.zip", after[0].URL)
	before := URLs(time.Date(2023, 10, 3, 0, 0, 0, 0, time.UTC))
	require.Len(t, before, 2)
	assert.Equal(t, "https://nsearchives.nseindia.com/content/historical/EQUITIES/2023/OCT/cm03OCT2023bhav.csv.zip", before[1].URL)
}

func TestPlausibleRejectsInconsistentBars(t *testing.T) {
	assert.False(t, Row{Open: 10, High: 9, Low: 8, Close: 9}.Plausible(), "open above high")
	assert.False(t, Row{Open: 10, High: 11, Low: 0, Close: 9}.Plausible(), "zero low")
}
