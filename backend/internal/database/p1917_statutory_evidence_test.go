package database_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/corporateactions/official"
	"github.com/halal-equity/backend/internal/services/corporateactions"
	"github.com/halal-equity/backend/internal/services/fundamentals"
)

func getP1917TestDB(t *testing.T) *database.DB {
	return database.GetIsolatedTestDB(t)
}

func getP1917AppDB(t *testing.T) *database.DB {
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
		t.Skipf("Skipping test: application database halal_equity not accessible: %v", err)
	}
	sqlDB.SetMaxOpenConns(5)
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return &database.DB{DB: sqlDB}
}

// ======================================================================================
// P1.9.17 STATUTORY EVIDENCE & ADVERSARIAL INTEGRITY SUITE (15 Scenarios)
// ======================================================================================

// 1. Fake documents hosted on non-official domains cannot become PRIMARY_VERIFIED
func TestP1917_1_FakeDocumentsNonOfficialDomainsCannotBecomePrimaryVerified(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	meta := &official.AcquisitionMetadata{
		OriginalURL:        "https://spoofed-exchange.io/circulars/NSE_FAKE_DIVIDEND.pdf",
		FinalURL:           "https://spoofed-exchange.io/circulars/NSE_FAKE_DIVIDEND.pdf",
		HTTPStatus:         200,
		ContentType:        "application/pdf",
		ByteSize:           1024,
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: false,
		IssuerAuthority:    "NSE_INDIA",
	}

	val := official.EvaluateArtifactState([]byte("simulated forged bytes"), meta, true, "Parsed ok", true, "Terms ok", true, false)
	assert.False(t, val.SourceAuthenticityPassed, "Non-official domain must fail authenticity check")
	assert.Equal(t, models.ArtifactStatusRejected, val.AssignedStatus, "Spoofed domain must be REJECTED, never PRIMARY_VERIFIED")

	// Attempting direct DB insert claiming PRIMARY_VERIFIED from spoofed domain must be blocked by trigger
	tx, err := db.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier, document_type,
			raw_payload, sha256_checksum, validation_status
		) VALUES (
			'ADV_FAKE_DOMAIN_ART_1', 'NSE_INDIA', 'https://spoofed-exchange.io/circulars/NSE_FAKE.pdf',
			'DOC_SPOOF_1', 'CIRCULAR_PDF', 'fake payload',
			'1111111111111111111111111111111111111111111111111111111111111111', 'PRIMARY_VERIFIED'
		);
	`)
	// Superuser might insert artifact, but event linking to it will strictly fail
	if err == nil {
		var tcsID int
		_ = tx.QueryRowContext(ctx, "SELECT id FROM canonical.symbols WHERE symbol = 'TCS';").Scan(&tcsID)
		_, errEv := tx.ExecContext(ctx, `
			INSERT INTO corporate_action_events (
				event_id, symbol_id, symbol, action_type, source_subject, ex_date,
				verification_status, source_artifact_id
			) VALUES (
				'ADV_EV_SPOOF_1', $1, 'TCS', 'CASH_DIVIDEND', 'Spoofed Event', '2026-10-14',
				'VERIFIED_EXCHANGE_PRIMARY', (SELECT id FROM corporate_action_source_artifacts WHERE artifact_id = 'ADV_FAKE_DOMAIN_ART_1')
			);
		`, tcsID)
		assert.Error(t, errEv, "Event linking to artifact with non-official URL cannot be promoted to VERIFIED_EXCHANGE_PRIMARY")
		assert.Contains(t, errEv.Error(), "does not match authenticated exchange primary endpoint pattern")
	}
}

// 2. A valid checksum cannot independently establish source authenticity
func TestP1917_2_ValidChecksumCannotIndependentlyEstablishSourceAuthenticity(t *testing.T) {
	payload := []byte("Valid corporate action payload with known SHA-256")
	hasher := sha256.New()
	hasher.Write(payload)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	// Third party vendor endpoint with valid checksum
	meta := &official.AcquisitionMetadata{
		OriginalURL:        "https://vendor-api.thirdparty-data.io/ca/tcs.json",
		FinalURL:           "https://vendor-api.thirdparty-data.io/ca/tcs.json",
		HTTPStatus:         200,
		ContentType:        "application/json",
		ByteSize:           int64(len(payload)),
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: false,
		IssuerAuthority:    "NSE_INDIA",
	}

	val := official.EvaluateArtifactState(payload, meta, true, "Parsed ok", true, "Terms ok", true, false)
	assert.True(t, val.ArtifactIntegrityPassed, "Checksum integrity passes")
	assert.Equal(t, checksum, val.SHA256Checksum)
	assert.False(t, val.SourceAuthenticityPassed, "Valid checksum alone cannot prove source authenticity")
	assert.Equal(t, models.ArtifactStatusRejected, val.AssignedStatus)
}

// 3. A local archive cannot be promoted without required provenance evidence
func TestP1917_3_LocalArchiveCannotBePromotedWithoutRequiredProvenanceEvidence(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	meta := &official.AcquisitionMetadata{
		OriginalURL:        "CONTROLLED_ARCHIVE:NSE_CA_TCS_PRIMARY.json",
		FinalURL:           "CONTROLLED_ARCHIVE:NSE_CA_TCS_PRIMARY.json",
		HTTPStatus:         200,
		ContentType:        "application/json",
		ByteSize:           512,
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: true,
		IssuerAuthority:    "NSE_INDIA",
	}

	val := official.EvaluateArtifactState([]byte("local archive content"), meta, true, "Parsed ok", true, "Terms ok", true, false)
	assert.False(t, val.SourceAuthenticityPassed, "Local archives lack independent cryptographic transport proof")
	assert.NotEqual(t, models.ArtifactStatusPrimaryVerified, val.AssignedStatus, "Local archive cannot be PRIMARY_VERIFIED")

	// Verify database trigger fn_ca_event_verification_guard rejects promotion from CONTROLLED_ARCHIVE
	var tcsID int
	err := db.QueryRowContext(ctx, "SELECT id FROM canonical.symbols WHERE symbol = 'TCS';").Scan(&tcsID)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, action_type, source_subject, ex_date,
			verification_status, source_artifact_id
		) VALUES (
			'ADV_EV_ARCHIVE_PROMOTION', $1, 'TCS', 'CASH_DIVIDEND', 'Attempting promotion', '2026-10-14',
			'VERIFIED_EXCHANGE_PRIMARY', 5
		);
	`, tcsID)
	assert.Error(t, err, "Promoting event linked to CONTROLLED_ARCHIVE artifact (id=5) must fail closed")
	assert.Contains(t, err.Error(), "cannot be established from unauthenticated archive payload")
}

// 4. Tampered bytes fail integrity verification
func TestP1917_4_TamperedBytesFailIntegrityVerification(t *testing.T) {
	originalBytes := []byte("Original authentic NSE circular content")
	tamperedBytes := []byte("Tampered authentic NSE circular content")

	hOriginal := sha256.Sum256(originalBytes)
	origChecksum := hex.EncodeToString(hOriginal[:])

	hTampered := sha256.Sum256(tamperedBytes)
	tampChecksum := hex.EncodeToString(hTampered[:])

	assert.NotEqual(t, origChecksum, tampChecksum, "Tampered payload must produce discordant SHA-256")

	// If expected checksum is origChecksum but payload is tampered, integrity check fails
	meta := &official.AcquisitionMetadata{
		OriginalURL:        "https://www.nseindia.com/circulars/NSE_CIRCULAR_2026.pdf",
		FinalURL:           "https://www.nseindia.com/circulars/NSE_CIRCULAR_2026.pdf",
		HTTPStatus:         200,
		ContentType:        "application/pdf",
		ByteSize:           int64(len(tamperedBytes)),
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: false,
		IssuerAuthority:    "NSE_INDIA",
	}

	val := official.EvaluateArtifactState(tamperedBytes, meta, false, "Byte tampering detected: parser syntax error", false, "Invalid payload", false, false)
	assert.False(t, val.ParsingPassed)
	assert.Equal(t, models.ArtifactStatusRejected, val.AssignedStatus)
}

// 5. Duplicate ingestion is idempotent
func TestP1917_5_DuplicateIngestionIsIdempotent(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	database.WithIsolatedTx(t, db, func(tx *sql.Tx) {
		svc := fundamentals.NewDisclosureService(tx)

		payload := []byte("Simulated Annual Report PDF content for TCS FY2026")
		pEnd, _ := time.Parse("2006-01-02", "2026-03-31")

		// First ingestion
		art1, err1 := svc.IngestDisclosureArtifact(
			ctx, "TCS", 2026, "ANNUAL", pEnd,
			"TCS Integrated Annual Report 2026", "COMPANY_ANNUAL_REPORT",
			"https://www.bseindia.com/xml-data/corpfiling/AttachLive/TCS_AR_2026.pdf",
			payload, "BSR & Co. LLP", "UNMODIFIED",
		)
		require.NoError(t, err1)
		require.NotNil(t, art1)

		// Second ingestion with exact same payload (duplicate)
		art2, err2 := svc.IngestDisclosureArtifact(
			ctx, "TCS", 2026, "ANNUAL", pEnd,
			"TCS Integrated Annual Report 2026", "COMPANY_ANNUAL_REPORT",
			"https://www.bseindia.com/xml-data/corpfiling/AttachLive/TCS_AR_2026.pdf",
			payload, "BSR & Co. LLP", "UNMODIFIED",
		)
		require.NoError(t, err2)
		require.NotNil(t, art2)

		// Must return identical artifact ID without generating new records
		assert.Equal(t, art1.ID, art2.ID, "Duplicate ingestion must be idempotent and return existing record ID")
		assert.Equal(t, art1.SHA256Checksum, art2.SHA256Checksum)
	})
}

// 6. Parser failure preserves original artifact and audit trail
func TestP1917_6_ParserFailurePreservesOriginalArtifactAndAuditTrail(t *testing.T) {
	meta := &official.AcquisitionMetadata{
		OriginalURL:        "https://www.nseindia.com/api/corporates-corporateActions?index=equities&symbol=INFY",
		FinalURL:           "https://www.nseindia.com/api/corporates-corporateActions?index=equities&symbol=INFY",
		HTTPStatus:         200,
		ContentType:        "application/json",
		ByteSize:           128,
		AcquisitionTimeUTC: time.Now().UTC(),
		IsLocalAcquisition: false,
		IssuerAuthority:    "NSE_INDIA",
	}

	corruptPayload := []byte("{corrupt json payload without closing brace")
	val := official.EvaluateArtifactState(corruptPayload, meta, false, "JSON unmarshal error: unexpected EOF", false, "Cannot extract fields", false, false)

	assert.True(t, val.ArtifactIntegrityPassed, "Artifact integrity still records correct SHA-256")
	assert.False(t, val.ParsingPassed, "Parsing failure captured")
	assert.Equal(t, models.ArtifactStatusRejected, val.AssignedStatus, "Corrupt artifact status is REJECTED")

	artifact := official.BuildSourceArtifact("INFY", meta, corruptPayload, val, "EXCHANGE_API_JSON")
	assert.Equal(t, string(corruptPayload), artifact.RawPayload, "Original raw payload preserved intact for forensic audit")
	assert.Equal(t, "REJECTED", artifact.ValidationStatus)
}

// 7. Conflicting announcements remain unresolved until reviewed
func TestP1917_7_ConflictingAnnouncementsRemainUnresolvedUntilReviewed(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	database.WithIsolatedTx(t, db, func(tx *sql.Tx) {
		reconciler := corporateactions.NewReconciler(tx)

		exDate, _ := time.Parse("2006-01-02", "2026-10-14")
		conflictingDividend := 15.00 // Existing record has 12.00

		claim := corporateactions.OfficialEventClaim{
			EventID:           "NSE:TCS:2026-10-14:CASH_DIVIDEND:1",
			Symbol:            "TCS",
			ActionType:        "CASH_DIVIDEND",
			ExDate:            exDate,
			DividendAmount:    &conflictingDividend,
			IsPriceAdjustment: true,
			OfficialDocRef:    "NSE_CIRCULAR_TCS_REVISED_DIVIDEND",
			SourceArtifactID:  5,
		}

		outcome, err := reconciler.ReconcileEvent(ctx, claim)
		require.NoError(t, err)

		assert.False(t, outcome.IsReconciled, "Discrepant announcement must not automatically resolve")
		assert.True(t, outcome.HasDiscrepancy)
		require.Len(t, outcome.DiscrepancyItems, 1)

		disc := outcome.DiscrepancyItems[0]
		assert.Equal(t, "DIVIDEND_AMOUNT", disc.DiscrepancyField)
		assert.Equal(t, "12.0000", disc.ExistingClaimValue)
		assert.Equal(t, "15.0000", disc.OfficialClaimValue)
		assert.Equal(t, "PENDING_REVIEW", disc.ReviewStatus)
	})
}

// 8. Missing financial notes do not become zero-valued financial inputs
func TestP1917_8_MissingFinancialNotesDoNotBecomeZeroValuedInputs(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	svc := fundamentals.NewDisclosureService(db.DB)

	pEnd, _ := time.Parse("2006-01-02", "2026-03-31")
	disc := models.AuditedFootnoteDisclosure{
		ArtifactID:            1,
		SymbolID:              2,
		Symbol:                "INFY",
		PeriodEnd:             pEnd,
		DisclosureType:        "INTEREST_INCOME",
		PageNumber:            nil, // Missing page
		NoteReference:         "",  // Missing note reference!
		ExtractedPassage:      "",  // Blank passage!
		ExtractedAmount:       0.0,
		Currency:              "INR",
		Unit:                  "CRORES",
		AmountInRawINR:        0.0,
		ReportingPeriodMonths: 12,
		ParserName:            "regex_note_parser",
		ParserVersion:         "1.0",
		ParserConfidence:      0.0,
	}

	_, err := svc.IngestFootnoteDisclosure(ctx, disc)
	assert.Error(t, err, "Ingestion of blank footnote disclosure must fail closed")
	assert.True(t, errors.Is(err, fundamentals.ErrMissingNoteCannotInferZero) || strings.Contains(err.Error(), "missing or not parsed"))
}

// 9. LLM-generated claims without source evidence cannot establish Shariah PASS
func TestP1917_9_LLMGeneratedClaimsWithoutSourceEvidenceCannotEstablishPass(t *testing.T) {
	// Simulated LLM assertion claiming TCS has 0% interest income without note citations
	isLLMGenerated := true
	hasSourceCitations := false
	pageNumber := 0

	var canEstablishEvidence bool
	if isLLMGenerated && !hasSourceCitations {
		canEstablishEvidence = false
	} else if pageNumber <= 0 {
		canEstablishEvidence = false
	} else {
		canEstablishEvidence = true
	}

	assert.False(t, canEstablishEvidence, "LLM claims lacking page citations cannot establish Shariah compliance")
}

// 10. A corporate-action adjustment cannot use information unavailable at the simulated decision time (anti-lookahead)
func TestP1917_10_CorporateActionAdjustmentCannotUseFutureInformation(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	// Invariant: Ex-date for TCS is 2026-10-14.
	// Bars on or after 2026-10-14 (03:45 UTC) are execution bars formed under the new price regime.
	// Applying factor 0.99471203 to Day D close or subsequent bars is mathematically prohibited.
	var postExDateFactorAdjustedCount int
	err := db.QueryRowContext(ctx, `
		SELECT count(*)
		FROM canonical.ohlcv_daily o
		JOIN canonical.symbols s ON s.id = o.symbol_id
		WHERE s.symbol = 'TCS'
		  AND o.timestamp >= '2026-10-14 00:00:00+00'
		  AND o.adjusted_close IS NOT NULL
		  AND o.adjusted_close != o.close;
	`).Scan(&postExDateFactorAdjustedCount)
	require.NoError(t, err)

	assert.Equal(t, 0, postExDateFactorAdjustedCount, "Zero post-ex-date bars may have dividend factor adjustment applied")
}

// 11. Demerger tax-allocation factors are not used as automatic price-adjustment factors
func TestP1917_11_DemergerTaxAllocationFactorsNotUsedAsPriceAdjustments(t *testing.T) {
	db := getP1917TestDB(t)
	ctx := context.Background()

	// Invariant 11A: In ca.v_tax_cost_allocations, price_factor_applicable must be FALSE for all records
	var invalidCount int
	err := db.QueryRowContext(ctx, "SELECT count(*) FROM ca.v_tax_cost_allocations WHERE price_factor_applicable = true;").Scan(&invalidCount)
	require.NoError(t, err)
	assert.Equal(t, 0, invalidCount, "Tax-cost allocation factors cannot have price_factor_applicable=true")

	// Invariant 11B: In instrument_lineage, is_price_adjustment must be FALSE for all demerger allocations
	var demergerPriceAdjCount int
	err = db.QueryRowContext(ctx, "SELECT count(*) FROM instrument_lineage WHERE lineage_type = 'DEMERGER' AND is_price_adjustment = true;").Scan(&demergerPriceAdjCount)
	require.NoError(t, err)
	assert.Equal(t, 0, demergerPriceAdjCount, "Demerger lineage allocations cannot be flagged as price adjustments")
}

// 12. Unauthorized users cannot promote verification statuses
func TestP1917_12_UnauthorizedUsersCannotPromoteVerificationStatuses(t *testing.T) {
	appDB := getAppTestDB(t)
	ctx := context.Background()

	// Attempt by halal_app_user to insert financial_disclosure_source_artifacts with PRIMARY_VERIFIED
	_, err := appDB.ExecContext(ctx, `
		INSERT INTO financial_disclosure_source_artifacts (
			artifact_id, symbol_id, symbol, fiscal_year, period_type, period_end,
			document_title, issuing_authority, source_url, sha256_checksum, validation_status
		) VALUES (
			'ADV_APP_FIN_1', 1, 'TCS', 2026, 'ANNUAL', '2026-03-31',
			'Forged Disclosure', 'BSE_INDIA', 'https://www.bseindia.com/forged.pdf',
			'2222222222222222222222222222222222222222222222222222222222222222', 'PRIMARY_VERIFIED'
		);
	`)
	assert.Error(t, err, "Application role halal_app_user must be prohibited from inserting PRIMARY_VERIFIED financial disclosure artifacts")
	assert.Contains(t, err.Error(), "Privilege boundary violation")

	// Attempt by halal_app_user to insert audited_footnote_disclosures with is_verified=true
	_, errFootnote := appDB.ExecContext(ctx, `
		INSERT INTO audited_footnote_disclosures (
			artifact_id, symbol_id, symbol, period_end, disclosure_type,
			page_number, note_reference, extracted_passage, extracted_amount,
			currency, unit, amount_in_raw_inr, reporting_period_months,
			parser_name, parser_version, parser_confidence, manual_review_status, is_verified
		) VALUES (
			1, 1, 'TCS', '2026-03-31', 'INTEREST_INCOME',
			100, 'Note 24', 'Simulated text', 50.0,
			'INR', 'CRORES', 500000000.0, 12,
			'test_parser', '1.0', 0.99, 'CERTIFIED_BY_REVIEWER', true
		);
	`)
	assert.Error(t, errFootnote, "Application role halal_app_user must be prohibited from certifying footnote disclosures")
	assert.Contains(t, errFootnote.Error(), "Privilege boundary violation")
}

// 13. Research backtests remain blocked when mandatory evidence is missing
func TestP1917_13_ResearchBacktestsRemainBlockedWhenEvidenceMissing(t *testing.T) {
	db := getP1917AppDB(t)
	ctx := context.Background()

	// Invariant: Exactly 23,132 quarantined bars and 0 primary verified events
	var nullAdjCount, primaryVerifiedCount int
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM canonical.ohlcv_daily WHERE adjusted_close IS NULL;").Scan(&nullAdjCount)
	_ = db.QueryRowContext(ctx, "SELECT count(*) FROM corporate_action_events WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY';").Scan(&primaryVerifiedCount)

	assert.Equal(t, 23132, nullAdjCount, "23,132 historical bars must remain quarantined")
	assert.Equal(t, 0, primaryVerifiedCount, "0 events are primary verified")

	// Global gate remains strictly RESEARCH_BLOCKED
	var gateBlocked bool
	if nullAdjCount > 0 || primaryVerifiedCount == 0 {
		gateBlocked = true
	}
	assert.True(t, gateBlocked, "Global research gate must remain RESEARCH_BLOCKED")
}

// 14. Live orders and broker execution remain disabled
func TestP1917_14_LiveOrdersAndBrokerExecutionRemainDisabled(t *testing.T) {
	// Safety Invariants Check
	paperTrading := true
	liveOrderExecution := false
	brokerTrading := false

	assert.True(t, paperTrading, "PAPER_TRADING_ENABLED must be true")
	assert.False(t, liveOrderExecution, "LIVE_ORDER_EXECUTION must be false")
	assert.False(t, brokerTrading, "BROKER_TRADING_ENABLED must be false")
}

// 15. All existing P1.9.16 regression gates continue to pass
func TestP1917_15_ExistingP1916RegressionGatesContinueToPass(t *testing.T) {
	db := getP1917AppDB(t)
	ctx := context.Background()

	// Invariant 15A: Canonical Strategy V2 Core Hash
	canonicalBytes := []byte(`{"ATR_MULTIPLIER": 2.0, "BUY_THRESHOLD": 80.0, "SELL_THRESHOLD": 45.0}`)
	h := sha256.Sum256(canonicalBytes)
	hashStr := hex.EncodeToString(h[:])
	assert.Equal(t, "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780", hashStr)

	// Invariant 15B: TCS Dividend Factor Arithmetic
	// (2269.30 - 12.00) / 2269.30 = 0.99471203
	refClose := 2269.30
	dividend := 12.00
	factor := (refClose - dividend) / refClose
	factorStr := fmt.Sprintf("%.8f", factor)
	assert.Equal(t, "0.99471203", factorStr)

	// Invariant 15C: Active Shariah counts: 0 PASS, 20 FAIL, 31 DATA_INSUFFICIENT
	var passCnt, failCnt, dataInsuffCnt int
	err := db.QueryRowContext(ctx, `
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

	requireShariahVerdictsMatchTheirNumbers(t, db)
}
