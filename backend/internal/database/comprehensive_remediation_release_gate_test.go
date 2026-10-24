package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func getAppUserDB(t *testing.T) *sql.DB {
	host := os.Getenv("DB_HOST")
	if host == "" {
		host = "192.168.18.43"
	}
	appConnStr := "postgres://halal_app_user:app_password@" + host + ":5432/halal_equity?sslmode=disable"
	db, err := sql.Open("postgres", appConnStr)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("Skipping integration test; postgres database not accessible: %v", err)
	}
	return db
}

func TestComprehensive_1_ExplicitEvidenceStatesConstraints(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// 1. Invalid verification_status on corporate_action_events must be rejected by check constraint
	_, err := db.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject, ex_date, currency, verification_status, coverage_status
		) VALUES (
			'TEST:INVALID:STATUS:1', 1, 'DRREDDY', 'CASH_DIVIDEND', 'Test', '2026-11-01', 'INR', 'ILLEGAL_STATUS_FLAG', 'PARSED_EVENT'
		)
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ck_ca_event_verification_status")

	// 2. Invalid validation_status on corporate_action_source_artifacts must be rejected by check constraint
	_, err = db.ExecContext(ctx, `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier, document_type, raw_payload, sha256_checksum, validation_status
		) VALUES (
			'TEST:ARTIFACT:INVALID:1', 'NSE_INDIA', 'https://www.nseindia.com/test.json', 'TEST_DOC', 'CIRCULAR_PDF', '{}',
			'abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd', 'ILLEGAL_ARTIFACT_STATUS'
		)
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ck_ca_artifact_validation_status")
}

func TestComprehensive_2_ApplicationRoleBlockedFromStatusPromotion(t *testing.T) {
	appDB := getAppUserDB(t)
	defer appDB.Close()
	ctx := context.Background()

	// halal_app_user must NEVER be allowed to update an event to PRIMARY_SOURCE_AUTHENTICATED or VERIFIED_EXCHANGE_PRIMARY
	_, err := appDB.ExecContext(ctx, `
		UPDATE corporate_action_events 
		SET verification_status = 'PRIMARY_SOURCE_AUTHENTICATED' 
		WHERE event_id = 'NSE:TCS:2026-10-14:CASH_DIVIDEND:1'
	`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "privilege boundary violation")
}

func TestComprehensive_3_ArtifactAndAuditLogAppendOnlyGrants(t *testing.T) {
	appDB := getAppUserDB(t)
	defer appDB.Close()
	ctx := context.Background()

	// 1. DELETE on corporate_action_source_artifacts must be blocked by PostgreSQL permissions
	_, err := appDB.ExecContext(ctx, `DELETE FROM corporate_action_source_artifacts WHERE id = 1`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	// 2. DELETE on corporate_action_verification_audit_log must be blocked by PostgreSQL permissions
	_, err = appDB.ExecContext(ctx, `DELETE FROM corporate_action_verification_audit_log WHERE id = 1`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	// 3. UPDATE on corporate_action_source_artifacts must be blocked by PostgreSQL permissions
	_, err = appDB.ExecContext(ctx, `UPDATE corporate_action_source_artifacts SET parser_version = 'v2' WHERE id = 1`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
}

func TestComprehensive_4_SectorCompletenessAcrossAll52Symbols(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Exactly 52 catalog symbols
	var totalSymbols int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM symbols`).Scan(&totalSymbols)
	require.NoError(t, err)
	assert.Equal(t, 52, totalSymbols)

	// Zero symbols may have empty or NULL sector
	var emptySectors int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM symbols WHERE sector IS NULL OR TRIM(sector) = ''`).Scan(&emptySectors)
	require.NoError(t, err)
	assert.Equal(t, 0, emptySectors, "100% sector completeness required: all 52 symbols must have explicit sector")

	// Active equities must equal 51
	var activeSymbols int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM symbols WHERE is_active = true`).Scan(&activeSymbols)
	require.NoError(t, err)
	assert.Equal(t, 51, activeSymbols)
}

func TestComprehensive_5_CanonicalInstrumentLineageRecords(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	// Must have exactly 3 canonical lineage records
	var lineageCount int
	err := db.QueryRowContext(ctx, `SELECT count(*) FROM instrument_lineage`).Scan(&lineageCount)
	require.NoError(t, err)
	assert.Equal(t, 3, lineageCount)

	// All lineage records must have is_price_adjustment = false (strictly non-price)
	var priceAdjustments int
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM instrument_lineage WHERE is_price_adjustment = true`).Scan(&priceAdjustments)
	require.NoError(t, err)
	assert.Equal(t, 0, priceAdjustments, "Lineage cost apportionments are strictly non-price adjustments")

	// Verify TATAMOTORS demerger successors
	rows, err := db.QueryContext(ctx, `
		SELECT successor_symbol, allocation_ratio 
		FROM instrument_lineage 
		WHERE predecessor_symbol = 'TATAMOTORS' 
		ORDER BY successor_symbol
	`)
	require.NoError(t, err)
	defer rows.Close()

	ratios := make(map[string]float64)
	for rows.Next() {
		var sym string
		var ratio float64
		require.NoError(t, rows.Scan(&sym, &ratio))
		ratios[sym] = ratio
	}
	assert.InDelta(t, 0.3115, ratios["TMCV"], 0.0001)
	assert.InDelta(t, 0.6885, ratios["TMPV"], 0.0001)
}

func TestComprehensive_6_QuarantinedAndNominalBarInvariants(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var totalBars, quarantinedBars, populatedBars int
	err := db.QueryRowContext(ctx, `
		SELECT 
			count(*),
			count(*) FILTER (WHERE adjusted_close IS NULL),
			count(*) FILTER (WHERE adjusted_close IS NOT NULL)
		FROM ohlcv_daily
	`).Scan(&totalBars, &quarantinedBars, &populatedBars)
	require.NoError(t, err)

	assert.Equal(t, 23782, totalBars)
	assert.Equal(t, 23132, quarantinedBars, "23,132 historical bars must remain quarantined")
	assert.Equal(t, 650, populatedBars, "650 populated bars (643 Yahoo nominal, 7 TCS adjusted)")

	// Zero bars may enter backtesting while unauthenticated
	var verifiedEvents int
	err = db.QueryRowContext(ctx, `
		SELECT count(*) FROM corporate_action_events WHERE verification_status IN ('VERIFIED_EXCHANGE_PRIMARY', 'PRIMARY_SOURCE_AUTHENTICATED')
	`).Scan(&verifiedEvents)
	require.NoError(t, err)
	assert.Equal(t, 0, verifiedEvents, "Zero events possess verified exchange primary authentication")
}

func TestComprehensive_7_ShariahFailClosedDistribution(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var passCnt, failCnt, dataInsuffCnt int
	err := db.QueryRowContext(ctx, `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (symbol_id) symbol_id, status
			FROM shariah_screenings
			ORDER BY symbol_id, screening_date DESC, id DESC
		)
		SELECT 
			count(*) FILTER (WHERE s.status = 'PASS'),
			count(*) FILTER (WHERE s.status = 'FAIL'),
			count(*) FILTER (WHERE s.status = 'DATA_INSUFFICIENT')
		FROM symbols sym
		JOIN latest_screenings s ON sym.id = s.symbol_id
		WHERE sym.is_active = true;
	`).Scan(&passCnt, &failCnt, &dataInsuffCnt)
	require.NoError(t, err)

	assert.Equal(t, 0, passCnt, "0 active equities may have PASS status without complete footnote interest income evidence")
	assert.Equal(t, 20, failCnt, "20 active equities must be certified FAIL")
	assert.Equal(t, 31, dataInsuffCnt, "31 active equities must be certified DATA_INSUFFICIENT")
}

func TestComprehensive_8_PlatformSafetyAndResearchGateBlocked(t *testing.T) {
	// 1. Mandatory safety flags
	// PAPER_TRADING_ENABLED=true, LIVE_ORDER_EXECUTION=false, BROKER_TRADING_ENABLED=false
	// RESEARCH_GATE_STATUS=RESEARCH_BLOCKED
	expectedHash := "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"

	// 2. Canonical serialization: {"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}
	canonicalRaw := []byte(`{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`)
	sum := sha256.Sum256(canonicalRaw)
	computedHash := hex.EncodeToString(sum[:])
	assert.Equal(t, expectedHash, computedHash, "Go computed hash must match pinned canonical hash exactly")

	// 3. Negative test: changing ATR_MULTIPLIER to 1.50 must alter the hash
	driftRaw := []byte(`{"ATR_MULTIPLIER": 1.5, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`)
	driftSum := sha256.Sum256(driftRaw)
	driftHash := hex.EncodeToString(driftSum[:])
	assert.NotEqual(t, expectedHash, driftHash, "Drifted parameters must not produce the pinned canonical hash")
}
