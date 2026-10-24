package truedata

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrueData_UnauthenticatedCallsFailClosed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Username = ""
	cfg.Password = ""
	cfg.APIKey = ""
	cfg.FixtureMode = false

	p := NewProvider(cfg)
	assert.False(t, p.HasActiveCredentials())
	assert.Equal(t, "truedata", p.Name())

	caps := p.Capabilities()
	assert.True(t, caps.DailyOHLCV)
	assert.True(t, caps.IntradayOHLCV)

	ctx := context.Background()
	_, err := p.GetDailyCandles(ctx, "TCS", "NSE", time.Now().AddDate(0, -1, 0), time.Now())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionUnauthorized)
	assert.Contains(t, err.Error(), "DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION")
}

func TestTrueData_FixtureModePreservesQualitySuspectProvenance(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FixtureMode = true
	fixtureDir := "../../../../../data/poc"
	if _, err := os.Stat(fixtureDir); err != nil {
		fixtureDir = "data/poc"
	}
	cfg.FixtureDir = fixtureDir

	p := NewProvider(cfg)
	ctx := context.Background()

	dp, err := p.GetDailyCandles(ctx, "TCS", "NSE", time.Now().AddDate(-1, 0, 0), time.Now())
	require.NoError(t, err)
	require.NotNil(t, dp)

	// Fixture MUST be labeled as fixture and suspect quality
	assert.Equal(t, "FIXTURE:truedata_schema_simulation", dp.Source)
	assert.Equal(t, models.DataQualitySuspect, dp.QualityStatus)
	assert.NotEmpty(t, dp.Data)

	// Check nominal prices (adjusted_close matches close)
	firstBar := dp.Data[0]
	require.NotNil(t, firstBar.AdjustedClose)
	assert.Equal(t, firstBar.Close, *firstBar.AdjustedClose)
	assert.Equal(t, "FIXTURE:truedata_schema_simulation", firstBar.Source)
}

func TestTrueData_LiveMockResponseAndRateLimiting(t *testing.T) {
	// Mock TrueData server
	requestCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		// First attempt: return 429 Too Many Requests to test retry/backoff
		if requestCount == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"status":"error","message":"rate limit exceeded"}`))
			return
		}

		// Second attempt: return valid TrueData response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"status": "success",
			"symbol": "TCS",
			"duration": "5D",
			"bar_size": "eod",
			"data": [
				["2026-10-20", 2500.0, 2520.0, 2490.0, 2510.0, 1500000, 0],
				["2026-10-21", 2510.0, 2530.0, 2505.0, 2525.0, 1600000, 0]
			]
		}`))
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:      ts.URL,
		APIKey:       "mock-live-key",
		Timeout:      2 * time.Second,
		MaxRetries:   2,
		BackoffBase:  10 * time.Millisecond,
		FixtureMode:  false,
	}

	p := NewProvider(cfg)
	assert.True(t, p.HasActiveCredentials())

	ctx := context.Background()
	dp, err := p.GetDailyCandles(ctx, "TCS", "NSE", time.Now().AddDate(0, 0, -5), time.Now())
	require.NoError(t, err)
	require.NotNil(t, dp)

	assert.Equal(t, "truedata_live_authenticated", dp.Source)
	assert.Equal(t, models.DataQualityValid, dp.QualityStatus)
	assert.Len(t, dp.Data, 2)
	assert.Equal(t, 2, requestCount) // Proves 429 triggered retry
}
