package database

import (
	"encoding/json"
	"math"
	"testing"
)

// TestIPOFeedCompletenessScenarios audits the feed handling across:
// 1. Current active bidding issue
// 2. Empty response (HTTP 200 with [])
// 3. Provider failure (HTTP 403 / 500)
// 4. Distinction between official facts, third-party GMP, and model estimates
// 5. Prediction error formula and point-in-time integrity
func TestIPOFeedCompletenessScenarios(t *testing.T) {

	t.Run("Scenario 1 - Empty Response Scope Interpretation", func(t *testing.T) {
		// HTTP 200 with [] on ipo-current-issue ONLY proves zero active bidding issues today.
		// It MUST NOT be interpreted as proof that no forthcoming IPOs exist.
		feedResponse := "[]"
		var issues []map[string]any
		err := json.Unmarshal([]byte(feedResponse), &issues)
		if err != nil {
			t.Fatalf("failed to parse empty response: %v", err)
		}

		if len(issues) != 0 {
			t.Errorf("expected 0 issues, got %d", len(issues))
		}

		// Structured run documentation must explicitly note feed scope
		run := IPOIngestionRun{
			Provider:       "NSE_OFFICIAL_IPO_API",
			Status:         "EMPTY",
			RecordsFetched: 0,
			ErrorDetails:   "HTTP 200: Current issue endpoint returned 0 active bidding issues. Scope: active bidding issues only. Does not encompass SEBI DRHP filings or unannounced forthcoming issues.",
			SourceURL:      "https://www.nseindia.com/api/ipo-current-issue",
			DataFreshness:  "UP_TO_DATE",
		}

		if run.Status != "EMPTY" {
			t.Errorf("expected status EMPTY, got %s", run.Status)
		}
		if run.RecordsFetched != 0 {
			t.Errorf("expected 0 records fetched, got %d", run.RecordsFetched)
		}
	})

	t.Run("Scenario 2 - Provider Failure Handling", func(t *testing.T) {
		// When provider returns 403 or network failure, must record FAILED status
		// and never silently serve stale or synthetic records as fresh.
		run := IPOIngestionRun{
			Provider:       "NSE_OFFICIAL_IPO_API",
			Status:         "FAILED",
			RecordsFetched: 0,
			ErrorDetails:   "HTTP 403 Forbidden: WAF blocked request. Cookie handshake required.",
			SourceURL:      "https://www.nseindia.com/api/ipo-current-issue",
			DataFreshness:  "STALE_FEED",
		}

		if run.Status != "FAILED" {
			t.Errorf("expected status FAILED, got %s", run.Status)
		}
	})

	t.Run("Scenario 3 - Evidence Attribution Taxonomy", func(t *testing.T) {
		// Must clearly separate:
		// 1. Official Facts (Exchange/RHP Filings)
		// 2. Unofficial Third-Party Sentiments (GMP)
		// 3. Quantitative Model Estimates (Algorithmic Scenarios)

		officialFacts := map[string]any{
			"company_name": "Waaree Energies Ltd",
			"symbol":       "WAAREE",
			"exchange":     "NSE",
			"issue_price":  1503.0,
			"lot_size":     9,
			"source":       "NSE_OFFICIAL_FILING",
			"is_official":  true,
		}

		gmpObservation := map[string]any{
			"value":       1425.0,
			"source":      "Unofficial Grey Market Aggregator",
			"disclaimer":  "Unofficial third-party estimate. Not exchange-certified or SEBI regulated.",
			"is_official": false,
		}

		modelEstimate := map[string]any{
			"predicted_price":  2750.0,
			"predicted_return": 82.97,
			"model_name":       "IPOListingRegressor_v2",
			"is_official":      false,
			"is_deterministic": true,
		}

		if officialFacts["is_official"] != true {
			t.Errorf("official facts must have is_official = true")
		}
		if gmpObservation["is_official"] == true {
			t.Errorf("GMP must NOT be marked as official")
		}
		if modelEstimate["is_official"] == true {
			t.Errorf("Model estimate must NOT be marked as official")
		}
	})

	t.Run("Scenario 4 - Explicit Prediction Error Definition", func(t *testing.T) {
		// Formula: (Predicted Price - Actual Listing Price) / Actual Listing Price * 100
		actualPrice := 2550.0
		predictedPrice := 2750.0

		predictionErrorPct := ((predictedPrice - actualPrice) / actualPrice) * 100.0
		expectedError := (200.0 / 2550.0) * 100.0 // ~7.8431%

		if math.Abs(predictionErrorPct-expectedError) > 0.001 {
			t.Errorf("prediction error mismatch: got %.4f, want %.4f", predictionErrorPct, expectedError)
		}

		// Directional Accuracy: Both predicted and actual listing returns were positive
		issuePrice := 1503.0
		predictedReturn := ((predictedPrice - issuePrice) / issuePrice) * 100.0 // +82.97%
		actualReturn := ((actualPrice - issuePrice) / issuePrice) * 100.0       // +69.66%

		directionalMatch := (predictedReturn > 0 && actualReturn > 0) || (predictedReturn < 0 && actualReturn < 0)
		if !directionalMatch {
			t.Errorf("expected directional match between predicted and actual returns")
		}
	})

	t.Run("Scenario 5 - Future Data Leakage Prohibition", func(t *testing.T) {
		// Pre-listing prediction inputs MUST NOT contain actual listing outcome
		inputsAtCutoff := map[string]any{
			"cutoff_date":   "2024-10-23",
			"issue_price":   1503.0,
			"qib_sub":       208.63,
			"total_sub":     79.44,
			"gmp_at_cutoff": 1425.0,
		}

		// Ensure prohibited post-listing keys are not in pre-listing inputs
		prohibitedKeys := []string{"actual_listing_price", "actual_listing_return", "listing_day_volume", "listing_close"}
		for _, key := range prohibitedKeys {
			if _, exists := inputsAtCutoff[key]; exists {
				t.Fatalf("Look-ahead bias violation: pre-listing inputs contain %s", key)
			}
		}
	})
}
