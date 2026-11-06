package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

func TestPlanIndicators_OnlyComputesWhatHistoryAllows(t *testing.T) {
	compute, skipped := planIndicators(0)
	assert.Empty(t, compute)
	assert.Len(t, skipped, len(technicalPlan))

	compute, skipped = planIndicators(19) // today's real depth of verified forward history
	assert.Equal(t, []string{"sma_5", "sma_10", "rsi_14", "atr_14"}, compute)
	for _, s := range skipped {
		assert.Greater(t, s["min_bars"], 19, "a skipped indicator must genuinely need more bars")
	}

	compute, skipped = planIndicators(500)
	assert.Len(t, compute, len(technicalPlan))
	assert.Empty(t, skipped)
	assert.NotNil(t, skipped, "skipped must serialize as [] not null")
}

func TestSimpleReturns(t *testing.T) {
	assert.InDeltaSlice(t, []float64{0.1, -0.1}, simpleReturns([]float64{100, 110, 99}), 1e-12)
	assert.Empty(t, simpleReturns([]float64{100}))
	assert.Empty(t, simpleReturns(nil))
	// a non-positive price never manufactures a return
	assert.InDeltaSlice(t, []float64{0.5}, simpleReturns([]float64{2, 3, 0, 4}), 1e-12)
}

func TestLatestScalarSkipsTrailingNulls(t *testing.T) {
	v := latestScalar([]interface{}{1.0, 2.5, nil, nil})
	require.NotNil(t, v)
	assert.Equal(t, 2.5, *v)
	assert.Nil(t, latestScalar([]interface{}{nil, nil}))
	assert.Nil(t, latestScalar(map[string]interface{}{"macd": []interface{}{1.0}}), "non-scalar series has no single latest")
	assert.Nil(t, latestScalar(nil))
}

// ---- endpoint behaviour against the isolated test database (52 symbols, no bars) ----

func stocksRouter(t *testing.T) http.Handler {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	db := database.GetIsolatedTestDB(t)
	return NewStocksHandler(db, nil).Routes()
}

func get(h http.Handler, path string) (*httptest.ResponseRecorder, envelope) {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func TestStockEndpoints_SearchMatchesTickerAndName(t *testing.T) {
	h := stocksRouter(t)

	rec, env := get(h, "/search?q=reliance")
	require.Equal(t, http.StatusOK, rec.Code)
	var hits []map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &hits))
	require.NotEmpty(t, hits)
	assert.Equal(t, "RELIANCE", hits[0]["symbol"])

	rec, _ = get(h, "/search?q=")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	// LIKE metacharacters are data, not wildcards: '%' must not match everything.
	rec, env = get(h, "/search?q=%25")
	require.Equal(t, http.StatusOK, rec.Code)
	require.NoError(t, json.Unmarshal(env.Data, &hits))
	assert.Empty(t, hits)
}

func TestStockEndpoints_UnknownSymbolIs404(t *testing.T) {
	h := stocksRouter(t)
	for _, p := range []string{"candles", "fundamentals", "technicals", "risk", "analytics"} {
		rec, _ := get(h, "/NOSUCHTICKER/"+p)
		assert.Equal(t, http.StatusNotFound, rec.Code, p)
	}
}

func TestStockEndpoints_CandleParamsAreValidated(t *testing.T) {
	h := stocksRouter(t)
	for _, q := range []string{"limit=0", "limit=1001", "limit=abc", "interval=weekly"} {
		rec, _ := get(h, "/RELIANCE/candles?"+q)
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, q)
	}
	rec, env := get(h, "/RELIANCE/candles?limit=5")
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Candles []any `json:"candles"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &body))
	assert.NotNil(t, body.Candles, "empty history is [] not null")
}

func TestStockEndpoints_NoEligibleHistoryIsReportedNotInvented(t *testing.T) {
	h := stocksRouter(t)

	for _, p := range []string{"technicals", "risk"} {
		rec, env := get(h, "/RELIANCE/"+p)
		require.Equal(t, http.StatusOK, rec.Code, p)
		var body map[string]any
		require.NoError(t, json.Unmarshal(env.Data, &body))
		assert.Equal(t, "DATA_INSUFFICIENT", body["status"], p)
	}

	rec, env := get(h, "/RELIANCE/analytics")
	require.Equal(t, http.StatusOK, rec.Code)
	var an map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &an))
	assert.Equal(t, "NO_ELIGIBLE_HISTORY", an["readiness"])
	assert.Equal(t, false, an["risk_metrics_available"])
}
