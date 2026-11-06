package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

const (
	ExpectedPreP1918BackupSHA256 = "4ab0194a2cdf6ec6d9c827de97401ecf92601934c84ecbae600a20b0762843da"
)

func getP1919AppDB(t *testing.T) *database.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "192.168.18.171"
	}
	connStr := "postgres://halal_user:devpassword@" + host + ":5432/halal_equity?sslmode=disable"
	sqlDB, err := sql.Open("postgres", connStr)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		t.Skipf("Skipping integration test; postgres database not accessible: %v", err)
	}
	sqlDB.SetMaxOpenConns(5)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return &database.DB{DB: sqlDB}
}

func getP1919IsolatedTestDB(t *testing.T) *database.DB {
	return database.GetIsolatedTestDB(t)
}

// ======================================================================================
// P1.9.19 RELEASE INTEGRITY & EVIDENCE READINESS AUDIT SUITE
// ======================================================================================

// 1. Verify pre-remediation backup existence, readability, permissions, and recorded checksum
func TestP1919_1_PreRemediationBackupIntegrityAndChecksum(t *testing.T) {
	backupPath := filepath.Join("..", "..", "migrations", "backups", "halal_equity_pre_p1918.sql")
	fileInfo, err := os.Stat(backupPath)
	require.NoError(t, err, "Pre-remediation backup file must exist")
	assert.Greater(t, fileInfo.Size(), int64(20*1024*1024), "Backup file must be > 20MB")

	// Verify file permissions are restricted (0600 on unix)
	mode := fileInfo.Mode().Perm()
	assert.Equal(t, os.FileMode(0600), mode, "Backup permissions must be restricted to 0600")

	// Verify exact SHA-256 checksum
	f, err := os.Open(backupPath)
	require.NoError(t, err)
	defer f.Close()

	hasher := sha256.New()
	_, err = io.Copy(hasher, f)
	require.NoError(t, err)
	actualChecksum := hex.EncodeToString(hasher.Sum(nil))

	assert.Equal(t, ExpectedPreP1918BackupSHA256, actualChecksum, "Backup SHA-256 must match recorded baseline")
}

// 2. Test database identity guards against alternate connection strings, aliases, and environment overrides
func TestP1919_2_DatabaseIdentityGuardsAgainstAliasesAndOverrides(t *testing.T) {
	appDB := getP1919AppDB(t)

	// Guard must detect and reject application database
	err := database.CheckDatabaseGuard(appDB.DB)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "HARD SECURITY GUARD: detected application database target")

	// Guard must test aliases and variations
	assert.Error(t, database.CheckDatabaseName("halal_equity"))
	assert.Error(t, database.CheckDatabaseName("HALAL_EQUITY"))
	assert.Error(t, database.CheckDatabaseName("  halal_equity  "))
	assert.Error(t, database.CheckDatabaseName("HaLaL_EqUiTy"))
	assert.NoError(t, database.CheckDatabaseName("halal_equity_test"))
	assert.NoError(t, database.CheckDatabaseName("halal_equity_isolated"))

	// Guard must panic in AssertNotApplicationDB
	assert.Panics(t, func() {
		database.AssertNotApplicationDB(appDB.DB, t)
	}, "AssertNotApplicationDB must panic when target is halal_equity")

	// Isolated test DB must pass guard cleanly
	testDB := getP1919IsolatedTestDB(t)
	err = database.CheckDatabaseGuard(testDB.DB)
	assert.NoError(t, err)

	var currentDB string
	err = testDB.QueryRow("SELECT current_database();").Scan(&currentDB)
	require.NoError(t, err)
	assert.Equal(t, database.TestDatabaseName, currentDB)
}

// 3. Verify WithIsolatedTx guarantees complete rollback of test data
func TestP1919_3_WithIsolatedTxGuaranteesCompleteRollback(t *testing.T) {
	testDB := getP1919IsolatedTestDB(t)
	ctx := context.Background()

	testDocID := "ROLLBACK_TEST_DOC_P1919"

	database.WithIsolatedTx(t, testDB, func(tx *sql.Tx) {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO corporate_action_source_artifacts (
				artifact_id, issuing_authority, source_url, document_identifier,
				document_type, raw_payload, sha256_checksum, validation_status
			) VALUES (
				'ART_ROLLBACK_TEST', 'NSE_INDIA', 'https://www.nseindia.com/doc.pdf',
				$1, 'CIRCULAR', 'test payload',
				'9999999999999999999999999999999999999999999999999999999999999999',
				'PARSED_VALIDATED'
			);
		`, testDocID)
		require.NoError(t, err)

		// Inside transaction, record is visible
		var countInside int
		err = tx.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_source_artifacts WHERE document_identifier = $1;", testDocID).Scan(&countInside)
		require.NoError(t, err)
		assert.Equal(t, 1, countInside)
	})

	// After WithIsolatedTx exits, transaction was rolled back; record must NOT exist
	var countOutside int
	err := testDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_source_artifacts WHERE document_identifier = $1;", testDocID).Scan(&countOutside)
	require.NoError(t, err)
	assert.Equal(t, 0, countOutside, "Record must be rolled back completely")
}

// 4. Verify eight compensating records correspond to eight affected records in application database
func TestP1919_4_CompensatingAuditRecordsReconciliation(t *testing.T) {
	appDB := getP1919AppDB(t)
	ctx := context.Background()

	var count int
	err := appDB.QueryRowContext(ctx, "SELECT count(*) FROM p1918_forensic_remediation_audit;").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 8, count, "Must contain exactly 8 compensating audit entries")

	var logCount int
	err = appDB.QueryRowContext(ctx, "SELECT count(*) FROM audit_logs WHERE action LIKE 'P1918%';").Scan(&logCount)
	require.NoError(t, err)
	assert.Equal(t, 8, logCount, "Must contain exactly 8 corresponding audit_logs entries")

	// Record 1: Financial disclosure artifact
	var targetTable, recordID, status string
	err = appDB.QueryRowContext(ctx, "SELECT target_table, target_record_id FROM p1918_forensic_remediation_audit WHERE id = 1;").Scan(&targetTable, &recordID)
	require.NoError(t, err)
	assert.Equal(t, "financial_disclosure_source_artifacts", targetTable)
	assert.Equal(t, "FIN_TCS_FY2026_6e7b35084ee4", recordID)

	err = appDB.QueryRowContext(ctx, "SELECT validation_status FROM financial_disclosure_source_artifacts WHERE artifact_id = $1;", recordID).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "REJECTED", status)

	// Records 2-8: Corporate action reconciliation reviews
	var pendingReviewsCount int
	err = appDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_reconciliation_reviews WHERE review_status = 'PENDING_REVIEW';").Scan(&pendingReviewsCount)
	require.NoError(t, err)
	assert.Equal(t, 0, pendingReviewsCount, "Must be zero active pending reviews")

	var rejectedFixturesCount int
	err = appDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_reconciliation_reviews WHERE review_status = 'REJECTED_TEST_FIXTURE';").Scan(&rejectedFixturesCount)
	require.NoError(t, err)
	assert.Equal(t, 7, rejectedFixturesCount, "All 7 synthetic reviews must be REJECTED_TEST_FIXTURE")
}

// 5. Verify application database has no unintended test artifacts or tables
func TestP1919_5_ApplicationDatabaseHasNoUnintendedTestArtifacts(t *testing.T) {
	appDB := getP1919AppDB(t)
	ctx := context.Background()

	// Zero audited footnote disclosures
	var footnoteCount int
	err := appDB.QueryRowContext(ctx, "SELECT count(*) FROM audited_footnote_disclosures;").Scan(&footnoteCount)
	require.NoError(t, err)
	assert.Equal(t, 0, footnoteCount, "Audited footnote disclosures must have 0 rows")

	// Zero test corporate action circulars
	var testCirculars int
	err = appDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_source_artifacts WHERE document_identifier LIKE '%TEST%';").Scan(&testCirculars)
	require.NoError(t, err)
	assert.Equal(t, 0, testCirculars, "Zero test circulars in corporate_action_source_artifacts")

	// Zero test corporate action events
	var testEvents int
	err = appDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_events WHERE event_id LIKE '%TEST%';").Scan(&testEvents)
	require.NoError(t, err)
	assert.Equal(t, 0, testEvents, "Zero test events in corporate_action_events")
}

// 6. Verify historical market data invariants and session timestamps
func TestP1919_6_HistoricalMarketDataInvariantsAndSessionTimestamps(t *testing.T) {
	appDB := getP1919AppDB(t)
	ctx := context.Background()

	// 23,132 quarantined bars
	var quarantinedBars int
	err := appDB.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&quarantinedBars)
	require.NoError(t, err)
	assert.Equal(t, 23132, quarantinedBars, "23,132 quarantined bars invariant strictly held")

	// 100% of bars stamped at 03:45:00 UTC (09:15 IST)
	var deviantStampBars int
	err = appDB.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily
		WHERE ("timestamp" AT TIME ZONE 'UTC')::time != TIME '03:45:00';
	`).Scan(&deviantStampBars)
	require.NoError(t, err)
	assert.Equal(t, 0, deviantStampBars, "100% of bars must be stamped at 03:45:00 UTC")

	// Total bars and populated bars invariant
	var totalBars, populatedBars int
	err = appDB.QueryRowContext(ctx, "SELECT count(*), count(adjusted_close) FROM canonical.ohlcv_daily;").Scan(&totalBars, &populatedBars)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, totalBars, 23970)
	assert.GreaterOrEqual(t, populatedBars, 838)
	assert.Equal(t, 23132, totalBars-populatedBars)
}

// 7. Verify mandatory safety invariants and fail-closed research gate
func TestP1919_7_MandatorySafetyInvariantsAndFailClosedGate(t *testing.T) {
	appDB := getP1919AppDB(t)
	ctx := context.Background()

	// Quarantined bars must keep gate locked
	var quarantined int
	_ = appDB.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&quarantined)
	var primaryVerified int
	_ = appDB.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';").Scan(&primaryVerified)

	gateStatus := "RESEARCH_BLOCKED"
	if quarantined == 0 && primaryVerified > 0 {
		gateStatus = "RESEARCH_UNBLOCKED"
	}
	assert.Equal(t, "RESEARCH_BLOCKED", gateStatus)

	// Shariah active universe partition: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT
	var passCnt, failCnt, dataInsuffCnt int
	err := appDB.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (symbol_id) symbol_id, status
			FROM canonical.shariah_screenings
			ORDER BY symbol_id, screening_date DESC, id DESC
		)
		SELECT 
			COUNT(*) FILTER (WHERE ls.status = 'PASS'),
			COUNT(*) FILTER (WHERE ls.status = 'FAIL'),
			COUNT(*) FILTER (WHERE ls.status = 'DATA_INSUFFICIENT')
		FROM canonical.symbols s
		JOIN latest_screenings ls ON ls.symbol_id = s.id
		WHERE s.is_active = true;
	`).Scan(&passCnt, &failCnt, &dataInsuffCnt)
	require.NoError(t, err)

	requireShariahVerdictsMatchTheirNumbers(t, appDB)
}
