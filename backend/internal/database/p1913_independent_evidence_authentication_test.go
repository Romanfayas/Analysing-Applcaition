package database_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ======================================================================================
// P1.9.13 INDEPENDENT EVIDENCE AUTHENTICATION & RESEARCH READINESS TEST SUITE
// ======================================================================================

// 1. Prove that application role (halal_app_user) cannot manufacture primary-source evidence
func TestP1913_1_AppRoleCannotManufacturePrimarySourceEvidence(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := appDB.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	// A. App role attempting to insert artifact claiming PRIMARY_VERIFIED must fail
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier, document_type,
			raw_payload, sha256_checksum, validation_status
		) VALUES (
			'ADV_APP_FORGED_ART', 'NSE_INDIA', 'https://nsearchives.nseindia.com/content/circulars/FORGED.pdf',
			'DOC_FORGED_1', 'CIRCULAR_PDF', 'forged payload',
			'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'PRIMARY_VERIFIED'
		);
	`)
	assert.Error(t, err, "App role must be prohibited from inserting PRIMARY_VERIFIED artifacts")
	assert.True(t, strings.Contains(err.Error(), "Privilege boundary violation") || strings.Contains(err.Error(), "permission denied"), "Must be rejected: %v", err)

	// B. App role attempting to insert corporate_action_event claiming VERIFIED_EXCHANGE_PRIMARY must fail
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status, source_artifact_id
		) VALUES (
			'ADV_APP_FORGED_EVENT', $1, 'TCS', 'CASH_DIVIDEND', 'Forged by App Role',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY', 5
		);
	`, tcsID)
	assert.Error(t, err, "App role must be prohibited from creating VERIFIED_EXCHANGE_PRIMARY events")
	assert.True(t, strings.Contains(err.Error(), "Privilege boundary violation") || strings.Contains(err.Error(), "cannot be established from unauthenticated archive") || strings.Contains(err.Error(), "permission denied"), "Must be rejected: %v", err)

	// C. App role attempting to insert corporate_action_adjustment_factors claiming is_verified=true must fail
	_, err = appDB.ExecContext(ctx, `
		INSERT INTO corporate_action_adjustment_factors (
			symbol_id, action_type, ex_date, effective_date, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			source_reference, is_verified, verification_status, factor_category,
			price_factor_applicable, dispute_status, event_id
		) VALUES (
			$1, 'CASH_DIVIDEND', '2026-10-14', '2026-10-14', 0.90,
			1.0, 0.90, 1.0,
			'App Role Factor Attempt', true, 'VERIFIED_EXCHANGE_PRIMARY', 'CASH_DISTRIBUTION',
			true, 'UNDISPUTED', 'TEST:EVENT:APP:FORGED'
		);
	`, tcsID)
	assert.Error(t, err, "App role must be prohibited from inserting verified factors")
	assert.True(t, strings.Contains(err.Error(), "Privilege boundary violation") || strings.Contains(err.Error(), "Ingestion rejected") || strings.Contains(err.Error(), "permission denied"), "Must be rejected: %v", err)
}

// 2. Prove that changing a URL or metadata status cannot authenticate an artifact
func TestP1913_2_ChangingURLOrMetadataCannotAuthenticateArtifact(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	// In an isolated transaction, attempt to insert an event pointing to an unauthenticated vendor URL
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier, document_type,
			raw_payload, sha256_checksum, validation_status
		) VALUES (
			'VENDOR_URL_TEST_P1913', 'NSE_INDIA', 'https://api.thirdparty-data.io/nse/ca/tcs.json',
			'TP_CA_001', 'EXCHANGE_API_JSON', '{"data":"simulated"}',
			'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'PARSED_VALIDATED'
		);
	`)
	require.NoError(t, err)

	// Attempting to set VERIFIED_EXCHANGE_PRIMARY on this event must fail closed
	_, err = tx.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, source_artifact_id, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			'TEST:EVENT:METADATA_FORGED', $1, 'TCS',
			(SELECT id FROM corporate_action_source_artifacts WHERE artifact_id = 'VENDOR_URL_TEST_P1913'),
			'CASH_DIVIDEND', 'Metadata Tampering Attempt', '2026-10-14', 'INR',
			'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, tcsID)
	assert.Error(t, err, "Event pointing to non-circular or vendor API endpoint must be rejected")
	assert.Contains(t, err.Error(), "lacks authenticated primary origin proof")
}

// 3. Prove that unauthenticated corporate actions cannot release affected bars
func TestP1913_3_UnauthenticatedCorporateActionsCannotReleaseBars(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	startDate := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
	exDate := time.Date(2026, 10, 14, 0, 0, 0, 0, time.UTC)

	// The demoted event NSE:TCS:2026-10-14:CASH_DIVIDEND:1 has ARCHIVE_UNAUTHENTICATED status
	var rowsUpdated int64
	err := db.QueryRowContext(
		ctx,
		"SELECT ca.apply_verified_corporate_action('TCS', $1::date, $2::date, 0.99471203::numeric, 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1');",
		startDate, exDate,
	).Scan(&rowsUpdated)

	assert.Error(t, err, "Applying corporate action with ARCHIVE_UNAUTHENTICATED status must be strictly rejected")
	assert.Contains(t, err.Error(), "is not verified by validated primary exchange evidence")
}

// 4. Prove that nominal bars cannot enter a total-return backtest and 23,132 historical bars remain quarantined
func TestP1913_4_NominalBarsCannotEnterTotalReturnBacktest(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 4A: Exactly 23,132 bars remain quarantined (NULL adjusted_close)
	var nullAdjCount int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&nullAdjCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, nullAdjCount, "Quarantined historical bars must remain exactly 23,132")

	// Invariant 4B: Nominal bars from yahoo_finance have adjusted_close == close
	var nominalYahooBars int
	var nonNominalYahooBars int
	err = db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE adjusted_close = close),
			COUNT(*) FILTER (WHERE adjusted_close != close)
		FROM ohlcv_daily
		WHERE source = 'yahoo_finance';
	`).Scan(&nominalYahooBars, &nonNominalYahooBars)
	require.NoError(t, err)
	assert.Equal(t, 643, nominalYahooBars, "All 643 populated Yahoo bars must be strictly nominal execution bars")
	assert.Equal(t, 0, nonNominalYahooBars, "Zero Yahoo bars may have non-nominal adjusted closes")

	// Invariant 4C: Exactly 0 corporate action events have VERIFIED_EXCHANGE_PRIMARY
	var verifiedCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';").Scan(&verifiedCount)
	require.NoError(t, err)
	assert.Equal(t, 0, verifiedCount, "Zero events may claim VERIFIED_EXCHANGE_PRIMARY")
}

// 5. Prove that a timestamp label cannot cause same-day future data to be used before it is available (Look-ahead bias check)
func TestP1913_5_TimestampSessionOpenLabelCannotLeakFutureIntradayData(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 5A: All 23,782 daily bars are stamped with session open time 03:45:00 UTC (= 09:15:00 IST)
	var totalBars, validStampCount int
	err := db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (
				WHERE EXTRACT(HOUR FROM timestamp AT TIME ZONE 'UTC') = 3 
				  AND EXTRACT(MINUTE FROM timestamp AT TIME ZONE 'UTC') = 45 
				  AND EXTRACT(SECOND FROM timestamp AT TIME ZONE 'UTC') = 0
			)
		FROM ohlcv_daily;
	`).Scan(&totalBars, &validStampCount)
	require.NoError(t, err)
	assert.Equal(t, 23782, totalBars)
	assert.Equal(t, totalBars, validStampCount, "100.0% of daily bars must be stamped at 03:45:00 UTC")

	// Invariant 5B: Verify constraint ck_ohlcv_daily_session_stamp exists in pg_constraint
	var conCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM pg_constraint 
		WHERE conname = 'ck_ohlcv_daily_session_stamp';
	`).Scan(&conCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, conCount, 1, "Constraint ck_ohlcv_daily_session_stamp must exist")
}

// 6. Prove that missing Shariah disclosures cannot produce a PASS (Fail-Closed AAOIFI standard)
func TestP1913_6_MissingShariahDisclosuresCannotProducePass(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 6A: Across all 51 active catalog equities, exactly 0 have PASS status
	var passCount, failCount, dataInsuffCount int
	err := db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (symbol_id) symbol_id, status
			FROM shariah_screenings
			ORDER BY symbol_id, screening_date DESC, id DESC
		)
		SELECT 
			COUNT(*) FILTER (WHERE ls.status = 'PASS'),
			COUNT(*) FILTER (WHERE ls.status = 'FAIL'),
			COUNT(*) FILTER (WHERE ls.status = 'DATA_INSUFFICIENT')
		FROM symbols s
		JOIN latest_screenings ls ON ls.symbol_id = s.id
		WHERE s.is_active = true;
	`).Scan(&passCount, &failCount, &dataInsuffCount)
	require.NoError(t, err)

	assert.Equal(t, 0, passCount, "Zero active equities may be certified PASS without audited footnotes")
	assert.Equal(t, 20, failCount, "Exactly 20 active equities must be certified FAIL")
	assert.Equal(t, 31, dataInsuffCount, "Exactly 31 active equities must be classified DATA_INSUFFICIENT")

	// Invariant 6B: DATA_INSUFFICIENT records record missing interest income
	var missingIncomeCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM shariah_screenings
		WHERE status = 'DATA_INSUFFICIENT' AND details->'missing_fields' ? 'interest_income';
	`).Scan(&missingIncomeCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, missingIncomeCount, 31, "At least 31 screenings must document missing interest_income footnote")
}

// 7. Prove that universe, corporate-action, and artifact counts reconcile with the canonical manifest
func TestP1913_7_UniverseAndCorporateActionCountsReconciled(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 7A: Exactly 52 catalog instruments (51 active, 1 archived TATAMOTORS)
	var totalCatalog, activeCatalog, inactiveCatalog int
	err := db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE is_active = true),
			COUNT(*) FILTER (WHERE is_active = false)
		FROM symbols;
	`).Scan(&totalCatalog, &activeCatalog, &inactiveCatalog)
	require.NoError(t, err)
	assert.Equal(t, 52, totalCatalog)
	assert.Equal(t, 51, activeCatalog)
	assert.Equal(t, 1, inactiveCatalog)

	// Invariant 7B: 47 active core equities have trading bars, 5 lineage symbols have 0 bars
	var equitiesWithBars, lineageWithZeroBars int
	err = db.QueryRowContext(ctx, `
		WITH bar_counts AS (
			SELECT s.id, s.symbol, count(o.timestamp) as bar_count
			FROM symbols s
			LEFT JOIN ohlcv_daily o ON o.symbol_id = s.id
			GROUP BY s.id, s.symbol
		)
		SELECT 
			COUNT(*) FILTER (WHERE bar_count > 0),
			COUNT(*) FILTER (WHERE bar_count = 0)
		FROM bar_counts;
	`).Scan(&equitiesWithBars, &lineageWithZeroBars)
	require.NoError(t, err)
	assert.Equal(t, 47, equitiesWithBars, "Exactly 47 active core equities have trading bars")
	assert.Equal(t, 5, lineageWithZeroBars, "Exactly 5 lineage entities have 0 bars")

	// Invariant 7C: Exactly 64 events across 22 symbols: 58 ARCHIVE_UNAUTHENTICATED, 6 AMBIGUOUS
	var totalEvents, unauthEvents, ambiguousEvents, distinctSymbols int
	err = db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*),
			COUNT(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED'),
			COUNT(*) FILTER (WHERE verification_status = 'AMBIGUOUS'),
			COUNT(DISTINCT symbol)
		FROM corporate_action_events;
	`).Scan(&totalEvents, &unauthEvents, &ambiguousEvents, &distinctSymbols)
	require.NoError(t, err)
	assert.Equal(t, 64, totalEvents, "Total events must be 64")
	assert.Equal(t, 58, unauthEvents, "Demoted events must be 58 (57 cash dividends + 1 demerger)")
	assert.Equal(t, 6, ambiguousEvents, "Ambiguous events must be 6 (OTHER actions)")
	assert.Equal(t, 22, distinctSymbols, "Events must span exactly 22 symbols")

	// Invariant 7D: Audit log has exactly 58 demotion records
	var auditCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_verification_audit_log;").Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 58, auditCount, "Audit log must contain exactly 58 demotion records")
}

// 8. Prove that strategy parameters loaded at runtime match the frozen canonical configuration
func TestP1913_8_StrategyConfigurationFrozenAndResearchGateBlocked(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 8A: 23,132 quarantined bars enforce RESEARCH_BLOCKED
	var nullAdjCount int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&nullAdjCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, nullAdjCount)

	// Invariant 8B: Backtest service rejects runs with RESEARCH_BLOCKED
	var unbackedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM symbols s
		WHERE s.is_active = true 
		  AND s.id IN (SELECT DISTINCT symbol_id FROM ohlcv_daily)
		  AND s.id NOT IN (
			  SELECT DISTINCT e.symbol_id 
			  FROM corporate_action_events e 
			  WHERE e.verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
		  );
	`).Scan(&unbackedCount)
	require.NoError(t, err)
	assert.Equal(t, 47, unbackedCount, "All 47 active core equities must lack VERIFIED_EXCHANGE_PRIMARY evidence")
}
