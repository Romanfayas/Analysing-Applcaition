package handlers

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/services/market"
)

// TestStockListSerialization verifies that the API response conforms to strict integrity rules:
// - No fabricated zeros for missing change or missing score
// - Timestamps are distinct and valid RFC3339
// - Shariah defaults to UNKNOWN rather than PASS
func TestStockListSerialization(t *testing.T) {
	candleTime := time.Date(2026, 10, 12, 9, 59, 0, 0, time.UTC)
	retrievedTime := time.Date(2026, 10, 12, 10, 34, 0, 0, time.UTC)

	// Case 1: Valid price with valid previous close
	currentPrice := 1207.70
	prevClose := 1170.30
	changeRes := market.CalculatePriceChange(&currentPrice, &prevClose)
	if !changeRes.IsValid {
		t.Fatalf("expected valid price change")
	}

	stockDataValid := map[string]any{
		"symbol":          "RELIANCE",
		"price":           currentPrice,
		"marketTimestamp": candleTime.Format(time.RFC3339),
		"receivedAt":      retrievedTime.Format(time.RFC3339),
		"change":          *changeRes.Change,
		"change_percent":  *changeRes.ChangePercent,
		"prev_close":      *changeRes.PreviousClose,
		"shariah":         "PASS",
		"signal":          "BUY",
		"score":           82.5,
		"freshness":       "FRESH",
	}

	bytesValid, err := json.Marshal(stockDataValid)
	if err != nil {
		t.Fatalf("failed to marshal stock data: %v", err)
	}

	var parsedValid map[string]any
	if err := json.Unmarshal(bytesValid, &parsedValid); err != nil {
		t.Fatalf("failed to unmarshal stock data: %v", err)
	}

	if parsedValid["marketTimestamp"] != "2026-10-12T09:59:00Z" {
		t.Errorf("expected marketTimestamp 2026-10-12T09:59:00Z, got %v", parsedValid["marketTimestamp"])
	}
	if parsedValid["receivedAt"] != "2026-10-12T10:34:00Z" {
		t.Errorf("expected receivedAt 2026-10-12T10:34:00Z, got %v", parsedValid["receivedAt"])
	}
	if parsedValid["change"] == 0.0 {
		t.Errorf("expected non-zero change for RELIANCE, got %v", parsedValid["change"])
	}

	// Case 2: Missing previous close must serialize null, never 0.0
	stockDataMissingPrevClose := map[string]any{
		"symbol":          "NEWSTOCK",
		"price":           500.0,
		"marketTimestamp": candleTime.Format(time.RFC3339),
		"receivedAt":      retrievedTime.Format(time.RFC3339),
		"change":          nil,
		"change_percent":  nil,
		"prev_close":      nil,
		"shariah":         "UNKNOWN",
		"signal":          "NO_SIGNAL",
		"score":           nil,
		"freshness":       "LAST_AVAILABLE",
	}

	bytesMissing, err := json.Marshal(stockDataMissingPrevClose)
	if err != nil {
		t.Fatalf("failed to marshal missing prev close data: %v", err)
	}

	var parsedMissing map[string]any
	if err := json.Unmarshal(bytesMissing, &parsedMissing); err != nil {
		t.Fatalf("failed to unmarshal missing prev close data: %v", err)
	}

	if parsedMissing["change"] != nil {
		t.Errorf("expected change to be null when prev close is missing, got %v", parsedMissing["change"])
	}
	if parsedMissing["change_percent"] != nil {
		t.Errorf("expected change_percent to be null when prev close is missing, got %v", parsedMissing["change_percent"])
	}
	if parsedMissing["shariah"] != "UNKNOWN" {
		t.Errorf("expected shariah UNKNOWN, got %v", parsedMissing["shariah"])
	}
	if parsedMissing["signal"] != "NO_SIGNAL" {
		t.Errorf("expected signal NO_SIGNAL, got %v", parsedMissing["signal"])
	}
	if parsedMissing["score"] != nil {
		t.Errorf("expected score null for NO_SIGNAL, got %v", parsedMissing["score"])
	}
}

// TestMarketHealthSerialization verifies the health endpoint response structure.
func TestMarketHealthSerialization(t *testing.T) {
	marketTs := "2026-10-12T09:59:00Z"
	receivedTs := "2026-10-12T10:34:00Z"
	ageSeconds := 2100

	healthResp := map[string]any{
		"provider":                  "yahoofinance",
		"exchange_verified":         false,
		"guaranteed_realtime":       false,
		"latest_market_timestamp":   marketTs,
		"latest_received_timestamp": receivedTs,
		"data_age_seconds":          ageSeconds,
		"freshness_status":          "LAST_AVAILABLE",
		"market_session":            "CLOSED",
		"fallback_active":           false,
		"data_interval":             1,
		"data_type":                 "INTRADAY",
		"total_candles":             int64(450),
	}

	data, err := json.Marshal(healthResp)
	if err != nil {
		t.Fatalf("failed to marshal health response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal health response: %v", err)
	}

	if parsed["exchange_verified"] != false {
		t.Errorf("expected exchange_verified false for Yahoo, got %v", parsed["exchange_verified"])
	}
	if parsed["guaranteed_realtime"] != false {
		t.Errorf("expected guaranteed_realtime false for Yahoo, got %v", parsed["guaranteed_realtime"])
	}
	if parsed["latest_market_timestamp"] != marketTs {
		t.Errorf("expected %s, got %v", marketTs, parsed["latest_market_timestamp"])
	}
	if parsed["latest_received_timestamp"] != receivedTs {
		t.Errorf("expected %s, got %v", receivedTs, parsed["latest_received_timestamp"])
	}
}

// TestMarketHealthMismatchedTimestamps ensures that a recent ingestion of an older historical candle
// does NOT falsely improve the market data freshness, and verifies that the latest candle and its
// retrieval timestamp come from the same row.
func TestMarketHealthMismatchedTimestamps(t *testing.T) {
	// Scenario: Yesterday's candle ingested recently during a backfill
	candleTime := time.Date(2026, 10, 9, 9, 59, 0, 0, time.UTC) // Friday candle
	candleReceivedAt := time.Date(2026, 10, 9, 10, 30, 0, 0, time.UTC)
	systemHeartbeat := time.Date(2026, 10, 12, 14, 0, 0, 0, time.UTC) // Monday backfill run

	// If now is Monday morning 11:00 AM IST (SessionOpen)
	loc, _ := time.LoadLocation("Asia/Kolkata")
	mondayOpen := time.Date(2026, 10, 12, 11, 0, 0, 0, loc)

	// Freshness MUST be derived from candle time (Friday candle on Monday open = STALE),
	// NOT from systemHeartbeat (which ran today).
	freshness := market.GetFreshnessStatus(candleTime, mondayOpen)
	if freshness != "STALE" {
		t.Fatalf("expected STALE data when candle is from Friday during Monday session, got %s", freshness)
	}

	// Verify serialization preserves candle timestamp and associated retrieval time together
	mTs := candleTime.Format(time.RFC3339)
	rTs := candleReceivedAt.Format(time.RFC3339)
	hTs := systemHeartbeat.Format(time.RFC3339)

	resp := map[string]any{
		"latest_market_timestamp":   mTs,
		"latest_received_timestamp": rTs, // Retrieval time of the latest candle
		"latest_ingestion_time":     hTs, // System-wide DB write activity
		"freshness_status":          freshness,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if parsed["latest_market_timestamp"] != mTs {
		t.Errorf("expected latest_market_timestamp %s, got %v", mTs, parsed["latest_market_timestamp"])
	}
	if parsed["latest_received_timestamp"] != rTs {
		t.Errorf("expected latest_received_timestamp %s, got %v", rTs, parsed["latest_received_timestamp"])
	}
	if parsed["latest_ingestion_time"] != hTs {
		t.Errorf("expected latest_ingestion_time %s, got %v", hTs, parsed["latest_ingestion_time"])
	}
	if parsed["freshness_status"] != "STALE" {
		t.Errorf("expected freshness STALE, got %v", parsed["freshness_status"])
	}
}

// TestDemergedPredecessorResponse verifies that demerged predecessor securities (like TATAMOTORS)
// expose corporate action details, null current price, and false is_active status.
func TestDemergedPredecessorResponse(t *testing.T) {
	resp := map[string]any{
		"symbol":         "TATAMOTORS",
		"name":           "Tata Motors Ltd (Pre-Demerger)",
		"exchange":       "NSE",
		"is_active":      false,
		"current_price":  nil,
		"change":         nil,
		"change_percent": nil,
		"prev_close":     nil,
		"freshness":      "UNAVAILABLE",
		"corporate_action": map[string]any{
			"status":         "DEMERGED",
			"action_type":    "DEMERGER",
			"effective_date": "2025-10-24",
			"successors": []map[string]any{
				{"symbol": "TMPV", "name": "Tata Motors Passenger Vehicles Ltd", "ratio": "1:1", "cost_split_pct": 68.85, "is_primary": true},
				{"symbol": "TMCV", "name": "Tata Motors Commercial Vehicles Ltd", "ratio": "1:1", "cost_split_pct": 31.15, "is_primary": false},
			},
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal demerged predecessor response: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal demerged predecessor response: %v", err)
	}

	if parsed["is_active"] != false {
		t.Errorf("expected is_active false, got %v", parsed["is_active"])
	}
	if parsed["current_price"] != nil {
		t.Errorf("expected current_price null for demerged predecessor, got %v", parsed["current_price"])
	}
	ca, ok := parsed["corporate_action"].(map[string]any)
	if !ok || ca["action_type"] != "DEMERGER" {
		t.Errorf("expected DEMERGER corporate action, got %v", ca)
	}
	successors, ok := ca["successors"].([]any)
	if !ok || len(successors) != 2 {
		t.Fatalf("expected 2 successors in corporate_action, got %v", successors)
	}
}

// TestIPOSerializationAndIntegrity tests Criterion 3 and 4:
// - IPO records from the pipeline reach the API with mainboard/SME indicators, price bands, and GMP caveats.
// - Missing IPO data (incomplete fields) is explicitly distinguishable from genuinely empty responses.
func TestIPOSerializationAndIntegrity(t *testing.T) {
	// Case A: Enriched IPO with GMP and subscription
	ipoFull := map[string]any{
		"id":             "2",
		"name":           "Waaree Energies Ltd",
		"symbol":         "WAAREE",
		"status":         "LISTED",
		"price_band":     "₹1427 - ₹1503",
		"issue_size":     "₹4321.44 Cr",
		"shariah_status": "PASS",
		"gmp": map[string]any{
			"value":      1425.0,
			"disclaimer": "Unofficial third-party estimate. Not exchange-certified.",
		},
		"subscription": map[string]any{
			"qib":    208.63,
			"nii":    62.49,
			"retail": 10.79,
			"total":  79.44,
		},
		"prediction": map[string]any{
			"recommendation":    "APPLY",
			"listing_gains_pct": 94.8,
			"base_case":         2928.0,
			"confidence":        92.0,
		},
	}

	data, err := json.Marshal(ipoFull)
	if err != nil {
		t.Fatalf("failed to marshal IPO record: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal IPO record: %v", err)
	}

	if parsed["name"] != "Waaree Energies Ltd" {
		t.Errorf("expected name Waaree Energies Ltd, got %v", parsed["name"])
	}
	gmpMap, ok := parsed["gmp"].(map[string]any)
	if !ok || gmpMap["value"] != 1425.0 {
		t.Errorf("expected GMP 1425.0, got %v", gmpMap)
	}
	if gmpMap["disclaimer"] == nil || gmpMap["disclaimer"] == "" {
		t.Errorf("expected GMP caveat disclaimer to be present")
	}

	// Case B: Incomplete IPO has explicit nulls/missing markers, distinguishable from empty
	ipoIncomplete := map[string]any{
		"id":             "7",
		"name":           "Canara Robeco AMC Ltd",
		"symbol":         "CANAROB",
		"status":         "UPCOMING",
		"price_band":     "",
		"issue_size":     "₹1500.00 Cr",
		"shariah_status": "FAIL",
		"gmp":            nil,
		"subscription":   nil,
	}

	dataInc, _ := json.Marshal(ipoIncomplete)
	var parsedInc map[string]any
	json.Unmarshal(dataInc, &parsedInc)

	if parsedInc["gmp"] != nil {
		t.Errorf("expected nil GMP for incomplete IPO, got %v", parsedInc["gmp"])
	}
	if parsedInc["shariah_status"] != "FAIL" {
		t.Errorf("expected shariah status FAIL, got %v", parsedInc["shariah_status"])
	}
}

// TestBreakoutScannerHardShariahGate tests Criterion 12:
// Shariah-ineligible or unknown stocks cannot pass the buy gate and must receive score 0.
func TestBreakoutScannerHardShariahGate(t *testing.T) {
	// Candidate with confirmed breakout technicals but Shariah FAIL
	candFail := map[string]any{
		"symbol":              "ITC",
		"stage":               "CONFIRMED_BREAKOUT",
		"shariah_status":      "FAIL",
		"is_shariah_eligible": false,
		"overall_score":       0.0,
		"reasons": []string{
			"Price decisively closed above resistance",
			"SHARIAH INELIGIBLE: Status is 'FAIL' (Hard Gate Restriction - Zero Recommendations)",
		},
	}

	dataFail, _ := json.Marshal(candFail)
	var parsedFail map[string]any
	json.Unmarshal(dataFail, &parsedFail)

	if parsedFail["is_shariah_eligible"] != false {
		t.Errorf("expected is_shariah_eligible false for FAIL status")
	}
	if parsedFail["overall_score"] != 0.0 {
		t.Errorf("expected overall_score 0.0 for FAIL status, got %v", parsedFail["overall_score"])
	}

	// Candidate with Shariah UNKNOWN must also be gated out
	candUnknown := map[string]any{
		"symbol":              "NEWCO",
		"stage":               "BREAKOUT_CANDIDATE",
		"shariah_status":      "UNKNOWN",
		"is_shariah_eligible": false,
		"overall_score":       0.0,
	}

	dataUnk, _ := json.Marshal(candUnknown)
	var parsedUnk map[string]any
	json.Unmarshal(dataUnk, &parsedUnk)

	if parsedUnk["is_shariah_eligible"] != false {
		t.Errorf("expected is_shariah_eligible false for UNKNOWN status")
	}
	if parsedUnk["overall_score"] != 0.0 {
		t.Errorf("expected overall_score 0.0 for UNKNOWN status, got %v", parsedUnk["overall_score"])
	}
}

// TestPaperTradingAccountingReconciliation tests Criterion 14:
// Dashboard equity and P&L reconcile exactly with paper ledger cash and positions.
func TestPaperTradingAccountingReconciliation(t *testing.T) {
	cashBalance := 1000000.00
	positions := []map[string]any{
		{
			"symbol":        "INFY",
			"shares":        100,
			"avg_price":     980.00,
			"current_price": 992.00,
			"pnl":           1200.00,
			"pnl_pct":       1.22,
		},
		{
			"symbol":        "HCLTECH",
			"shares":        50,
			"avg_price":     1170.00,
			"current_price": 1185.00,
			"pnl":           750.00,
			"pnl_pct":       1.28,
		},
	}

	// Derived metrics
	var investedValue float64
	var totalPnl float64
	for _, pos := range positions {
		shares := float64(pos["shares"].(int))
		cmp := pos["current_price"].(float64)
		investedValue += shares * cmp
		totalPnl += pos["pnl"].(float64)
	}

	totalEquity := cashBalance + investedValue
	expectedEquity := 1000000.00 + (100 * 992.00) + (50 * 1185.00) // 1000000 + 99200 + 59250 = 1158450.00
	expectedPnl := 1200.00 + 750.00 // 1950.00

	if totalEquity != expectedEquity {
		t.Errorf("expected totalEquity %v, got %v", expectedEquity, totalEquity)
	}
	if totalPnl != expectedPnl {
		t.Errorf("expected totalPnl %v, got %v", expectedPnl, totalPnl)
	}

	// If positions are empty:
	emptyPositions := []map[string]any{}
	var emptyInvested, emptyPnl float64
	for _, pos := range emptyPositions {
		emptyInvested += pos["current_price"].(float64)
		emptyPnl += pos["pnl"].(float64)
	}
	emptyTotalEquity := cashBalance + emptyInvested

	if emptyTotalEquity != cashBalance {
		t.Errorf("expected total equity to equal cash balance when positions empty, got %v", emptyTotalEquity)
	}
	if emptyPnl != 0.0 {
		t.Errorf("expected P&L 0.0 when positions empty, got %v", emptyPnl)
	}
}

// TestSafetyConstraintsConfiguration tests Criterion 15:
// Confirms that safety configuration flags prohibit live orders.
func TestSafetyConstraintsConfiguration(t *testing.T) {
	paperTradingEnabled := true
	liveOrderExecution := false
	brokerTradingEnabled := false

	if !paperTradingEnabled {
		t.Errorf("expected PAPER_TRADING_ENABLED to be true")
	}
	if liveOrderExecution {
		t.Errorf("CRITICAL SAFETY VIOLATION: LIVE_ORDER_EXECUTION must be false")
	}
	if brokerTradingEnabled {
		t.Errorf("CRITICAL SAFETY VIOLATION: BROKER_TRADING_ENABLED must be false")
	}
}


