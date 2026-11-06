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
	"github.com/halal-equity/backend/internal/services/quant"
)

func fakeQuant(t *testing.T, matches bool) *quant.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runtime_hash": "abc", "pinned_hash": "abc", "matches_pinned": matches, "problems": []string{},
		})
	}))
	t.Cleanup(srv.Close)
	return quant.NewClient(srv.URL)
}

func gateFor(t *testing.T, q *quant.Client) researchGate {
	t.Helper()
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	h := NewBacktestingHandler(database.GetIsolatedTestDB(t), q)
	rec := httptest.NewRecorder()
	h.GetGate(rec, httptest.NewRequest(http.MethodGet, "/gate", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var env struct {
		Data researchGate `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Data
}

func TestGate_ClosedWhenStrategyAttestationUnavailable(t *testing.T) {
	g := gateFor(t, nil)
	assert.Equal(t, "RESEARCH_BLOCKED", g.Status)
	assert.Nil(t, g.StrategyHashValid, "unknown attestation must not be reported as valid")
	assert.Contains(t, g.Reason, "attestation")
}

func TestGate_ClosedWhenRunningStrategyDriftsFromPinned(t *testing.T) {
	g := gateFor(t, fakeQuant(t, false))
	assert.Equal(t, "RESEARCH_BLOCKED", g.Status)
	require.NotNil(t, g.StrategyHashValid)
	assert.False(t, *g.StrategyHashValid)
	assert.Contains(t, g.Reason, "pinned hash")
}

func TestGate_CountsAreMeasuredNotConstants(t *testing.T) {
	g := gateFor(t, fakeQuant(t, true))
	// The isolated database holds no bars: a hardcoded "27 equities / N bars" string would be wrong here.
	assert.EqualValues(t, 0, g.QuarantinedBars)
	assert.Equal(t, 0, g.EquitiesWithQuarantine)
	assert.Equal(t, 52, g.TotalInstruments)
	assert.NotContains(t, g.Reason, "27 core equities")
}

func TestRunBacktest_RefusesWhileGateClosed(t *testing.T) {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	h := NewBacktestingHandler(database.GetIsolatedTestDB(t), fakeQuant(t, false))
	rec := httptest.NewRecorder()
	h.RunBacktest(rec, httptest.NewRequest(http.MethodPost, "/run", nil))
	var env struct {
		Data map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	assert.Equal(t, "RESEARCH_BLOCKED", env.Data["status"])
}

func TestUpdateWeights_FrozenStrategyRefusesEdits(t *testing.T) {
	h := &SignalsHandler{}
	rec := httptest.NewRecorder()
	h.UpdateWeights(rec, httptest.NewRequest(http.MethodPut, "/weights", nil))
	assert.Equal(t, http.StatusConflict, rec.Code, "must not claim success for a change that was not made")
}
