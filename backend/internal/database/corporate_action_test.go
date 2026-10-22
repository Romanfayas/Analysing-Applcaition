package database_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "github.com/lib/pq"

	"github.com/halal-equity/backend/internal/database"
)

func getTestDB(t *testing.T) *database.DB {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		host := os.Getenv("DB_HOST")
		if host == "" {
			host = "192.168.18.43" // Docker host LAN IP to reach postgres container
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

func TestActivePriceAdjustmentFactorsIsolation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. TCS should resolve exclusively to Revision 3 with exact factor 0.99471203
	tcsFactors, err := db.GetActivePriceAdjustmentFactors(ctx, "TCS")
	require.NoError(t, err)
	require.Len(t, tcsFactors, 1, "TCS must have exactly 1 active corporate action factor")

	tcs := tcsFactors[0]
	assert.Equal(t, 3, tcs.RevisionNumber, "TCS must resolve to latest Revision 3")
	assert.Equal(t, "CASH_DISTRIBUTION", tcs.FactorCategory)
	assert.True(t, tcs.PriceFactorApplicable)
	assert.InDelta(t, 0.99471203, tcs.TotalReturnFactor, 0.00000001, "Total return factor must match 0.99471203")
	assert.Equal(t, 1.0, tcs.NominalPriceFactor, "Nominal price factor for cash dividend must be 1.0")
	assert.Equal(t, 1.0, tcs.VolumeAdjustmentFactor, "Volume adjustment factor for cash dividend must be 1.0")
	assert.True(t, tcs.IsVerified)
	assert.Equal(t, "UNDISPUTED", tcs.DisputeStatus)

	// 2. TATAMOTORS must return ZERO active price adjustment factors (demerger tax allocation rejected)
	tmFactors, err := db.GetActivePriceAdjustmentFactors(ctx, "TATAMOTORS")
	require.NoError(t, err)
	assert.Empty(t, tmFactors, "TATAMOTORS tax cost allocation must be rejected for price adjustments; superseded rev 1 must never leak")

	// 3. TATAMOTORS tax cost allocation must be available via dedicated tax view
	tmTax, err := db.GetTaxCostAllocations(ctx, "TATAMOTORS")
	require.NoError(t, err)
	require.Len(t, tmTax, 1, "TATAMOTORS must have 1 tax cost allocation entry")
	assert.Equal(t, 2, tmTax[0].RevisionNumber)
	assert.Equal(t, "TAX_COST_ALLOCATION", tmTax[0].FactorCategory)
	assert.False(t, tmTax[0].PriceFactorApplicable)
	require.NotNil(t, tmTax[0].TaxCostAllocationPct)
	assert.InDelta(t, 68.85, *tmTax[0].TaxCostAllocationPct, 0.001)

	// 4. LTIM must return ZERO active price adjustment factors (symbology lineage rejected)
	ltimFactors, err := db.GetActivePriceAdjustmentFactors(ctx, "LTIM")
	require.NoError(t, err)
	assert.Empty(t, ltimFactors, "LTIM symbol rename must be rejected for price adjustments")
}

func TestFactorIngestionFailClosedTrigger(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Attempting to insert an UNSPECIFIED factor with price_factor_applicable=true must fail
	errUnspecified := db.QueryRowContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, factor_category, event_id, ex_date, effective_date,
			adjustment_factor, source_reference, revision_number, is_verified, price_factor_applicable
		) VALUES (
			9, 'STOCK_SPLIT', 'UNSPECIFIED', 'TEST:STOCK_SPLIT:1', '2026-12-01', '2026-12-01',
			0.5, 'TEST', 1, true, true
		) RETURNING id;
	`).Scan(new(int))
	require.Error(t, errUnspecified)
	assert.Contains(t, errUnspecified.Error(), "Ingestion rejected: factor_category UNSPECIFIED cannot have price_factor_applicable=true")

	// Attempting to insert a TAX_COST_ALLOCATION with price_factor_applicable=true must fail
	errTax := db.QueryRowContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, factor_category, event_id, ex_date, effective_date,
			adjustment_factor, source_reference, revision_number, is_verified, price_factor_applicable
		) VALUES (
			24, 'DEMERGER', 'TAX_COST_ALLOCATION', 'TEST:DEMERGER:1', '2026-12-01', '2026-12-01',
			0.6885, 'TEST', 1, true, true
		) RETURNING id;
	`).Scan(new(int))
	require.Error(t, errTax)
	assert.Contains(t, errTax.Error(), "Ingestion rejected: factor_category TAX_COST_ALLOCATION cannot have price_factor_applicable=true")

	// Attempting to insert a DISPUTED factor with price_factor_applicable=true must fail
	errDisputed := db.QueryRowContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, factor_category, event_id, ex_date, effective_date,
			adjustment_factor, source_reference, revision_number, is_verified, price_factor_applicable,
			dispute_status
		) VALUES (
			9, 'STOCK_SPLIT', 'SHARE_TRANSFORMATION', 'TEST:STOCK_SPLIT:2', '2026-12-01', '2026-12-01',
			0.5, 'TEST', 1, true, true, 'DISPUTED_RATIO'
		) RETURNING id;
	`).Scan(new(int))
	require.Error(t, errDisputed)
	assert.Contains(t, errDisputed.Error(), "Ingestion rejected: disputed factor status (DISPUTED_RATIO) cannot have price_factor_applicable=true")
}

func TestNominalVsTotalReturnViewsIsolation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. TCS dividend must NOT appear in ca.v_active_nominal_price_factors
	nominalFactors, err := db.GetActiveNominalPriceFactors(ctx, "TCS")
	require.NoError(t, err)
	assert.Empty(t, nominalFactors, "Cash dividends must be strictly excluded from nominal price factors view")

	// 2. TCS dividend MUST appear in ca.v_active_total_return_factors with factor 0.99471203
	totalReturnFactors, err := db.GetActiveTotalReturnFactors(ctx, "TCS")
	require.NoError(t, err)
	require.Len(t, totalReturnFactors, 1, "TCS must appear in total return factors view")
	assert.InDelta(t, 0.99471203, totalReturnFactors[0].TotalReturnFactor, 0.00000001)
	assert.Equal(t, 3, totalReturnFactors[0].RevisionNumber)
}

func TestSourceArtifactsAppendOnlyGuard(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Direct UPDATE on corporate_action_source_artifacts must be blocked by trigger
	_, errUpdate := db.ExecContext(ctx, `
		UPDATE corporate_action_source_artifacts
		SET validation_status = 'MUTATED'
		WHERE id = (SELECT id FROM corporate_action_source_artifacts LIMIT 1);
	`)
	require.Error(t, errUpdate, "UPDATE on corporate_action_source_artifacts must fail closed")
	assert.Contains(t, errUpdate.Error(), "corporate_action_source_artifacts is append-only")

	// Direct DELETE on corporate_action_source_artifacts must be blocked by trigger
	_, errDelete := db.ExecContext(ctx, `
		DELETE FROM corporate_action_source_artifacts
		WHERE id = (SELECT id FROM corporate_action_source_artifacts LIMIT 1);
	`)
	require.Error(t, errDelete, "DELETE on corporate_action_source_artifacts must fail closed")
	assert.Contains(t, errDelete.Error(), "corporate_action_source_artifacts is append-only")
}

func TestCorporateActionEventsIdempotency(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Query events for TCS
	events, err := db.GetCorporateActionEvents(ctx, "TCS")
	require.NoError(t, err)
	assert.NotEmpty(t, events)

	// Verify all events have distinct non-empty event_ids
	eventIDs := make(map[string]bool)
	for _, ev := range events {
		assert.NotEmpty(t, ev.EventID)
		assert.False(t, eventIDs[ev.EventID], "Event ID %s must be globally distinct", ev.EventID)
		eventIDs[ev.EventID] = true
	}
}

func TestNoActiveFactorHasDefaultEventID(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Verify that no active or latest corporate action factor in the database has event_id == 'DEFAULT' or empty
	rows, err := db.QueryContext(ctx, `
		SELECT symbol, action_type, factor_category, event_id, revision_number
		FROM ca.v_latest_corporate_action_factors;
	`)
	require.NoError(t, err)
	defer rows.Close()

	count := 0
	for rows.Next() {
		var sym, actionType, category, eventID string
		var rev int
		require.NoError(t, rows.Scan(&sym, &actionType, &category, &eventID, &rev))
		assert.NotEqual(t, "DEFAULT", eventID, "Factor for %s must not have default event_id", sym)
		assert.NotEmpty(t, eventID, "Factor for %s must have non-empty event_id", sym)
		assert.Contains(t, eventID, ":", "Factor event_id must be deterministic and namespaced")
		count++
	}
	assert.GreaterOrEqual(t, count, 3, "Must have at least 3 resolved latest factor entities")

	// 2. Inserting any factor with event_id='DEFAULT' must be rejected fail-closed by trigger
	_, errDefault := db.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, factor_category, event_id, ex_date, effective_date,
			adjustment_factor, nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, revision_number
		) VALUES (
			(SELECT id FROM symbols WHERE symbol = 'TCS'), 'CASH_DIVIDEND', 'CASH_DISTRIBUTION', 'DEFAULT', '2026-10-14', '2026-10-14',
			0.99471203, 1.0, 0.99471203, 1.0,
			'TEST_DEFAULT_REJECT', 99
		);
	`)
	require.Error(t, errDefault, "Inserting factor with event_id=DEFAULT must fail closed")
	assert.Contains(t, errDefault.Error(), "DEFAULT or blank is prohibited")
}

func TestUniverseCoverageReconciliation52Symbols(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	coverage, err := db.GetUniverseHarvestingCoverage(ctx)
	require.NoError(t, err)
	assert.Equal(t, 52, len(coverage), "Coverage matrix must span exactly 52 total universe instruments")

	coreCount := 0
	lineageCount := 0
	totalEvents := 0

	for _, c := range coverage {
		if c.InstrumentCategory == "CORE_EQUITY" {
			coreCount++
			assert.True(t, c.IsActive, "Core equity %s must be active", c.Symbol)
		} else {
			lineageCount++
		}
		totalEvents += c.EventsCount
	}

	assert.Equal(t, 47, coreCount, "Must have exactly 47 active core equities with daily OHLCV")
	assert.Equal(t, 5, lineageCount, "Must have exactly 5 lineage instruments (2 rename, 3 demerger)")
	assert.Equal(t, 64, totalEvents, "Total events across canonical universe must reconcile to exactly 64 in P1.8.15")
}

func TestP1813DataReadinessAndAdjustedPriceAudit(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Audit adjusted price counts: 23,594 total, 462 populated, 23,132 quarantined (NULL)
	var totalBars, popAdj, nullAdj int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*) as total_bars,
			count(adjusted_close) as pop_adj,
			count(*) - count(adjusted_close) as null_adj
		FROM ohlcv_daily;
	`).Scan(&totalBars, &popAdj, &nullAdj)
	require.NoError(t, err)
	assert.Contains(t, []int{23641, 23688, 23735}, totalBars, "Total daily bars must be 23641 (through Oct 19), 23688 (through Oct 21), or 23735 (through Oct 22)")
	assert.Contains(t, []int{509, 556, 603}, popAdj, "Populated adjusted_close must be 509 (through Oct 19), 556 (through Oct 21), or 603 (through Oct 22)")
	assert.Equal(t, 23132, nullAdj, "Quarantined NULL adjusted_close must be exactly 23132 (97.85% quarantine invariant)")

	// 2. Verify that ONLY TCS has adjusted_close != close across all populated bars
	var diffSymbols int
	err = db.QueryRowContext(ctx, `
		SELECT count(DISTINCT s.symbol)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE o.adjusted_close IS NOT NULL AND o.adjusted_close != o.close;
	`).Scan(&diffSymbols)
	require.NoError(t, err)
	assert.Equal(t, 1, diffSymbols, "Exactly 1 symbol (TCS) may have adjusted_close != close")

	var tcsDiffBars int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol = 'TCS' AND o.adjusted_close != o.close;
	`).Scan(&tcsDiffBars)
	require.NoError(t, err)
	assert.Equal(t, 7, tcsDiffBars, "TCS must have exactly 7 bars with adjusted_close != close prior to ex-date 2026-10-14")

	// 3. Verify HCLTECH has 0 unverified vendor adjusted bars (all 8 quarantined to NULL)
	var hclDiffBars int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol = 'HCLTECH' AND o.adjusted_close IS NOT NULL AND o.adjusted_close != o.close;
	`).Scan(&hclDiffBars)
	require.NoError(t, err)
	assert.Equal(t, 0, hclDiffBars, "HCLTECH must have 0 unverified vendor adjusted bars")

	// 4. Verify 7-state data readiness taxonomy
	coverage, err := db.GetUniverseHarvestingCoverage(ctx)
	require.NoError(t, err)

	stateCounts := make(map[string]int)
	for _, c := range coverage {
		stateCounts[c.DataReadinessState]++
		assert.Contains(t, []string{
			"SOURCE_MISSING", "ACCESS_DENIED", "ARTIFACT_DEPOSITED",
			"PARSE_VALIDATED", "PRIMARY_SOURCE_VERIFIED", "FACTOR_VALIDATED",
			"ELIGIBLE_FOR_SPECIFIC_USE",
		}, c.DataReadinessState, "Symbol %s has invalid data readiness state %s", c.Symbol, c.DataReadinessState)
	}

	assert.Equal(t, 1, stateCounts["ELIGIBLE_FOR_SPECIFIC_USE"], "Exactly 1 symbol (TCS) is ELIGIBLE_FOR_SPECIFIC_USE")
	assert.Equal(t, 2, stateCounts["FACTOR_VALIDATED"], "Exactly 2 symbols (LTIM, TATAMOTORS) are FACTOR_VALIDATED")
	assert.Equal(t, 19, stateCounts["PRIMARY_SOURCE_VERIFIED"], "Exactly 19 symbols are PRIMARY_SOURCE_VERIFIED in P1.8.15")
	assert.Equal(t, 30, stateCounts["SOURCE_MISSING"], "Exactly 30 symbols are SOURCE_MISSING in P1.8.15 (27 core + 3 lineage)")
	assert.Equal(t, 52, stateCounts["ELIGIBLE_FOR_SPECIFIC_USE"]+stateCounts["FACTOR_VALIDATED"]+stateCounts["PRIMARY_SOURCE_VERIFIED"]+stateCounts["SOURCE_MISSING"])
}

func TestP1815UniverseAndCorporateActionReconciliation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Canonical Instrument Universe Reconciliation (52 instruments)
	var totalSymbols, coreCount, renameCount, demergerCount, activeCount, inactiveCount int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')),
			count(*) FILTER (WHERE symbol IN ('LTIM', 'LTM')),
			count(*) FILTER (WHERE symbol IN ('TATAMOTORS', 'TMPV', 'TMCV')),
			count(*) FILTER (WHERE is_active = true),
			count(*) FILTER (WHERE is_active = false)
		FROM symbols;
	`).Scan(&totalSymbols, &coreCount, &renameCount, &demergerCount, &activeCount, &inactiveCount)
	require.NoError(t, err)

	assert.Equal(t, 52, totalSymbols, "Canonical universe must contain exactly 52 instruments")
	assert.Equal(t, 47, coreCount, "Must have exactly 47 core listed equities")
	assert.Equal(t, 2, renameCount, "Must have exactly 2 rename lineage instruments (LTIM, LTM)")
	assert.Equal(t, 3, demergerCount, "Must have exactly 3 demerger lineage instruments (TATAMOTORS, TMPV, TMCV)")
	assert.Equal(t, 51, activeCount, "Must have exactly 51 active instruments")
	assert.Equal(t, 1, inactiveCount, "Must have exactly 1 inactive instrument (TATAMOTORS pre-demerger)")

	// 2. DIVISLAB presence and bar verification
	var divislabID int
	var divislabActive bool
	err = db.QueryRowContext(ctx, "SELECT id, is_active FROM symbols WHERE symbol = 'DIVISLAB';").Scan(&divislabID, &divislabActive)
	require.NoError(t, err)
	assert.Equal(t, 29, divislabID)
	assert.True(t, divislabActive)

	var divislabBars int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM ohlcv_daily WHERE symbol_id = $1;", divislabID).Scan(&divislabBars)
	require.NoError(t, err)
	assert.Contains(t, []int{503, 504, 505}, divislabBars, "DIVISLAB must have 503 (through Oct 19), 504 (through Oct 21), or 505 (through Oct 22) daily bars")

	// 3. Corporate Action Event Count Reconciliation
	var totalArtifacts, uniqueDocIDs int
	err = db.QueryRowContext(ctx, `
		SELECT count(*), count(DISTINCT document_identifier)
		FROM corporate_action_source_artifacts;
	`).Scan(&totalArtifacts, &uniqueDocIDs)
	require.NoError(t, err)
	assert.Equal(t, 23, totalArtifacts)
	assert.Equal(t, 23, uniqueDocIDs)

	var totalEvents, uniqueEventIDs, coreEvents, lineageEvents int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(DISTINCT event_id),
			count(*) FILTER (WHERE symbol NOT IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV')),
			count(*) FILTER (WHERE symbol IN ('LTIM', 'LTM', 'TATAMOTORS', 'TMPV', 'TMCV'))
		FROM corporate_action_events;
	`).Scan(&totalEvents, &uniqueEventIDs, &coreEvents, &lineageEvents)
	require.NoError(t, err)
	assert.Equal(t, 64, totalEvents, "Must have exactly 64 corporate action events")
	assert.Equal(t, 64, uniqueEventIDs, "All 64 event_ids must be globally unique")
	assert.Equal(t, 57, coreEvents, "Must have exactly 57 events on active core instruments")
	assert.Equal(t, 7, lineageEvents, "Must have exactly 7 events on lineage instruments (4 LTIM + 3 TATAMOTORS)")

	// 4. Action Type Breakdown
	var divCount, splitCount, bonusCount, rightsCount, demCount, otherCount int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*) FILTER (WHERE action_type = 'CASH_DIVIDEND'),
			count(*) FILTER (WHERE action_type = 'STOCK_SPLIT'),
			count(*) FILTER (WHERE action_type = 'BONUS_ISSUE'),
			count(*) FILTER (WHERE action_type = 'RIGHTS_ISSUE'),
			count(*) FILTER (WHERE action_type = 'DEMERGER'),
			count(*) FILTER (WHERE action_type NOT IN ('CASH_DIVIDEND', 'STOCK_SPLIT', 'BONUS_ISSUE', 'RIGHTS_ISSUE', 'DEMERGER'))
		FROM corporate_action_events;
	`).Scan(&divCount, &splitCount, &bonusCount, &rightsCount, &demCount, &otherCount)
	require.NoError(t, err)
	assert.Equal(t, 57, divCount, "Must have 57 cash dividends")
	assert.Equal(t, 0, splitCount, "Must have 0 stock splits")
	assert.Equal(t, 0, bonusCount, "Must have 0 bonus issues")
	assert.Equal(t, 0, rightsCount, "Must have 0 rights issues")
	assert.Equal(t, 1, demCount, "Must have 1 demerger event (TATAMOTORS)")
	assert.Equal(t, 6, otherCount, "Must have 6 circular/other records (4 LTIM + 2 TATAMOTORS)")

	// 5. Historical Data Quarantine Invariant
	var totalBars, populatedBars, quarantinedBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(adjusted_close),
			count(*) - count(adjusted_close)
		FROM ohlcv_daily;
	`).Scan(&totalBars, &populatedBars, &quarantinedBars)
	require.NoError(t, err)
	assert.Contains(t, []int{23641, 23688, 23735}, totalBars)
	assert.Contains(t, []int{509, 556, 603}, populatedBars)
	assert.Equal(t, 23132, quarantinedBars)
}

func TestP1814TCSMultiplicativeFactorAuditAndConsumerIsolation(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Audit TCS row-by-row prices for 2026-10-05 through 2026-10-13
	rows, err := db.QueryContext(ctx, `
		SELECT 
			o.timestamp::date::text,
			o.close,
			o.adjusted_close,
			ROUND((o.close - o.adjusted_close), 4),
			ROUND((o.adjusted_close / o.close), 8)
		FROM ohlcv_daily o
		JOIN symbols s ON o.symbol_id = s.id
		WHERE s.symbol = 'TCS'
		  AND o.timestamp::date >= '2026-10-05'
		  AND o.timestamp::date <= '2026-10-13'
		ORDER BY o.timestamp::date ASC;
	`)
	require.NoError(t, err)
	defer rows.Close()

	type barAudit struct {
		date           string
		rawClose       float64
		storedAdjClose float64
		diff           float64
		ratio          float64
	}

	var audited []barAudit
	for rows.Next() {
		var b barAudit
		err := rows.Scan(&b.date, &b.rawClose, &b.storedAdjClose, &b.diff, &b.ratio)
		require.NoError(t, err)
		audited = append(audited, b)
	}
	require.Len(t, audited, 7, "Must have exactly 7 pre-ex-date TCS bars")

	// Verify constant ratio ~0.99471203 across all 7 bars
	for _, b := range audited {
		assert.InDelta(t, 0.99471203, b.ratio, 0.00000015, "Bar %s ratio must match Revision 3 factor", b.date)
		if b.date == "2026-10-13" {
			// Cum-dividend date: raw close = 2269.30, diff is exactly 12.0000
			assert.InDelta(t, 12.0000, b.diff, 0.0001, "Cum-dividend date 2026-10-13 price difference must equal dividend ₹12.0000")
		} else {
			// Prior dates: price difference is proportional to raw close (NOT a flat ₹12)
			assert.NotEqual(t, 12.0000, b.diff, "Prior date %s must have proportional difference, NOT flat ₹12", b.date)
			assert.True(t, b.diff < 12.0, "Prior date %s diff must be strictly less than 12.0 due to lower raw close", b.date)
		}
	}

	// 2. Verify Consumer Isolation: Nominal indicators view has ZERO rows
	var nominalFactorCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM ca.v_active_nominal_price_factors;").Scan(&nominalFactorCount)
	require.NoError(t, err)
	assert.Equal(t, 0, nominalFactorCount, "Nominal price indicators must consume ZERO cash dividend factors")

	// 3. Verify Total Return view has exactly 1 row (TCS Revision 3)
	var trFactorCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM ca.v_active_total_return_factors;").Scan(&trFactorCount)
	require.NoError(t, err)
	assert.Equal(t, 1, trFactorCount, "Total return consumers must resolve to exactly 1 active factor (TCS)")

	var trSymbol, trRef string
	var trRev int
	var trFactor float64
	err = db.QueryRowContext(ctx, `
		SELECT symbol, revision_number, price_factor, source_reference
		FROM ca.v_active_total_return_factors
		LIMIT 1;
	`).Scan(&trSymbol, &trRev, &trFactor, &trRef)
	require.NoError(t, err)
	assert.Equal(t, "TCS", trSymbol)
	assert.Equal(t, 3, trRev)
	assert.InDelta(t, 0.99471203, trFactor, 0.00000001)
	assert.Equal(t, "NSE/CML/2026/DIV/TCS_INT_12", trRef)
}



