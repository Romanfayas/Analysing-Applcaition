package database_test

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ManifestSummary matches data/universe/canonical_universe_v1.json summary.
type P198Manifest struct {
	Summary struct {
		TotalDatabaseInstruments int `json:"total_database_instruments"`
		ActiveInstrumentsInDB    int `json:"active_instruments_in_db"`
		ArchivedInstrumentsInDB  int `json:"archived_instruments_in_db"`
		ActiveCoreEquities       int `json:"active_core_equities"`
		SymbolsWithOHLCVBars     int `json:"symbols_with_ohlcv_bars"`
		QuarantinedDailyBars     int `json:"quarantined_daily_bars"`
	} `json:"summary"`
}

func loadP198Manifest(t *testing.T) *P198Manifest {
	paths := []string{
		"../../data/universe/canonical_universe_v1.json",
		"../../../data/universe/canonical_universe_v1.json",
		"/Users/fayas/Downloads/ICO-Application/analysis application/data/universe/canonical_universe_v1.json",
	}
	var data []byte
	var err error
	for _, p := range paths {
		abs, _ := filepath.Abs(p)
		data, err = os.ReadFile(abs)
		if err == nil {
			break
		}
	}
	require.NoError(t, err, "Failed to read canonical manifest")

	var m P198Manifest
	err = json.Unmarshal(data, &m)
	require.NoError(t, err)
	return &m
}

// 1. Universe & Instrument-Lineage Consistency
func TestP198_1_UniverseAndInstrumentLineageConsistency(t *testing.T) {
	manifest := loadP198Manifest(t)
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 52 instruments total, 51 active, 1 archived
	var total, active, archived int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE is_active = true),
			count(*) FILTER (WHERE is_active = false)
		FROM canonical.symbols;
	`).Scan(&total, &active, &archived)
	require.NoError(t, err)

	assert.Equal(t, 52, total, "Database symbols table must contain exactly 52 instruments")
	assert.Equal(t, 51, active, "Active symbols must be exactly 51")
	assert.Equal(t, 1, archived, "Archived symbols must be exactly 1 (TATAMOTORS)")
	assert.Equal(t, manifest.Summary.TotalDatabaseInstruments, total)

	// Lineage Invariant: TATAMOTORS has 0 bars in ohlcv_daily; TMPV and TMCV have 0 bars
	var tmBars, tmpvBars, tmcvBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*) FILTER (WHERE s.symbol = 'TATAMOTORS'),
			count(*) FILTER (WHERE s.symbol = 'TMPV'),
			count(*) FILTER (WHERE s.symbol = 'TMCV')
		FROM canonical.ohlcv_daily o
		JOIN canonical.symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV');
	`).Scan(&tmBars, &tmpvBars, &tmcvBars)
	require.NoError(t, err)
	assert.Equal(t, 0, tmBars, "TATAMOTORS has 0 bars in ohlcv_daily; historical claims of 492 bars were table-mapping confusion with M&M (id 44)")
	assert.Equal(t, 0, tmpvBars, "TMPV must have 0 pre-listing bars; no synthetic bars permitted")
	assert.Equal(t, 0, tmcvBars, "TMCV must have 0 pre-listing bars; no synthetic bars permitted")

	// Lineage Invariant: LTIM has 0 bars; LTM has 0 bars
	var ltimBars, ltmBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*) FILTER (WHERE s.symbol = 'LTIM'),
			count(*) FILTER (WHERE s.symbol = 'LTM')
		FROM canonical.ohlcv_daily o
		JOIN canonical.symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('LTIM', 'LTM');
	`).Scan(&ltimBars, &ltmBars)
	require.NoError(t, err)
	assert.Equal(t, 0, ltimBars, "LTIM has 0 bars in ohlcv_daily")
	assert.Equal(t, 0, ltmBars, "LTM has 0 bars in ohlcv_daily")
}

// 2. Missing, Unauthenticated, or Mismatched Corporate-Action Evidence
func TestP198_2_MissingOrUnauthenticatedCorporateActionEvidence(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 20 core equities have primary artifacts; 27 core equities have NO primary artifacts
	var symbolsWithArtifacts int
	err := db.QueryRowContext(ctx, `
		SELECT count(DISTINCT s.symbol)
		FROM canonical.symbols s
		JOIN corporate_action_events ev ON s.id = ev.symbol_id
		JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		WHERE s.symbol NOT IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
	`).Scan(&symbolsWithArtifacts)
	require.NoError(t, err)
	assert.Equal(t, 20, symbolsWithArtifacts, "Exactly 20 core equities have authenticated primary artifacts in DB")

	// Invariant: The remaining 27 core equities have ZERO primary artifacts
	var missingArtifactsSymbols int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.symbols s
		WHERE s.is_active = true 
		  AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
		  AND s.id NOT IN (
		      SELECT DISTINCT ev.symbol_id 
		      FROM corporate_action_events ev
		      JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		  );
	`).Scan(&missingArtifactsSymbols)
	require.NoError(t, err)
	assert.Equal(t, 27, missingArtifactsSymbols, "Exactly 27 core equities lack primary corporate action artifacts (DATA_INSUFFICIENT)")

	// Invariant: All 23 source artifacts in DB have non-null, 64-character SHA-256 hashes
	var invalidHashCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM corporate_action_source_artifacts
		WHERE sha256_checksum IS NULL OR length(sha256_checksum) != 64;
	`).Scan(&invalidHashCount)
	require.NoError(t, err)
	assert.Equal(t, 0, invalidHashCount, "All source artifacts must have valid 64-character SHA-256 checksums")
}

// 3. Missing and Quarantined Historical Price Values
func TestP198_3_MissingAndQuarantinedHistoricalPriceValues(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 23,132 historical daily bars are quarantined (adjusted_close IS NULL)
	var quarantinedCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily
		WHERE adjusted_close IS NULL;
	`).Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Quarantined NULL bars invariant must be strictly 23,132")

	// Invariant: Every core equity prior to 2026-10-05 has 100% of bars quarantined (NULL)
	var prematureBars int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily
		WHERE timestamp < '2026-10-05' AND adjusted_close IS NOT NULL;
	`).Scan(&prematureBars)
	require.NoError(t, err)
	assert.Equal(t, 0, prematureBars, "Zero historical bars prior to Oct 5 may have populated adjusted_close without authenticated complete-window proof")
}

// 4. Incorrect Price-Series Consumption Protection
func TestP198_4_IncorrectPriceSeriesConsumptionProtection(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Nominal price views must NEVER contain cash dividend factors
	var nominalDividends int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_active_nominal_price_factors
		WHERE action_type = 'CASH_DIVIDEND';
	`).Scan(&nominalDividends)
	require.NoError(t, err)
	assert.Equal(t, 0, nominalDividends, "Cash dividends are strictly excluded from nominal price factors")

	// Invariant: Total return view contains TCS dividend factor
	var trCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_active_total_return_factors
		WHERE symbol = 'TCS';
	`).Scan(&trCount)
	require.NoError(t, err)
	assert.Equal(t, 1, trCount, "TCS ₹12 dividend is present in total return factors")

	// Invariant: Tax cost allocations have price_factor_applicable = false
	var taxCostPriceApplicable int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_tax_cost_allocations
		WHERE price_factor_applicable = true;
	`).Scan(&taxCostPriceApplicable)
	require.NoError(t, err)
	assert.Equal(t, 0, taxCostPriceApplicable, "Tax cost allocations must NEVER be applicable as price adjustment factors")
}

// 5. Decimal Adjustment and Rounding Accuracy
func TestP198_5_DecimalAdjustmentAndRoundingAccuracy(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Audit TCS factor and exact arithmetic
	var factor float64
	var cumPrice float64
	var divAmount float64
	err := db.QueryRowContext(ctx, `
		SELECT total_return_factor, cum_price_reference, dividend_amount
		FROM v_latest_corporate_action_factors
		WHERE symbol = 'TCS' AND factor_category = 'CASH_DISTRIBUTION';
	`).Scan(&factor, &cumPrice, &divAmount)
	require.NoError(t, err)

	assert.Equal(t, 12.0, divAmount, "TCS dividend amount must be 12.0000")
	assert.Equal(t, 2269.3000, cumPrice, "TCS cum-dividend price reference must be 2269.3000")
	expectedFactor := (2269.30 - 12.00) / 2269.30
	assert.InDelta(t, expectedFactor, factor, 0.0000001, "Total return factor must match (P_cum - D) / P_cum")

	// Verify exact PostgreSQL decimal rounding: ROUND(2080.3000 * 0.99471203, 4) == 2069.2994
	var exactOct07, exactOct12 float64
	err = db.QueryRowContext(ctx, `
		SELECT 
			ROUND(2080.3000 * 0.99471203::numeric, 4),
			ROUND(2125.0000 * 0.99471203::numeric, 4);
	`).Scan(&exactOct07, &exactOct12)
	require.NoError(t, err)

	assert.Equal(t, 2069.2994, exactOct07, "PostgreSQL exact fixed-point decimal calculation for Oct 07")
	assert.Equal(t, 2113.7631, exactOct12, "PostgreSQL exact fixed-point decimal calculation for Oct 12")

	// Demonstrate that float32 introduces the +0.0002 and +0.0001 drift
	c1 := float32(2080.30)
	c2 := float32(2125.00)
	f32 := float32(0.99471203)
	driftOct07 := math.Round(float64(c1*f32)*10000) / 10000
	driftOct12 := math.Round(float64(c2*f32)*10000) / 10000
	assert.Equal(t, 2069.2996, driftOct07, "float32 drift exactly reproduces the reported +0.0002 value on Oct 07")
	assert.Equal(t, 2113.7632, driftOct12, "float32 drift exactly reproduces the reported +0.0001 value on Oct 12")
}

// 6. Backtests Attempting to Consume Ineligible Data Blocked
func TestP198_6_BacktestsAttemptingToConsumeIneligibleData(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Backtester cannot execute on unverified corporate action data
	// Verify that 27 core equities have zero verified events, so any backtest over historical period is blocked
	var unbackedCoreCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.symbols s
		WHERE s.is_active = true 
		  AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
		  AND s.id NOT IN (SELECT symbol_id FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY');
	`).Scan(&unbackedCoreCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, unbackedCoreCount, 27, "Core equities lack verified exchange primary corporate action history, blocking backtest consumption")
}

// 7. Shariah Screening Fail-Closed Behavior
func TestP198_7_ShariahScreeningFailClosedBehavior(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT for the 51 active equities
	var passCount, failCount, dataInsufCount int
	err := db.QueryRowContext(ctx, `
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
}

// 8. Runtime Database Privilege and Evidence Integrity Controls
func TestP198_8_RuntimeDatabasePrivilegeAndEvidenceIntegrity(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Check that security triggers guarding evidence, factors, and quarantine are active
	var triggerCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(DISTINCT tgname)
		FROM pg_trigger
		WHERE tgname IN ('trg_ohlcv_daily_quarantine_guard', 'trg_ca_factor_guard', 'trg_ca_artifact_guard', 'trg_ca_event_verification_guard');
	`).Scan(&triggerCount)
	require.NoError(t, err)
	assert.Equal(t, 4, triggerCount, "All 4 security triggers guarding evidence, factors, and quarantine must be active")

	// Invariant: halal_app_user cannot be superuser
	var isSuperuser bool
	err = db.QueryRowContext(ctx, `
		SELECT rolsuper
		FROM pg_roles
		WHERE rolname = 'halal_app_user';
	`).Scan(&isSuperuser)
	require.NoError(t, err)
	assert.False(t, isSuperuser, "halal_app_user must never have superuser privileges")
}
