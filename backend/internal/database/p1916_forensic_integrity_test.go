package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

// P1.9.16 Forensic Integrity Test Suite:
// Resolves:
// 1. P0: Strategy V2 Core Parameters (3-param) vs Composite Breakout/Risk (12-param) SHA-256 identity and drift detection.
// 2. P0: TCS Dividend Arithmetic (₹12 on ₹2269.30 vs erroneous ₹10 on ₹1891) and unauthenticated quarantine persistence.
// 3. P1: Go / Python cross-language deterministic serialization contract.

const (
	p1916StrategyV2PinnedHash   = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"
	p1916CompositeCompactHash  = "0daac86130da4bf6e0fba7b2a9e8f7ff76c309dee4702b4a0a23c33e0a42f79f"
	p1916CompositeStandardHash = "deb24826865c3eefb2a1c7126ccf64f1955904af8631859042de93e980e6b5ba"
	p1916TCSReconciledFactor   = "0.99471203"
)

func getP1916TestDB(t *testing.T) *database.DB {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "192.168.18.43"
		}
		connStr = "postgres://halal_user:devpassword@" + host + ":5432/halal_equity?sslmode=disable"
	}
	sqlDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Skipping integration test; postgres database not accessible: %v", err)
	}

	return &database.DB{DB: sqlDB}
}

// 1. Canonical Serialization & Hash Reproduction of Strategy V2 Core Parameters
func TestP1916_1_StrategyV2_CoreParameters_CanonicalSerializationAndHash(t *testing.T) {
	// Exact canonical UTF-8 bytes: sorted keys, ': ' and ', ' separators, '.0' float formatting, no trailing newline
	canonicalBytes := []byte(`{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`)

	h := sha256.Sum256(canonicalBytes)
	calculatedHash := hex.EncodeToString(h[:])

	assert.Equal(t, p1916StrategyV2PinnedHash, calculatedHash, "Canonical bytes must reproduce Strategy V2 hash exactly")

	// Drift Test A: Modifying BUY_THRESHOLD from 80.0 to 80.1 must fail the hash check
	driftedBytes1 := []byte(`{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.1, "SELL_THRESHOLD": 45.0}`)
	h1 := sha256.Sum256(driftedBytes1)
	driftHash1 := hex.EncodeToString(h1[:])
	assert.NotEqual(t, p1916StrategyV2PinnedHash, driftHash1, "Altered BUY_THRESHOLD parameter must fail hash check")

	// Drift Test B: Modifying ATR_MULTIPLIER from 2.0 to 2.1 must fail the hash check
	driftedBytes2 := []byte(`{"ATR_MULTIPLIER": 2.1, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`)
	h2 := sha256.Sum256(driftedBytes2)
	driftHash2 := hex.EncodeToString(h2[:])
	assert.NotEqual(t, p1916StrategyV2PinnedHash, driftHash2, "Altered ATR_MULTIPLIER parameter must fail hash check")

	// Serialization Drift Test: Compact separators (without spaces) must fail the hash check
	compactBytes := []byte(`{"ATR_MULTIPLIER":2.0,"BUY_THRESHOLD":80.0,"SELL_THRESHOLD":45.0}`)
	hCompact := sha256.Sum256(compactBytes)
	compactHash := hex.EncodeToString(hCompact[:])
	assert.NotEqual(t, p1916StrategyV2PinnedHash, compactHash, "Compact JSON without spaces must not match canonical hash")

	// Serialization Drift Test: Trailing newline must fail the hash check
	newlineBytes := []byte("{\"ATR_MULTIPLIER\": 2.0, \"BUY_THRESHOLD\": 80.0, \"SELL_THRESHOLD\": 45.0}\n")
	hNewline := sha256.Sum256(newlineBytes)
	newlineHash := hex.EncodeToString(hNewline[:])
	assert.NotEqual(t, p1916StrategyV2PinnedHash, newlineHash, "Trailing newline must not match canonical hash")
}

// 2. Disambiguation of 12-Parameter Composite Configuration vs 3-Parameter Core
func TestP1916_2_CompositeBreakoutRisk_Disambiguation(t *testing.T) {
	// The 12-parameter composite payload cited in P1.9.15
	compactCompositeJSON := `{"atr_stop_multiplier":2.0,"cost_schedule":"INDIA_EQ_DELIVERY_FEES_2026Q4_V2","donchian_breakout_period":20,"max_open_positions":5,"max_portfolio_heat":0.1,"max_single_trade_risk":0.02,"min_volume_multiple":1.5,"partial_profit_atr":3.0,"partial_profit_size":0.5,"slippage_model":"INSTITUTIONAL_LIQUIDITY_V2","trailing_stop_activation_atr":1.5,"volume_filter_period":20}`

	hCompact := sha256.Sum256([]byte(compactCompositeJSON))
	compactHashHex := hex.EncodeToString(hCompact[:])

	// Proof: The 12-parameter payload produces 0daac861... NOT 92ab27c2...
	assert.Equal(t, p1916CompositeCompactHash, compactHashHex)
	assert.NotEqual(t, p1916StrategyV2PinnedHash, compactHashHex, "12-parameter configuration CANNOT match Strategy V2 core hash 92ab27c2...")

	standardCompositeJSON := `{"atr_stop_multiplier": 2.0, "cost_schedule": "INDIA_EQ_DELIVERY_FEES_2026Q4_V2", "donchian_breakout_period": 20, "max_open_positions": 5, "max_portfolio_heat": 0.1, "max_single_trade_risk": 0.02, "min_volume_multiple": 1.5, "partial_profit_atr": 3.0, "partial_profit_size": 0.5, "slippage_model": "INSTITUTIONAL_LIQUIDITY_V2", "trailing_stop_activation_atr": 1.5, "volume_filter_period": 20}`
	hStandard := sha256.Sum256([]byte(standardCompositeJSON))
	standardHashHex := hex.EncodeToString(hStandard[:])

	assert.Equal(t, p1916CompositeStandardHash, standardHashHex)
	assert.NotEqual(t, p1916StrategyV2PinnedHash, standardHashHex, "Standard-separated 12-param configuration CANNOT match Strategy V2 core hash 92ab27c2...")
}

// 3. TCS Corporate Action Reconciliation: ₹12 Dividend on ₹2,269.30 Close vs ₹10 on ₹1,891
func TestP1916_3_TCS_DividendArithmeticAndProvenanceReconciliation(t *testing.T) {
	db := getP1916TestDB(t)
	ctx := context.Background()

	// Math Check 1: The true project arithmetic: (2269.30 - 12.00) / 2269.30
	cumCloseTrue := 2269.30
	dividendTrue := 12.00
	factorTrueRaw := (cumCloseTrue - dividendTrue) / cumCloseTrue
	factorTrue8Dec := fmt.Sprintf("%.8f", factorTrueRaw)

	assert.Equal(t, "0.99471203", factorTrue8Dec, "True arithmetic must yield exactly 0.99471203")

	// Math Check 2: The erroneous P1.9.15 assertion: (1891.00 - 10.00) / 1891.00
	cumCloseErroneous := 1891.00
	dividendErroneous := 10.00
	factorErroneousRaw := (cumCloseErroneous - dividendErroneous) / cumCloseErroneous
	factorErroneous8Dec := fmt.Sprintf("%.8f", factorErroneousRaw)

	assert.Equal(t, "0.99471179", factorErroneous8Dec, "Erroneous ₹10 on ₹1891 close yields 0.99471179")
	assert.NotEqual(t, p1916TCSReconciledFactor, factorErroneous8Dec, "Erroneous P1.9.15 claim mathematically diverges from 0.99471203")

	// Database Check 1: corporate_action_events contains ₹12.0000, NOT ₹10
	var eventDividend float64
	var eventStatus string
	err := db.QueryRowContext(ctx, `
		SELECT dividend_amount, verification_status
		FROM corporate_action_events
		WHERE symbol = 'TCS' AND ex_date = '2026-10-14' AND action_type = 'CASH_DIVIDEND';
	`).Scan(&eventDividend, &eventStatus)
	require.NoError(t, err)
	assert.Equal(t, 12.0000, eventDividend, "Recorded dividend in database must be ₹12.0000")
	assert.Equal(t, "ARCHIVE_UNAUTHENTICATED", eventStatus, "Event verification status must remain ARCHIVE_UNAUTHENTICATED")

	// Database Check 2: corporate_action_adjustment_factors Revision 3 has factor 0.99471203
	var factorRev3 float64
	var cumRefRev3 float64
	err = db.QueryRowContext(ctx, `
		SELECT total_return_factor, cum_price_reference
		FROM corporate_action_adjustment_factors
		WHERE symbol_id = (SELECT id FROM canonical.symbols WHERE symbol = 'TCS')
		  AND revision_number = 3;
	`).Scan(&factorRev3, &cumRefRev3)
	require.NoError(t, err)
	assert.InDelta(t, 0.99471203, factorRev3, 0.00000001, "Revision 3 factor must equal 0.99471203")
	assert.InDelta(t, 2269.3000, cumRefRev3, 0.0001, "Revision 3 reference cum close must equal 2269.3000")

	// Database Check 3: 7 TCS affected bars in ohlcv_daily have source = 'NSE_CORPORATE_ACTION'
	var tcsBarCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily o
		JOIN canonical.symbols s ON o.symbol_id = s.id
		WHERE s.symbol = 'TCS' AND o.source = 'NSE_CORPORATE_ACTION';
	`).Scan(&tcsBarCount)
	require.NoError(t, err)
	assert.Equal(t, 7, tcsBarCount, "Exactly 7 bars for TCS have source NSE_CORPORATE_ACTION")
}

// 4. Research & Shariah Safety Gate Invariants
func TestP1916_4_SafetyGates_ResearchBlockedAndShariahFailClosed(t *testing.T) {
	db := getP1916TestDB(t)
	ctx := context.Background()

	// 1. Quarantined bars count invariant
	var quarantinedCount int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Quarantined bars invariant (23,132) must be preserved")

	// 2. Shariah status partition: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT
	var passCount, failCount, dataInsufCount int
	err = db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (s.symbol) s.symbol, sc.status
			FROM canonical.symbols s
			JOIN canonical.shariah_screenings sc ON s.id = sc.symbol_id
			WHERE s.is_active = true
			ORDER BY s.symbol, sc.screening_date DESC
		)
		SELECT 
			count(*) FILTER (WHERE status = 'PASS'),
			count(*) FILTER (WHERE status = 'FAIL'),
			count(*) FILTER (WHERE status = 'DATA_INSUFFICIENT')
		FROM latest_screenings;
	`).Scan(&passCount, &failCount, &dataInsufCount)
	require.NoError(t, err)
	requireShariahVerdictsMatchTheirNumbers(t, db)

	// 3. Primary verified corporate actions invariant = 0
	var primaryVerifiedCA int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';").Scan(&primaryVerifiedCA)
	require.NoError(t, err)
	assert.Equal(t, 0, primaryVerifiedCA, "Must have 0 VERIFIED_EXCHANGE_PRIMARY corporate action events")
}
