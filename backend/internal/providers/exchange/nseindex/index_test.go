package nseindex

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sample = `Index Name,Index Date,Open Index Value,High Index Value,Low Index Value,Closing Index Value,Points Change,Change(%),Volume,Turnover (Rs. Cr.),P/E,P/B,Div Yield
Nifty 50,03-11-2026,23582.25,23810.85,23482.8,23810.85,370.05,1.58,408375815,40323.17,19.71,2.92,1.46
Nifty Midcap 50,03-11-2026,-,-,-,15000.5,-,-,-,-,-,-,-
Broken,03-11-2026,100,90,95,95,0,0,0,0,0,0,0
Old Day,02-11-2026,100,110,90,105,0,0,0,0,0,0,0
`

func TestParse_KeepsOnlyTheRequestedDayAndNeverInventsMissingFields(t *testing.T) {
	want := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	rows, err := Parse(strings.NewReader(sample), want)
	require.NoError(t, err)
	require.Len(t, rows, 2, "an impossible row and another day's row are dropped")
	assert.Equal(t, "Nifty 50", rows[0].Name)
	assert.InDelta(t, 23810.85, rows[0].Close, 1e-9)
	require.NotNil(t, rows[0].PE)
	assert.InDelta(t, 19.71, *rows[0].PE, 1e-9)
	assert.Equal(t, int64(408375815), *rows[0].Volume)
	assert.Nil(t, rows[1].Open, "a dash is 'not published', not zero")
	assert.Nil(t, rows[1].PE)
}

func TestParse_RejectsAFileWithoutTheExpectedColumns(t *testing.T) {
	_, err := Parse(strings.NewReader("a,b\n1,2\n"), time.Now())
	assert.Error(t, err)
}

func TestURL(t *testing.T) {
	assert.Equal(t, "https://nsearchives.nseindia.com/content/indices/ind_close_all_03112026.csv", URL(time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)))
}
