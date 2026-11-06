package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

// seedPredictions writes one model run and three resolved forecasts into the isolated test DB.
// Forecasts are append-only by trigger, so cleanup bypasses triggers (test database only).
func seedPredictions(t *testing.T) http.Handler {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	db := database.GetIsolatedTestDB(t)
	var run int64
	require.NoError(t, db.QueryRow(`INSERT INTO prediction_model_runs (model_version, horizon_minutes, verdict, metrics, params, cost_model, is_active)
		VALUES ('TEST_V', 60, 'NO_DEMONSTRATED_EDGE', '{"edge_checks":{"enough_trades":true}}', '{}', '{"round_trip_cost_fraction":0.002}', false)
		RETURNING id`).Scan(&run))
	t.Cleanup(func() {
		_, _ = db.Exec(`BEGIN; SET LOCAL session_replication_role = replica;
			DELETE FROM predictions WHERE model_run_id IN (SELECT id FROM prediction_model_runs WHERE model_version = 'TEST_V');
			DELETE FROM prediction_model_runs WHERE model_version = 'TEST_V'; COMMIT;`)
	})
	// realized returns: +1% (BUY, wins net), -0.5% (BUY, loses), +0.1% (NO_TRADE)
	for i, row := range []struct {
		sym, action, reasons string
		ret                  float64
	}{
		{"TCS", "BUY_CANDIDATE", `[{"code":"MODEL_VIEW","detail":"BUY"}]`, 0.01},
		{"INFY", "BUY_CANDIDATE", `[{"code":"MODEL_VIEW","detail":"BUY"}]`, -0.005},
		{"WIPRO", "NO_TRADE", `[{"code":"MODEL_VIEW","detail":"NO_EDGE_AFTER_COSTS"}]`, 0.001},
	} {
		_, err := db.Exec(`INSERT INTO predictions (model_run_id, symbol_id, horizon_minutes, as_of, target_time, reference_price,
			p_down, p_neutral, p_up, expected_return, return_q10, return_q50, return_q90, action, action_reasons,
			entry_price, target_price, stop_price, shariah_status, realized_price, realized_return, realized_class, resolved_at)
			SELECT $1, id, 60, now() - interval '2 hours', now() - interval '1 hour', 100, 0.2, 0.3, 0.5, 0.004, -0.01, 0.001, 0.012,
			       $2, $3::jsonb, CASE WHEN $2 = 'BUY_CANDIDATE' THEN 100 END, CASE WHEN $2 = 'BUY_CANDIDATE' THEN 101 END,
			       CASE WHEN $2 = 'BUY_CANDIDATE' THEN 99 END, 'DATA_INSUFFICIENT', 100 * (1 + $4::numeric), $4::numeric,
			       CASE WHEN $4::numeric > 0.002 THEN 'UP' WHEN $4::numeric < -0.002 THEN 'DOWN' ELSE 'NEUTRAL' END, now()
			FROM symbols WHERE symbol = $5 AND exchange = 'NSE'`, run, row.action, row.reasons, row.ret, row.sym)
		require.NoError(t, err, i)
	}
	return NewPredictionHandler(db).Routes()
}

func TestPredictions_TrackingIsNetOfCostAndSeparatesRecommendationsFromModelView(t *testing.T) {
	h := seedPredictions(t)
	rec, env := get(h, "/tracking")
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Horizons []struct {
			Horizon  int `json:"horizon_minutes"`
			Resolved int `json:"resolved"`
			Buys     struct {
				N   int      `json:"n"`
				Net *float64 `json:"mean_net_return"`
				Hit *float64 `json:"hit_rate_net"`
			} `json:"recommended_buys"`
			AlwaysLong *float64 `json:"baseline_always_long_mean_net_return"`
		} `json:"horizons"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &body))
	require.Len(t, body.Horizons, 1)
	hz := body.Horizons[0]
	assert.Equal(t, 3, hz.Resolved)
	assert.Equal(t, 2, hz.Buys.N)
	// ((0.01 - 0.002) + (-0.005 - 0.002)) / 2 = 0.0005 ; one of two wins after cost
	assert.InDelta(t, 0.0005, *hz.Buys.Net, 1e-9)
	assert.InDelta(t, 0.5, *hz.Buys.Hit, 1e-9)
	assert.InDelta(t, (0.008-0.007-0.001)/3, *hz.AlwaysLong, 1e-9)
}

func TestPredictions_LatestCarriesTradePlanAndVerdict(t *testing.T) {
	h := seedPredictions(t)
	rec, env := get(h, "/latest?horizon=60")
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Predictions []map[string]any `json:"predictions"`
		Disclaimer  string           `json:"disclaimer"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &body))
	require.Len(t, body.Predictions, 3)
	first := body.Predictions[0]
	assert.Equal(t, "BUY_CANDIDATE", first["action"], "buy candidates are listed first")
	assert.Equal(t, "NO_DEMONSTRATED_EDGE", first["model_verdict"])
	assert.NotNil(t, first["target_price"])
	assert.NotEmpty(t, body.Disclaimer)

	rec, _ = get(h, "/latest?horizon=abc")
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestPredictions_BySymbolAndUnknownSymbol(t *testing.T) {
	h := seedPredictions(t)
	rec, env := get(h, "/symbol/tcs")
	require.Equal(t, http.StatusOK, rec.Code)
	var list []map[string]any
	require.NoError(t, json.Unmarshal(env.Data, &list))
	assert.Len(t, list, 1)
	rec, _ = get(h, "/symbol/NOPE")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
