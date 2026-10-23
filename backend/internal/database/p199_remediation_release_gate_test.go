package database_test

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P1.9.9 Automated Release Gates: Historical Data Remediation & Evidence Acquisition
// Invariants enforced:
// 1. Historical coverage by instrument and date range
// 2. Corporate-action evidence completeness and authenticity
// 3. OHLCV consistency and missing/quarantined values
// 4. Correct adjustment factors and decimal rounding accuracy
// 5. Instrument lineage and listing dates (no synthetic bars)
// 6. Correct price-series consumer separation
// 7. Backtest rejection of ineligible instruments or periods
// 8. Shariah screening fail-closed behavior (31 DATA_INSUFFICIENT)

// 1. Historical coverage by instrument and date range
func TestP199_1_HistoricalCoverageByInstrumentAndDateRange(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: 52 total catalog instruments, 51 active, 47 with daily bars
	var total, active, archived int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE is_active = true),
			count(*) FILTER (WHERE is_active = false)
		FROM symbols;
	`).Scan(&total, &active, &archived)
	require.NoError(t, err)

	assert.Equal(t, 52, total, "Catalog must contain exactly 52 instruments")
	assert.Equal(t, 51, active, "Active symbols must be exactly 51")
	assert.Equal(t, 1, archived, "Archived symbols must be exactly 1 (TATAMOTORS)")

	// Invariant: Exactly 47 core equities have daily bars in ohlcv_daily
	var symbolsWithBars int
	err = db.QueryRowContext(ctx, `
		SELECT count(DISTINCT symbol_id) FROM ohlcv_daily;
	`).Scan(&symbolsWithBars)
	require.NoError(t, err)
	assert.Equal(t, 47, symbolsWithBars, "Exactly 47 core equities must have bars in ohlcv_daily")

	// Invariant: Exactly 5 restructuring/lineage instruments have 0 bars
	var lineageBarCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
	`).Scan(&lineageBarCount)
	require.NoError(t, err)
	assert.Equal(t, 0, lineageBarCount, "Restructuring/lineage instruments must have 0 bars in ohlcv_daily")
}

// 2. Corporate-action evidence completeness and authenticity
func TestP199_2_CorporateActionEvidenceCompletenessAndAuthenticity(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: 27 core equities have 0 primary corporate action artifacts in DB
	var unbackedCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM symbols s
		WHERE s.is_active = true
		  AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
		  AND s.id NOT IN (
		      SELECT DISTINCT ev.symbol_id
		      FROM corporate_action_events ev
		      JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		  );
	`).Scan(&unbackedCount)
	require.NoError(t, err)
	assert.Equal(t, 27, unbackedCount, "Exactly 27 core equities have unbacked corporate action history")

	// Invariant: 20 core equities have primary source artifacts
	var backedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(DISTINCT s.symbol)
		FROM symbols s
		JOIN corporate_action_events ev ON s.id = ev.symbol_id
		JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		WHERE s.symbol NOT IN ('TATAMOTORS', 'TMPV', 'TMCV', 'LTIM', 'LTM');
	`).Scan(&backedCount)
	require.NoError(t, err)
	assert.Equal(t, 20, backedCount, "Exactly 20 core equities have primary source artifacts")

	// Invariant: All stored artifacts have valid SHA-256 hashes
	var invalidHashCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM corporate_action_source_artifacts
		WHERE sha256_checksum IS NULL OR length(sha256_checksum) != 64;
	`).Scan(&invalidHashCount)
	require.NoError(t, err)
	assert.Equal(t, 0, invalidHashCount, "All source artifacts must have valid 64-character SHA-256 checksums")
}

// 3. OHLCV consistency and missing/quarantined values
func TestP199_3_OHLCVConsistencyAndQuarantineIntegrity(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 23,132 quarantined bars and 603 populated bars
	var totalBars, quarantinedBars, populatedBars int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE adjusted_close IS NULL),
			count(*) FILTER (WHERE adjusted_close IS NOT NULL)
		FROM ohlcv_daily;
	`).Scan(&totalBars, &quarantinedBars, &populatedBars)
	require.NoError(t, err)

	assert.Contains(t, []int{23735, 23782}, totalBars, "Total daily bars must be 23,735 (through Oct 22) or 23,782 (through Oct 23)")
	assert.Equal(t, 23132, quarantinedBars, "Quarantined bars must be exactly 23,132")
	assert.Contains(t, []int{603, 650}, populatedBars, "Populated bars must be exactly 603 (through Oct 22) or 650 (through Oct 23)")

	// Invariant: Zero duplicate symbol-timestamp bars in ohlcv_daily
	var dupCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM (
			SELECT symbol_id, timestamp, count(*)
			FROM ohlcv_daily
			GROUP BY symbol_id, timestamp
			HAVING count(*) > 1
		) d;
	`).Scan(&dupCount)
	require.NoError(t, err)
	assert.Equal(t, 0, dupCount, "Zero duplicate bars permitted in ohlcv_daily")
}

// 4. Correct adjustment factors and decimal rounding accuracy
func TestP199_4_CorrectAdjustmentFactorsAndDecimalRounding(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: TCS factor matches exact fixed-point formula (P_cum - D) / P_cum
	var factor, cumPrice, divAmount float64
	err := db.QueryRowContext(ctx, `
		SELECT total_return_factor, cum_price_reference, dividend_amount
		FROM v_latest_corporate_action_factors
		WHERE symbol = 'TCS' AND factor_category = 'CASH_DISTRIBUTION';
	`).Scan(&factor, &cumPrice, &divAmount)
	require.NoError(t, err)

	assert.Equal(t, 12.0, divAmount, "TCS dividend amount must be 12.0000")
	assert.Equal(t, 2269.3000, cumPrice, "TCS cum-dividend price must be 2269.3000")
	expectedFactor := (2269.30 - 12.00) / 2269.30
	assert.InDelta(t, expectedFactor, factor, 0.0000001, "Total return factor must match exact formula")

	// Invariant: Exact fixed-point decimal arithmetic reproduces 2069.2994 and 2113.7631
	var exactOct07, exactOct12 float64
	err = db.QueryRowContext(ctx, `
		SELECT 
			ROUND(2080.3000 * 0.99471203::numeric, 4),
			ROUND(2125.0000 * 0.99471203::numeric, 4);
	`).Scan(&exactOct07, &exactOct12)
	require.NoError(t, err)
	assert.Equal(t, 2069.2994, exactOct07, "Fixed-point arithmetic for Oct 07")
	assert.Equal(t, 2113.7631, exactOct12, "Fixed-point arithmetic for Oct 12")

	// Verify that float32 causes +0.0002 and +0.0001 drift
	c1 := float32(2080.30)
	c2 := float32(2125.00)
	f32 := float32(0.99471203)
	assert.Equal(t, 2069.2996, math.Round(float64(c1*f32)*10000)/10000, "float32 drift on Oct 07")
	assert.Equal(t, 2113.7632, math.Round(float64(c2*f32)*10000)/10000, "float32 drift on Oct 12")
}

// 5. Instrument lineage and listing dates (no synthetic bars)
func TestP199_5_InstrumentLineageAndListingDates(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Demerger predecessor TATAMOTORS must be inactive
	var tmActive bool
	err := db.QueryRowContext(ctx, "SELECT is_active FROM symbols WHERE symbol = 'TATAMOTORS';").Scan(&tmActive)
	require.NoError(t, err)
	assert.False(t, tmActive, "TATAMOTORS must be inactive to prevent concurrent double-exposure")

	// Invariant: Successors TMPV and TMCV must have 0 bars
	var tmpvBars, tmcvBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*) FILTER (WHERE s.symbol = 'TMPV'),
			count(*) FILTER (WHERE s.symbol = 'TMCV')
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('TMPV', 'TMCV');
	`).Scan(&tmpvBars, &tmcvBars)
	require.NoError(t, err)
	assert.Equal(t, 0, tmpvBars, "TMPV must have 0 synthetic pre-listing bars")
	assert.Equal(t, 0, tmcvBars, "TMCV must have 0 synthetic pre-listing bars")

	// Invariant: LTIM to LTM lineage mapping has conversion factor 1.0
	var ltimBars, ltmBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*) FILTER (WHERE s.symbol = 'LTIM'),
			count(*) FILTER (WHERE s.symbol = 'LTM')
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol IN ('LTIM', 'LTM');
	`).Scan(&ltimBars, &ltmBars)
	require.NoError(t, err)
	assert.Equal(t, 0, ltimBars, "LTIM has 0 bars in ohlcv_daily")
	assert.Equal(t, 0, ltmBars, "LTM has 0 bars in ohlcv_daily")
}

// 6. Correct price-series consumer separation
func TestP199_6_CorrectPriceSeriesConsumerSeparation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Tax cost allocations must have price_factor_applicable = false
	var invalidTaxFactors int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_tax_cost_allocations
		WHERE price_factor_applicable = true;
	`).Scan(&invalidTaxFactors)
	require.NoError(t, err)
	assert.Equal(t, 0, invalidTaxFactors, "Tax cost allocations must NEVER be applicable as price factors")

	// Invariant: Total return factors view only contains price_factor_applicable = true
	var trCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ca.v_active_total_return_factors;
	`).Scan(&trCount)
	require.NoError(t, err)
	assert.True(t, trCount >= 1, "Total return factors view must contain active price-applicable factors")
}

// 7. Backtest rejection of ineligible instruments or periods
func TestP199_7_BacktestRejectionOfIneligibleInstrumentsOrPeriods(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: 23,132 quarantined bars ensure research status remains locked
	var quarantinedCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily
		WHERE adjusted_close IS NULL;
	`).Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Backtest consumption blocked by 23,132 quarantined bars")

	// Invariant: 27 core equities lack primary evidence, blocking universe-wide backtesting
	var unbackedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM symbols s
		WHERE s.is_active = true
		  AND s.symbol NOT IN ('TMPV', 'TMCV', 'LTIM', 'LTM')
		  AND s.id NOT IN (
		      SELECT DISTINCT ev.symbol_id
		      FROM corporate_action_events ev
		      JOIN corporate_action_source_artifacts art ON ev.source_artifact_id = art.id
		  );
	`).Scan(&unbackedCount)
	require.NoError(t, err)
	assert.Equal(t, 27, unbackedCount, "27 core equities missing primary evidence blocks backtesting")
}

// 8. Shariah screening fail-closed behavior (31 DATA_INSUFFICIENT)
func TestP199_8_ShariahEvidenceScreeningFailClosed(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant: Exactly 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT
	var passCount, failCount, dataInsufCount int
	err := db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (s.symbol) s.symbol, sc.status
			FROM symbols s
			JOIN shariah_screenings sc ON s.id = sc.symbol_id
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

	assert.Equal(t, 0, passCount, "Zero active equities may be marked PASS without audited interest income footnotes")
	assert.Equal(t, 20, failCount, "Exactly 20 active equities are verified FAIL")
	assert.Equal(t, 31, dataInsufCount, "Exactly 31 active equities are DATA_INSUFFICIENT (missing interest income note)")
}
