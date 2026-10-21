package database_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// restoreTCSBenchmarks restores the exact benchmark values for the 7 pre-ex-date TCS bars.
func restoreTCSBenchmarks(t *testing.T, db *database.DB, ctx context.Context) {
	_, err := db.ExecContext(ctx, `
		UPDATE ohlcv_daily SET adjusted_close = 2103.2190, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-05';
		UPDATE ohlcv_daily SET adjusted_close = 2088.8953, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-06';
		UPDATE ohlcv_daily SET adjusted_close = 2069.2996, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-07';
		UPDATE ohlcv_daily SET adjusted_close = 2065.0222, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-08';
		UPDATE ohlcv_daily SET adjusted_close = 2144.5991, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-09';
		UPDATE ohlcv_daily SET adjusted_close = 2113.7632, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-12';
		UPDATE ohlcv_daily SET adjusted_close = 2257.3000, source = 'NSE_CORPORATE_ACTION' WHERE symbol_id = (SELECT id FROM symbols WHERE symbol = 'TCS') AND timestamp::date = '2026-10-13';
	`)
	require.NoError(t, err)
}

// ======================================================================================
// ADVERSARIAL TEST SUITE (P1.9.3 Invariants 1 through 7)
// ======================================================================================

// TestAdversarial_1_OrdinaryIngestionSuppliesForgedSourceAndFabricatedAdjClose verifies that
// when ordinary vendor ingestion supplies a forged trusted source string ('NSE_CORPORATE_ACTION' or 'PRIMARY_EXCHANGE')
// with a fabricated adjusted_close:
// 1. Quarantined NULL bars remain strictly NULL.
// 2. Unadjusted bars reject the fabricated adjusted close.
// 3. Forged source strings are strictly neutralized and cannot be written to the database.
func TestAdversarial_1_OrdinaryIngestionSuppliesForgedSourceAndFabricatedAdjClose(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Case 1A: Quarantined NULL bar (HCLTECH 2026-10-06)
	var hclID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	tsHCL := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)

	// Verify initial state is NULL
	var initAdjHCL *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&initAdjHCL)
	require.NoError(t, err)
	require.Nil(t, initAdjHCL, "Initial HCLTECH bar must be quarantined NULL")

	// Attempt forged ingestion with 'NSE_CORPORATE_ACTION' and fabricated adjusted close
	fakeAdj1 := 1555.55
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      hclID,
			Timestamp:     tsHCL,
			Open:          1202.0,
			High:          1208.9,
			Low:           1188.2,
			Close:         1204.4,
			Volume:        2500000,
			AdjClose:      &fakeAdj1,
			Source:        "NSE_CORPORATE_ACTION", // Forged trusted source label
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	// Verify database row: adjusted_close MUST REMAIN NULL, source MUST NOT be NSE_CORPORATE_ACTION
	var afterAdjHCL1 *float64
	var afterSourceHCL1 string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&afterAdjHCL1, &afterSourceHCL1)
	require.NoError(t, err)
	assert.Nil(t, afterAdjHCL1, "Quarantined NULL must remain NULL when ordinary ingestion supplies forged source")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", afterSourceHCL1, "Forged NSE_CORPORATE_ACTION source must be rejected")

	// Attempt forged ingestion with 'PRIMARY_EXCHANGE'
	fakeAdj2 := 1666.66
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      hclID,
			Timestamp:     tsHCL,
			Open:          1202.0,
			High:          1208.9,
			Low:           1188.2,
			Close:         1204.4,
			Volume:        2500000,
			AdjClose:      &fakeAdj2,
			Source:        "PRIMARY_EXCHANGE", // Forged trusted source label
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterAdjHCL2 *float64
	var afterSourceHCL2 string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&afterAdjHCL2, &afterSourceHCL2)
	require.NoError(t, err)
	assert.Nil(t, afterAdjHCL2, "Quarantined NULL must remain NULL when ordinary ingestion supplies PRIMARY_EXCHANGE")
	assert.NotEqual(t, "PRIMARY_EXCHANGE", afterSourceHCL2, "Forged PRIMARY_EXCHANGE source must be rejected")

	// Case 1B: Unadjusted bar (INFY 2026-10-06, where adjusted_close == close)
	var infyID int
	err = db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'INFY';").Scan(&infyID)
	require.NoError(t, err)

	tsINFY := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)
	var initOpenINFY, initHighINFY, initLowINFY, initCloseINFY, initAdjINFY float64
	var initVolINFY int64
	err = db.QueryRowContext(ctx, "SELECT open, high, low, close, volume, adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", infyID, tsINFY).Scan(
		&initOpenINFY, &initHighINFY, &initLowINFY, &initCloseINFY, &initVolINFY, &initAdjINFY,
	)
	require.NoError(t, err)
	require.Equal(t, initCloseINFY, initAdjINFY, "INFY bar must initially be unadjusted")

	// Ingestion supplies fabricated adjusted close with forged source
	fakeAdjINFY := 2999.99
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      infyID,
			Timestamp:     tsINFY,
			Open:          initOpenINFY,
			High:          initHighINFY,
			Low:           initLowINFY,
			Close:         initCloseINFY,
			Volume:        initVolINFY,
			AdjClose:      &fakeAdjINFY,
			Source:        "NSE_CORPORATE_ACTION",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterCloseINFY, afterAdjINFY float64
	var afterSourceINFY string
	err = db.QueryRowContext(ctx, "SELECT close, adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", infyID, tsINFY).Scan(&afterCloseINFY, &afterAdjINFY, &afterSourceINFY)
	require.NoError(t, err)
	assert.Equal(t, afterCloseINFY, afterAdjINFY, "Unadjusted bar must retain adjusted_close == close")
	assert.NotEqual(t, fakeAdjINFY, afterAdjINFY, "Fabricated adjusted close must be rejected")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", afterSourceINFY, "Forged source must be rejected")
}

// TestAdversarial_2_DirectSQLAttemptsToPopulateQuarantinedNull verifies that
// raw SQL UPDATE or INSERT statements executed directly on the database cannot bypass
// the quarantine guard trigger.
func TestAdversarial_2_DirectSQLAttemptsToPopulateQuarantinedNull(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 7, 3, 45, 0, 0, time.UTC)

	// Verify initial state is NULL
	var initAdj *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, ts).Scan(&initAdj)
	require.NoError(t, err)
	require.Nil(t, initAdj, "Target HCLTECH bar must be quarantined NULL")

	// Attempt direct raw SQL UPDATE setting adjusted_close and forged source
	_, err = db.ExecContext(ctx, `
		UPDATE ohlcv_daily 
		SET adjusted_close = 1888.50, source = 'NSE_CORPORATE_ACTION' 
		WHERE symbol_id = $1 AND timestamp = $2;
	`, hclID, ts)
	require.NoError(t, err)

	// Verify that the trigger neutralized the bypass attempt
	var afterAdj *float64
	var afterSource string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, ts).Scan(&afterAdj, &afterSource)
	require.NoError(t, err)
	assert.Nil(t, afterAdj, "Direct SQL UPDATE must NOT be able to un-quarantine NULL adjusted_close")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", afterSource, "Direct SQL UPDATE must NOT set NSE_CORPORATE_ACTION without verified event")

	// Attempt direct raw SQL INSERT with fabricated adjusted_close and forged source
	dummyTs := time.Date(2028, 1, 1, 3, 45, 0, 0, time.UTC)
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, dummyTs)
	})

	_, err = db.ExecContext(ctx, `
		INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source, quality_status, retrieved_at)
		VALUES ($1, $2, 100.0, 105.0, 95.0, 100.0, 10000, 85.0, 'NSE_CORPORATE_ACTION', 'VALID', NOW());
	`, hclID, dummyTs)
	require.NoError(t, err)

	var insertAdj *float64
	var insertSource string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, dummyTs).Scan(&insertAdj, &insertSource)
	require.NoError(t, err)
	assert.Nil(t, insertAdj, "Direct SQL INSERT with unverified adjustment must have adjusted_close forced to NULL")
	assert.NotEqual(t, "NSE_CORPORATE_ACTION", insertSource, "Direct SQL INSERT must NOT have forged NSE_CORPORATE_ACTION provenance")
}

// TestAdversarial_3_VerifiedAdjustmentFakeOrUnverifiedEventID verifies that
// ApplyVerifiedCorporateActionAdjustment strictly fails when provided with:
// 1. A nonexistent event ID.
// 2. An event ID with unverified status (UNVERIFIED / PARSED_EVENT).
// 3. A non-positive adjustment factor.
// 4. A factor that does not match the verified corporate action factor.
func TestAdversarial_3_VerifiedAdjustmentFakeOrUnverifiedEventID(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	startDate := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	exDate := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

	// 1. Fake/Nonexistent event ID
	_, err := db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.99471203, "FAKE:NONEXISTENT:EVENT")
	assert.Error(t, err, "Nonexistent event ID must be rejected")
	assert.Contains(t, err.Error(), "not verified")

	// 2. Unverified event ID
	var tcsID int
	err = db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	unverifiedEventID := "TEST:UNVERIFIED:TCS:EVENT:1"
	_, err = db.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			$1, $2, 'TCS', 'CASH_DIVIDEND', 'Unverified Test Event',
			'2026-10-14', 'INR', 'UNVERIFIED', 'PARSED_EVENT'
		) ON CONFLICT (symbol_id, ex_date, action_type, event_id) DO NOTHING;
	`, unverifiedEventID, tcsID)
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM corporate_action_events WHERE event_id = $1;", unverifiedEventID)
	})

	_, err = db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.99471203, unverifiedEventID)
	assert.Error(t, err, "Unverified event ID must be rejected")
	assert.Contains(t, err.Error(), "not verified")

	// 3. Non-positive factor
	_, err = db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, -0.9947, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1")
	assert.Error(t, err, "Negative factor must be rejected")

	// 4. Mismatched factor (factor doesn't match total_return_factor in corporate_action_adjustment_factors)
	_, err = db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.85000000, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1")
	assert.Error(t, err, "Mismatched factor must be rejected")
	assert.Contains(t, err.Error(), "does not match verified factor")
}

// TestAdversarial_4_AuthorizedVerifiedEventScoping verifies that
// an authorized corporate action adjustment updates ONLY the designated symbol
// and ONLY bars within the verified date window [startDate, exDate).
func TestAdversarial_4_AuthorizedVerifiedEventScoping(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, db, ctx)
	})

	startDate := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	exDate := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

	// Apply authorized adjustment
	rowsUpdated, err := db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.99471203, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1")
	require.NoError(t, err)
	assert.Equal(t, int64(7), rowsUpdated, "Exactly 7 pre-ex-date trading days must be updated")

	// Invariant A: Exactly 7 TCS bars have source = 'NSE_CORPORATE_ACTION'
	var tcsVerifiedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM ohlcv_daily o 
		JOIN symbols s ON s.id = o.symbol_id 
		WHERE s.symbol = 'TCS' AND o.source = 'NSE_CORPORATE_ACTION';
	`).Scan(&tcsVerifiedCount)
	require.NoError(t, err)
	assert.Equal(t, 7, tcsVerifiedCount, "TCS must have exactly 7 verified bars")

	// Invariant B: TCS bars on or after exDate (2026-10-14) are NOT updated
	var postExDateCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM ohlcv_daily o 
		JOIN symbols s ON s.id = o.symbol_id 
		WHERE s.symbol = 'TCS' 
		  AND o.timestamp::date >= '2026-10-14' 
		  AND o.source = 'NSE_CORPORATE_ACTION';
	`).Scan(&postExDateCount)
	require.NoError(t, err)
	assert.Equal(t, 0, postExDateCount, "Post-ex-date bars must not be updated by pre-ex-date corporate action")

	// Invariant C: Historical TCS bars before 2026-10-05 remain 100% quarantined NULL
	var preCohortNullCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM ohlcv_daily o 
		JOIN symbols s ON s.id = o.symbol_id 
		WHERE s.symbol = 'TCS' 
		  AND o.timestamp::date < '2026-10-05' 
		  AND o.adjusted_close IS NULL;
	`).Scan(&preCohortNullCount)
	require.NoError(t, err)
	assert.True(t, preCohortNullCount > 0, "Pre-cohort TCS bars must remain quarantined NULL")

	// Invariant D: Other symbols (INFY, HCLTECH, RELIANCE) have 0 rows updated
	var otherSymbolsVerifiedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM ohlcv_daily o 
		JOIN symbols s ON s.id = o.symbol_id 
		WHERE s.symbol != 'TCS' AND o.source = 'NSE_CORPORATE_ACTION';
	`).Scan(&otherSymbolsVerifiedCount)
	require.NoError(t, err)
	assert.Equal(t, 0, otherSymbolsVerifiedCount, "No other symbols should have source = NSE_CORPORATE_ACTION")
}

// TestAdversarial_5_VendorOverwriteVerifiedAdjustmentOrProvenance verifies that
// ordinary vendor updates (via UpsertOHLCVBatchWithStats or direct SQL) CANNOT overwrite
// an existing verified corporate-action adjustment or downgrade its provenance.
func TestAdversarial_5_VendorOverwriteVerifiedAdjustmentOrProvenance(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, db, ctx)
	})

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)

	// Check verified baseline
	var initAdj float64
	var initSource string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, ts).Scan(&initAdj, &initSource)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", initSource)
	assert.InDelta(t, 2103.2190, initAdj, 0.0001)

	// 1. Vendor batch upsert attempts to overwrite with raw unadjusted close and vendor source
	conflictingAdj := 2114.3999
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      tcsID,
			Timestamp:     ts,
			Open:          2114.0,
			High:          2140.8999,
			Low:           2063.6001,
			Close:         2114.3999,
			Volume:        1800000,
			AdjClose:      &conflictingAdj,
			Source:        "yahoo_finance",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	// Verify verified state was preserved
	var afterAdj1 float64
	var afterSource1 string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, ts).Scan(&afterAdj1, &afterSource1)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", afterSource1, "Vendor upsert must NOT overwrite source")
	assert.InDelta(t, 2103.2190, afterAdj1, 0.0001, "Vendor upsert must NOT overwrite verified adjusted_close")

	// 2. Direct SQL UPDATE attempts to overwrite
	_, err = db.ExecContext(ctx, `
		UPDATE ohlcv_daily 
		SET adjusted_close = 2114.3999, source = 'yahoo_finance' 
		WHERE symbol_id = $1 AND timestamp = $2;
	`, tcsID, ts)
	require.NoError(t, err)

	var afterAdj2 float64
	var afterSource2 string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, ts).Scan(&afterAdj2, &afterSource2)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", afterSource2, "Direct SQL UPDATE must NOT overwrite source")
	assert.InDelta(t, 2103.2190, afterAdj2, 0.0001, "Direct SQL UPDATE must NOT overwrite verified adjusted_close")
}

// TestAdversarial_6_ConcurrentIngestionRacesCorporateActionUpdate verifies that
// concurrent execution of ordinary vendor ingestion racing with legitimate corporate-action
// adjustments preserves consistency, avoids deadlocks, and maintains fail-closed quarantine.
func TestAdversarial_6_ConcurrentIngestionRacesCorporateActionUpdate(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, db, ctx)
	})

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	var hclID int
	err = db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	tsTCS := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)
	tsHCL := time.Date(2026, 10, 8, 3, 45, 0, 0, time.UTC)
	startDate := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	exDate := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

	const workers = 8
	var wg sync.WaitGroup
	wg.Add(workers + 1)

	// Worker 0: Legitimate Corporate Action updater
	go func() {
		defer wg.Done()
		for i := 0; i < 5; i++ {
			_, _ = db.ApplyVerifiedCorporateActionAdjustment(ctx, "TCS", startDate, exDate, 0.99471203, "NSE:TCS:2026-10-14:CASH_DIVIDEND:1")
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Workers 1..8: Concurrent ordinary vendor ingestion workers
	for w := 1; w <= workers; w++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				// Vendor attempts to push unadjusted close to TCS
				vendorAdjTCS := 2114.3999
				_, _ = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
					{
						SymbolID:      tcsID,
						Timestamp:     tsTCS,
						Open:          2114.0,
						High:          2140.8999,
						Low:           2063.6001,
						Close:         2114.3999,
						Volume:        1800000 + int64(id*100),
						AdjClose:      &vendorAdjTCS,
						Source:        "yahoo_finance",
						QualityStatus: "VALID",
						IntervalMins:  1440,
					},
				})

				// Vendor attempts to push fake adjusted close to quarantined HCLTECH bar
				fakeAdjHCL := float64(1100 + id*10)
				_, _ = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
					{
						SymbolID:      hclID,
						Timestamp:     tsHCL,
						Open:          1185.0,
						High:          1211.8,
						Low:           1176.9,
						Close:         1176.9,
						Volume:        2300000 + int64(id*100),
						AdjClose:      &fakeAdjHCL,
						Source:        "yahoo_finance",
						QualityStatus: "VALID",
						IntervalMins:  1440,
					},
				})
			}
		}(w)
	}

	wg.Wait()

	// Verify TCS state: adjusted_close remains verified, source remains NSE_CORPORATE_ACTION
	var finalAdjTCS float64
	var finalSourceTCS string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, tsTCS).Scan(&finalAdjTCS, &finalSourceTCS)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", finalSourceTCS, "TCS source must remain NSE_CORPORATE_ACTION after race")
	assert.InDelta(t, 2103.2190, finalAdjTCS, 0.0005, "TCS adjusted_close must remain verified after race")

	// Verify HCLTECH state: adjusted_close remains strictly NULL
	var finalAdjHCL *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, tsHCL).Scan(&finalAdjHCL)
	require.NoError(t, err)
	assert.Nil(t, finalAdjHCL, "Quarantined HCLTECH bar must remain NULL after race")
}

// TestAdversarial_7_DatabasePermissionsBoundary verifies that
// database permissions, immutability triggers, and procedure security defend the trust boundary:
// 1. corporate_action_adjustment_factors rows are immutable (UPDATE and DELETE prohibited).
// 2. Unverified factors cannot have price_factor_applicable = true.
// 3. ca.apply_verified_corporate_action runs with SECURITY DEFINER and fixed search_path.
func TestAdversarial_7_DatabasePermissionsBoundary(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	// 1. Verify factor immutability: UPDATE on corporate_action_adjustment_factors is blocked by ca_factor_guard
	_, err = db.ExecContext(ctx, `
		UPDATE corporate_action_adjustment_factors 
		SET total_return_factor = 0.50000000 
		WHERE symbol_id = $1 AND event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1';
	`, tcsID)
	assert.Error(t, err, "Direct UPDATE on corporate_action_adjustment_factors must be blocked")
	assert.Contains(t, err.Error(), "rows are append-only")

	// 2. Verify factor immutability: DELETE on corporate_action_adjustment_factors is blocked
	_, err = db.ExecContext(ctx, `
		DELETE FROM corporate_action_adjustment_factors 
		WHERE symbol_id = $1 AND event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1';
	`, tcsID)
	assert.Error(t, err, "Direct DELETE on corporate_action_adjustment_factors must be blocked")
	assert.Contains(t, err.Error(), "immutable and may not be deleted")

	// 3. Verify ingest guard: Inserting an unverified factor with price_factor_applicable=true is blocked
	_, err = db.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, ex_date, effective_date, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, is_verified, verification_status, factor_category,
			price_factor_applicable, event_id
		) VALUES (
			$1, 'CASH_DIVIDEND', '2026-10-20', '2026-10-20', 0.95,
			1.0, 0.95, 1.0,
			'Unverified Ingest Test', false, 'UNVERIFIED', 'TOTAL_RETURN_FACTOR',
			true, 'TEST:FORGED:EVENT:1'
		);
	`, tcsID)
	assert.Error(t, err, "Ingestion of unverified factor with price_factor_applicable=true must be rejected")
	assert.Contains(t, err.Error(), "unverified factor cannot have price_factor_applicable=true")

	// 4. Verify procedure security properties: SECURITY DEFINER and fixed search_path
	var prosecdef bool
	var proconfigStr string
	err = db.QueryRowContext(ctx, `
		SELECT prosecdef, COALESCE(array_to_string(proconfig, ','), '')
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'ca' AND p.proname = 'apply_verified_corporate_action';
	`).Scan(&prosecdef, &proconfigStr)
	require.NoError(t, err)
	assert.True(t, prosecdef, "ca.apply_verified_corporate_action must be SECURITY DEFINER")
	assert.Contains(t, proconfigStr, "search_path=public, ca, pg_temp", "Procedure must have fixed search_path to prevent hijack")
}

// ======================================================================================
// EXISTING REGRESSION SUITE
// ======================================================================================

// TestQuarantineGuard_ExistingNullIncomingNonNullVendorAdjRemainsNull asserts that
// when an ordinary vendor feed supplies a non-NULL adjusted_close for a quarantined row,
// the fail-closed quarantine policy keeps adjusted_close strictly NULL.
func TestQuarantineGuard_ExistingNullIncomingNonNullVendorAdjRemainsNull(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 6, 3, 45, 0, 0, time.UTC)

	var initAdj *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, ts).Scan(&initAdj)
	require.NoError(t, err)
	require.Nil(t, initAdj, "Initial HCLTECH bar must be quarantined NULL")

	fakeAdj := 1150.0
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      hclID,
			Timestamp:     ts,
			Open:          1202.0,
			High:          1208.9,
			Low:           1188.2,
			Close:         1204.4,
			Volume:        2500000,
			AdjClose:      &fakeAdj,
			Source:        "yahoo_finance",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterAdj *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, ts).Scan(&afterAdj)
	require.NoError(t, err)
	assert.Nil(t, afterAdj, "Quarantined NULL must remain NULL even when vendor supplies non-NULL adjusted_close")
}

// TestQuarantineGuard_ExistingVerifiedNonNullConflictingVendorValue asserts that
// when an ordinary vendor feed supplies conflicting values for a verified corporate-action bar,
// the verified adjusted_close and source provenance are preserved.
func TestQuarantineGuard_ExistingVerifiedNonNullConflictingVendorValue(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	t.Cleanup(func() {
		restoreTCSBenchmarks(t, db, ctx)
	})

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)

	var initAdj float64
	var initSource string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, ts).Scan(&initAdj, &initSource)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", initSource, "TCS bar must have NSE_CORPORATE_ACTION provenance")
	assert.InDelta(t, 2103.2190, initAdj, 0.0001, "TCS bar must have verified adjusted_close")

	conflictingAdj := 2114.3999
	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      tcsID,
			Timestamp:     ts,
			Open:          2114.0,
			High:          2140.8999,
			Low:           2063.6001,
			Close:         2114.3999,
			Volume:        1800000,
			AdjClose:      &conflictingAdj,
			Source:        "yahoo_finance",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterAdj float64
	var afterSource string
	err = db.QueryRowContext(ctx, "SELECT adjusted_close, source FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", tcsID, ts).Scan(&afterAdj, &afterSource)
	require.NoError(t, err)
	assert.Equal(t, "NSE_CORPORATE_ACTION", afterSource, "Verified source provenance must NOT be overwritten by vendor")
	assert.InDelta(t, 2103.2190, afterAdj, 0.0001, "Verified adjusted_close must NOT be altered by conflicting vendor feed")
}

// TestQuarantineGuard_ExistingNullIncomingNullRemainsNull asserts that
// an existing quarantined NULL remains NULL when incoming payload has NULL AdjClose.
func TestQuarantineGuard_ExistingNullIncomingNullRemainsNull(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var hclID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'HCLTECH';").Scan(&hclID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 7, 3, 45, 0, 0, time.UTC)

	_, err = db.UpsertOHLCVBatchWithStats(ctx, []database.OHLCVRecord{
		{
			SymbolID:      hclID,
			Timestamp:     ts,
			Open:          1196.1,
			High:          1202.6,
			Low:           1172.8,
			Close:         1185.0,
			Volume:        2100000,
			AdjClose:      nil,
			Source:        "yahoo_finance",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	})
	require.NoError(t, err)

	var afterAdj *float64
	err = db.QueryRowContext(ctx, "SELECT adjusted_close FROM ohlcv_daily WHERE symbol_id = $1 AND timestamp = $2;", hclID, ts).Scan(&afterAdj)
	require.NoError(t, err)
	assert.Nil(t, afterAdj, "Existing NULL with incoming NULL must remain NULL")
}

// TestQuarantineGuard_RepeatedIngestionIdempotenceAndProvenance asserts that
// repeated ingestion runs are idempotent and do not corrupt source provenance.
func TestQuarantineGuard_RepeatedIngestionIdempotenceAndProvenance(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	ts := time.Date(2026, 10, 5, 3, 45, 0, 0, time.UTC)
	vendorAdj := 2114.3999

	records := []database.OHLCVRecord{
		{
			SymbolID:      tcsID,
			Timestamp:     ts,
			Open:          2114.0,
			High:          2140.8999,
			Low:           2063.6001,
			Close:         2114.3999,
			Volume:        1800000,
			AdjClose:      &vendorAdj,
			Source:        "yahoo_finance",
			QualityStatus: "VALID",
			IntervalMins:  1440,
		},
	}

	_, _ = db.UpsertOHLCVBatchWithStats(ctx, records)
	stats2, err := db.UpsertOHLCVBatchWithStats(ctx, records)
	require.NoError(t, err)

	assert.Equal(t, 1, stats2.Unchanged, "Repeated identical ingestion must register 100% Unchanged")
	assert.Equal(t, 0, stats2.Inserted, "Second run must have 0 inserts")
	assert.Equal(t, 0, stats2.Updated, "Second run must have 0 updates")
}
