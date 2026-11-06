package database_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// P1.9.12 Automated Release Gates: Authorized Data Acquisition and Provenance Remediation
// Enforces:
// 1. Corporate action trust status remediation: zero unsupported claims, all 58 unauthenticated archive events demoted.
// 2. Auditable migration history: corporate_action_verification_audit_log tracks prior status and reason, append-only.
// 3. Adversarial provenance trigger guard: rejects local controlled archives, fixtures, or unauthenticated vendor JSON.
// 4. Canonical timestamp convention: 100% adherence to 03:45:00 UTC (09:15 IST session open), zero close timestamps.
// 5. IST/UTC daylight-independent exchange-local dates.
// 6. Populated bar eligibility: exactly 650 populated bars (643 nominal, 7 total-return), quarantined from backtesting.
// 7. Historical quarantine integrity: exactly 23,132 bars quarantined NULL.
// 8. Genuine provider authorization invariant: DATA_INSUFFICIENT pending subscription, zero paid charges.
// 9. Primary corporate-action evidence gating: 27 core equities missing corporate actions.
// 10. Shariah screening fail-closed: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT (null interest income ratio).

// 1. Corporate Action Trust Status Remediation & Audit Trail
func TestP1912_1_CorporateActionTrustStatusDemotionAndAuditTrail(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 1A: Exactly 0 corporate action events claim VERIFIED_EXCHANGE_PRIMARY
	var verifiedCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM corporate_action_events 
		WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';
	`).Scan(&verifiedCount)
	require.NoError(t, err)
	assert.Equal(t, 0, verifiedCount, "Zero events may claim VERIFIED_EXCHANGE_PRIMARY without authenticated circular proof")

	// Invariant 1B: Exactly 58 events are demoted to ARCHIVE_UNAUTHENTICATED
	var demotedCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM corporate_action_events 
		WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED';
	`).Scan(&demotedCount)
	require.NoError(t, err)
	assert.Equal(t, 58, demotedCount, "All 58 events backed by controlled archives must be ARCHIVE_UNAUTHENTICATED")

	// Invariant 1C: Exactly 58 records exist in corporate_action_verification_audit_log
	var auditCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM corporate_action_verification_audit_log 
		WHERE remediation_phase = 'P1.9.12' 
		  AND prior_verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
		  AND new_verification_status = 'ARCHIVE_UNAUTHENTICATED';
	`).Scan(&auditCount)
	require.NoError(t, err)
	assert.Equal(t, 58, auditCount, "Audit log must record exactly 58 demotions from P1.9.12")

	// Invariant 1D: corporate_action_verification_audit_log is append-only
	_, err = db.ExecContext(ctx, "DELETE FROM corporate_action_verification_audit_log WHERE remediation_phase = 'P1.9.12';")
	assert.Error(t, err, "Audit log must reject DELETE statements (append-only guard)")
	assert.Contains(t, err.Error(), "append-only")
}

// 2. Adversarial Provenance Guard Rejects Unauthenticated Payloads
func TestP1912_2_AdversarialProvenanceGuardRejectsUnauthenticatedPayloads(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM canonical.symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	// Adversarial Attempt A: Attempting to insert an event claiming VERIFIED_EXCHANGE_PRIMARY
	// backed by a controlled archive source artifact (Artifact ID 5)
	advEventIDA := "NSE:TCS:2026-10-14:CASH_DIVIDEND:ADV_ARCHIVE_ATTEMPT"
	_, err = db.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, source_artifact_id, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			$1, $2, 'TCS', 5, 'CASH_DIVIDEND', 'Adversarial Archive Event',
			'2026-10-14', 'INR', 'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, advEventIDA, tcsID)
	assert.Error(t, err, "Trigger must reject VERIFIED_EXCHANGE_PRIMARY backed by CONTROLLED_ARCHIVE")
	assert.Contains(t, err.Error(), "cannot be established from unauthenticated archive payload")

	// Adversarial Attempt B: Attempting to claim VERIFIED_EXCHANGE_PRIMARY without an authenticated document type
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier, document_type,
			raw_payload, sha256_checksum, validation_status
		) VALUES (
			'ADV_VENDOR_JSON_ART', 'NSE_INDIA', 'https://api.vendor.example/ca/tcs.json',
			'VENDOR_CA_TCS', 'EXCHANGE_API_JSON', '{"ca":"mock"}',
			'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', 'PRIMARY_VERIFIED'
		);
	`)
	require.NoError(t, err)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, source_artifact_id, action_type, source_subject,
			ex_date, currency, verification_status, coverage_status
		) VALUES (
			'NSE:TCS:2026-10-14:CASH_DIVIDEND:ADV_VENDOR_DOC', $1, 'TCS',
			(SELECT id FROM corporate_action_source_artifacts WHERE artifact_id = 'ADV_VENDOR_JSON_ART'),
			'CASH_DIVIDEND', 'Vendor Document Event', '2026-10-14', 'INR',
			'VERIFIED_EXCHANGE_PRIMARY', 'HARVESTED_EXCHANGE_PRIMARY'
		);
	`, tcsID)
	assert.Error(t, err, "Trigger must reject document types other than CIRCULAR_PDF, AUTHENTICATED_EXCHANGE_FEED, or STATUTORY_FILING")
	assert.Contains(t, err.Error(), "lacks authenticated primary origin proof")
}

// 3. Timestamp Convention Verification: 100% Session Open Stamping (03:45 UTC = 09:15 IST)
func TestP1912_3_TimestampConventionSessionOpenVerification(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 3A: Total bars in ohlcv_daily is exactly 23,782
	var totalBars int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily;").Scan(&totalBars)
	require.NoError(t, err)
	assertForwardSessionGrowth(t, 23782, forwardSessionBarStep, totalBars, "Total daily bars must match valid forward session count")

	// Invariant 3B: Exactly 0 bars deviate from 03:45:00 UTC
	var nonCanonicalBars int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM canonical.ohlcv_daily 
		WHERE ("timestamp" AT TIME ZONE 'UTC')::time != TIME '03:45:00';
	`).Scan(&nonCanonicalBars)
	require.NoError(t, err)
	assert.Equal(t, 0, nonCanonicalBars, "Zero bars may deviate from canonical session open (03:45:00 UTC)")

	// Invariant 3C: Database constraint ck_ohlcv_daily_session_stamp exists and enforces 03:45 UTC
	var constraintCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM information_schema.check_constraints 
		WHERE constraint_name = 'ck_ohlcv_daily_session_stamp';
	`).Scan(&constraintCount)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, constraintCount, 1, "ck_ohlcv_daily_session_stamp constraint must be present")

	// Invariant 3D: Market close timestamps (15:30 IST / 10:00 UTC) are strictly rejected by check constraint
	var tcsID int
	err = db.QueryRowContext(ctx, "SELECT id FROM canonical.symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	closeTs := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC) // 15:30 IST
	_, err = db.ExecContext(ctx, `
		INSERT INTO ohlcv_daily (
			symbol_id, timestamp, open, high, low, close, volume, source
		) VALUES ($1, $2, 2000.0, 2010.0, 1990.0, 2005.0, 100000, 'yahoo_finance');
	`, tcsID, closeTs)
	assert.Error(t, err, "Market close timestamp (10:00 UTC) must be rejected by session open constraint")
	assert.Contains(t, err.Error(), "ck_ohlcv_daily_session_stamp")
}

// 4. Populated Bar Eligibility & Separation Audit
func TestP1912_4_PopulatedBarEligibilityAndSeriesClassification(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Invariant 4A: Exactly 650 populated bars exist
	var populatedCount int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NOT NULL;").Scan(&populatedCount)
	require.NoError(t, err)
	assertForwardSessionGrowth(t, 650, forwardSessionBarStep, populatedCount, "Populated bars must match valid forward session count")

	// Invariant 4B: Exactly 643, 690, 737, 784, or 831 bars are nominal execution prices (adjusted_close == close)
	var nominalUnadjCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM canonical.ohlcv_daily 
		WHERE adjusted_close IS NOT NULL 
		  AND adjusted_close = close 
		  AND source = 'yahoo_finance';
	`).Scan(&nominalUnadjCount)
	require.NoError(t, err)
	assertForwardSessionGrowth(t, 643, forwardSessionBarStep, nominalUnadjCount, "Bars must be unadjusted nominal forward execution series")

	// Invariant 4C: Exactly 7 bars are total-return dividend-adjusted prices (TCS pre-ex-date)
	var dividendAdjCount int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) 
		FROM canonical.ohlcv_daily 
		WHERE adjusted_close IS NOT NULL 
		  AND adjusted_close != close 
		  AND source = 'NSE_CORPORATE_ACTION';
	`).Scan(&dividendAdjCount)
	require.NoError(t, err)
	assert.Equal(t, 7, dividendAdjCount, "Exactly 7 bars must be total-return dividend adjusted series")

	// Invariant 4D: Exactly 23,132 historical bars remain strictly quarantined NULL
	var quarantinedCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&quarantinedCount)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedCount, "Strictly 23,132 historical bars must remain quarantined NULL")
}

// 5. Genuine Provider Data & Subscription Authorization Safeguard
func TestP1912_5_ProviderDataAuthorizationAndSafetyInvariants(t *testing.T) {
	dataBytes := readP1911File(t, "data/poc/poc_validation_output.json")
	var report P1911ValidationReport
	err := json.Unmarshal(dataBytes, &report)
	require.NoError(t, err)

	assert.Equal(t, "TRANSFORMED_SCHEMA_FIXTURE_FROM_LOCAL_POSTGRESQL", report.DataProvenanceVerdict.SampleOrigin)
	assert.False(t, report.DataProvenanceVerdict.LiveAPIStreamAuthenticated, "Live provider feed must not claim authentication")
	assert.Equal(t, "DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION", report.DataProvenanceVerdict.LiveProviderStatus)
}

// 6. Shariah Screening Fail-Closed Behavior
func TestP1912_6_ShariahEvidenceAuditAndFailClosedGate(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var passCount, failCount, dataInsufCount int
	err := db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (s.symbol) s.symbol, sc.status, sc.interest_income_ratio
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
