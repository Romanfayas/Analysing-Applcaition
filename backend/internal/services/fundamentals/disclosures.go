package fundamentals

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

var (
	ErrMissingNoteCannotInferZero = errors.New("audited footnote missing or not parsed; interest income cannot be inferred as zero (fail-closed)")
	ErrLLMUnsupportedClaim        = errors.New("LLM-generated interpretation without cited page and audited note passage cannot establish Shariah PASS")
	ErrReviewerSignoffRequired    = errors.New("qualified Shariah reviewer sign-off required before certification")
)

// DBTX is the common interface satisfied by *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// DisclosureService manages audited statutory financial statements and footnote extractions.
type DisclosureService struct {
	db DBTX
}

// NewDisclosureService initializes a financial disclosures management service.
func NewDisclosureService(db DBTX) *DisclosureService {
	return &DisclosureService{db: db}
}

// IngestDisclosureArtifact stores an official annual report or statutory filing.
func (s *DisclosureService) IngestDisclosureArtifact(
	ctx context.Context,
	symbol string,
	fiscalYear int,
	periodType string,
	periodEnd time.Time,
	docTitle string,
	issuingAuthority string,
	sourceURL string,
	payload []byte,
	auditorFirm string,
	auditorOpinion string,
) (*models.FinancialDisclosureSourceArtifact, error) {
	if len(payload) == 0 {
		return nil, fmt.Errorf("payload cannot be empty")
	}

	hasher := sha256.New()
	hasher.Write(payload)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	var symbolID int
	err := s.db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = $1;", symbol).Scan(&symbolID)
	if err != nil {
		return nil, fmt.Errorf("unknown symbol %s: %w", symbol, err)
	}

	artifactID := fmt.Sprintf("FIN_%s_FY%d_%s", symbol, fiscalYear, checksum[:12])

	// Check if already registered (Idempotency)
	var existingID int
	checkErr := s.db.QueryRowContext(ctx, "SELECT id FROM financial_disclosure_source_artifacts WHERE sha256_checksum = $1;", checksum).Scan(&existingID)
	if checkErr == nil {
		// Return existing artifact idempotently
		return s.GetArtifactByID(ctx, existingID)
	}

	rawStr := string(payload)
	byteSize := int64(len(payload))
	httpStatus := 200
	contentType := "application/pdf"
	valStatus := models.ArtifactStatusAcquiredUnverified

	artifact := &models.FinancialDisclosureSourceArtifact{
		ArtifactID:         artifactID,
		SymbolID:           symbolID,
		Symbol:             symbol,
		FiscalYear:         fiscalYear,
		PeriodType:         periodType,
		PeriodEnd:          periodEnd,
		DocumentTitle:      docTitle,
		IssuingAuthority:   issuingAuthority,
		SourceURL:          sourceURL,
		HTTPStatus:         &httpStatus,
		ByteSize:           &byteSize,
		ContentType:        &contentType,
		RawPayload:         &rawStr,
		SHA256Checksum:     checksum,
		RetrievalTimestamp: time.Now().UTC(),
		AuditorFirm:        &auditorFirm,
		AuditorOpinion:     &auditorOpinion,
		ValidationStatus:   valStatus,
		ReviewStatus:       "PENDING_REVIEW",
		CreatedAt:          time.Now().UTC(),
	}

	insertQ := `
		INSERT INTO financial_disclosure_source_artifacts (
			artifact_id, symbol_id, symbol, fiscal_year, period_type, period_end,
			document_title, issuing_authority, source_url, http_status, byte_size,
			content_type, raw_payload, sha256_checksum, retrieval_timestamp,
			auditor_firm, auditor_opinion, validation_status, review_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id;
	`
	err = s.db.QueryRowContext(
		ctx, insertQ,
		artifact.ArtifactID, artifact.SymbolID, artifact.Symbol, artifact.FiscalYear, artifact.PeriodType, artifact.PeriodEnd,
		artifact.DocumentTitle, artifact.IssuingAuthority, artifact.SourceURL, artifact.HTTPStatus, artifact.ByteSize,
		artifact.ContentType, artifact.RawPayload, artifact.SHA256Checksum, artifact.RetrievalTimestamp,
		artifact.AuditorFirm, artifact.AuditorOpinion, artifact.ValidationStatus, artifact.ReviewStatus,
	).Scan(&artifact.ID)

	if err != nil {
		return nil, fmt.Errorf("failed to insert financial disclosure artifact: %w", err)
	}

	return artifact, nil
}

// IngestFootnoteDisclosure records an audited footnote extraction with citation details.
func (s *DisclosureService) IngestFootnoteDisclosure(
	ctx context.Context,
	disclosure models.AuditedFootnoteDisclosure,
) (*models.AuditedFootnoteDisclosure, error) {
	// Rule: Missing note cannot infer zero
	if disclosure.NoteReference == "" || strings.TrimSpace(disclosure.ExtractedPassage) == "" {
		return nil, ErrMissingNoteCannotInferZero
	}

	// Rule: Extracted amount cannot be negative
	if disclosure.ExtractedAmount < 0 || disclosure.AmountInRawINR < 0 {
		return nil, fmt.Errorf("extracted financial amount cannot be negative")
	}

	// Rule: Confidence check
	if disclosure.ParserConfidence < 0.80 {
		disclosure.ManualReviewStatus = "PENDING_QUALIFIED_REVIEW"
		disclosure.IsVerified = false
	}

	q := `
		INSERT INTO audited_footnote_disclosures (
			artifact_id, symbol_id, symbol, period_end, disclosure_type,
			page_number, note_reference, extracted_passage, extracted_amount,
			currency, unit, amount_in_raw_inr, reporting_period_months,
			parser_name, parser_version, parser_confidence, manual_review_status,
			shariah_methodology_version, is_verified
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id, created_at;
	`
	err := s.db.QueryRowContext(
		ctx, q,
		disclosure.ArtifactID, disclosure.SymbolID, disclosure.Symbol, disclosure.PeriodEnd, disclosure.DisclosureType,
		disclosure.PageNumber, disclosure.NoteReference, disclosure.ExtractedPassage, disclosure.ExtractedAmount,
		disclosure.Currency, disclosure.Unit, disclosure.AmountInRawINR, disclosure.ReportingPeriodMonths,
		disclosure.ParserName, disclosure.ParserVersion, disclosure.ParserConfidence, disclosure.ManualReviewStatus,
		disclosure.ShariahMethodologyVer, disclosure.IsVerified,
	).Scan(&disclosure.ID, &disclosure.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("failed to insert audited footnote disclosure: %w", err)
	}

	return &disclosure, nil
}

// GetArtifactByID retrieves an artifact by ID.
func (s *DisclosureService) GetArtifactByID(ctx context.Context, id int) (*models.FinancialDisclosureSourceArtifact, error) {
	var art models.FinancialDisclosureSourceArtifact
	q := `
		SELECT id, artifact_id, symbol_id, symbol, fiscal_year, period_type, period_end,
		       document_title, issuing_authority, source_url, http_status, byte_size,
		       content_type, sha256_checksum, retrieval_timestamp, auditor_firm,
		       auditor_opinion, validation_status, review_status, created_at
		FROM financial_disclosure_source_artifacts
		WHERE id = $1;
	`
	err := s.db.QueryRowContext(ctx, q, id).Scan(
		&art.ID, &art.ArtifactID, &art.SymbolID, &art.Symbol, &art.FiscalYear, &art.PeriodType, &art.PeriodEnd,
		&art.DocumentTitle, &art.IssuingAuthority, &art.SourceURL, &art.HTTPStatus, &art.ByteSize,
		&art.ContentType, &art.SHA256Checksum, &art.RetrievalTimestamp, &art.AuditorFirm,
		&art.AuditorOpinion, &art.ValidationStatus, &art.ReviewStatus, &art.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &art, nil
}
