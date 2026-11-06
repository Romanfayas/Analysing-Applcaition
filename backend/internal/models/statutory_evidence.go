package models

import "time"

// ==================================================
// P1.9.17 Statutory Evidence & Authentication Models
// ==================================================

// Verification state machine constants for corporate action source artifacts
const (
	ArtifactStatusAcquiredUnverified = "ACQUIRED_UNVERIFIED"
	ArtifactStatusParsedValidated    = "PARSED_VALIDATED"
	ArtifactStatusAuthPending        = "AUTHENTICITY_PENDING"
	ArtifactStatusPrimaryVerified    = "PRIMARY_VERIFIED"
	ArtifactStatusRejected           = "REJECTED"
	ArtifactStatusAmbiguous          = "AMBIGUOUS"
	ArtifactStatusSuperseded         = "SUPERSEDED"
)

// Verification state machine constants for corporate action events
const (
	EventStatusSourceMissing         = "SOURCE_MISSING"
	EventStatusArchiveUnauth         = "ARCHIVE_UNAUTHENTICATED"
	EventStatusHashVerified          = "HASH_VERIFIED"
	EventStatusVendorConfirmed       = "VENDOR_CONFIRMED"
	EventStatusAuthPending           = "AUTHENTICITY_PENDING"
	EventStatusPrimarySourceAuth     = "PRIMARY_SOURCE_AUTHENTICATED"
	EventStatusVerifiedExchPrimary   = "VERIFIED_EXCHANGE_PRIMARY"
	EventStatusAmbiguous             = "AMBIGUOUS"
	EventStatusRejected              = "REJECTED"
	EventStatusUnverified            = "UNVERIFIED"
)

// FinancialDisclosureSourceArtifact stores immutable primary statutory annual reports, XBRL filings, or circulars.
type FinancialDisclosureSourceArtifact struct {
	ID                 int        `json:"id" db:"id"`
	ArtifactID         string     `json:"artifact_id" db:"artifact_id"`
	SymbolID           int        `json:"symbol_id" db:"symbol_id"`
	Symbol             string     `json:"symbol" db:"symbol"`
	FiscalYear         int        `json:"fiscal_year" db:"fiscal_year"`
	PeriodType         string     `json:"period_type" db:"period_type"` // 'ANNUAL', 'QUARTERLY'
	PeriodEnd          time.Time  `json:"period_end" db:"period_end"`
	DocumentTitle      string     `json:"document_title" db:"document_title"`
	IssuingAuthority   string     `json:"issuing_authority" db:"issuing_authority"`
	SourceURL          string     `json:"source_url" db:"source_url"`
	HTTPStatus         *int       `json:"http_status,omitempty" db:"http_status"`
	ByteSize           *int64     `json:"byte_size,omitempty" db:"byte_size"`
	ContentType        *string    `json:"content_type,omitempty" db:"content_type"`
	FinalRedirectURL   *string    `json:"final_redirect_url,omitempty" db:"final_redirect_url"`
	RawPayload         *string    `json:"raw_payload,omitempty" db:"raw_payload"`
	SHA256Checksum     string     `json:"sha256_checksum" db:"sha256_checksum"`
	RetrievalTimestamp time.Time  `json:"retrieval_timestamp" db:"retrieval_timestamp"`
	AuditorFirm        *string    `json:"auditor_firm,omitempty" db:"auditor_firm"`
	AuditorOpinion     *string    `json:"auditor_opinion,omitempty" db:"auditor_opinion"`
	ValidationStatus   string     `json:"validation_status" db:"validation_status"`
	ReviewStatus       string     `json:"review_status" db:"review_status"`
	Reviewer           *string    `json:"reviewer,omitempty" db:"reviewer"`
	ReviewedAt         *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
}

// AuditedFootnoteDisclosure stores audited financial note extractions (e.g. Note 24, Note 28) with exact citations.
type AuditedFootnoteDisclosure struct {
	ID                      int       `json:"id" db:"id"`
	ArtifactID              int       `json:"artifact_id" db:"artifact_id"`
	SymbolID                int       `json:"symbol_id" db:"symbol_id"`
	Symbol                  string    `json:"symbol" db:"symbol"`
	PeriodEnd               time.Time `json:"period_end" db:"period_end"`
	DisclosureType          string    `json:"disclosure_type" db:"disclosure_type"` // 'INTEREST_INCOME', 'TOTAL_RECEIVABLES', etc.
	PageNumber              *int      `json:"page_number,omitempty" db:"page_number"`
	NoteReference           string    `json:"note_reference" db:"note_reference"`
	ExtractedPassage        string    `json:"extracted_passage" db:"extracted_passage"`
	ExtractedAmount         float64   `json:"extracted_amount" db:"extracted_amount"`
	Currency                string    `json:"currency" db:"currency"`
	Unit                    string    `json:"unit" db:"unit"`
	AmountInRawINR          float64   `json:"amount_in_raw_inr" db:"amount_in_raw_inr"`
	ReportingPeriodMonths   int       `json:"reporting_period_months" db:"reporting_period_months"`
	ParserName              string    `json:"parser_name" db:"parser_name"`
	ParserVersion           string    `json:"parser_version" db:"parser_version"`
	ParserConfidence        float64   `json:"parser_confidence" db:"parser_confidence"`
	ManualReviewStatus      string    `json:"manual_review_status" db:"manual_review_status"`
	ShariahMethodologyVer   string    `json:"shariah_methodology_version" db:"shariah_methodology_version"`
	IsVerified              bool      `json:"is_verified" db:"is_verified"`
	CreatedAt               time.Time `json:"created_at" db:"created_at"`
}

// CorporateActionReconciliationReview stores audit items for discrepancies between official exchange circulars and stored events.
type CorporateActionReconciliationReview struct {
	ID                   int       `json:"id" db:"id"`
	EventID              string    `json:"event_id" db:"event_id"`
	SymbolID             int       `json:"symbol_id" db:"symbol_id"`
	Symbol               string    `json:"symbol" db:"symbol"`
	SourceArtifactID     *int      `json:"source_artifact_id,omitempty" db:"source_artifact_id"`
	OfficialDocumentRef  *string   `json:"official_document_ref,omitempty" db:"official_document_ref"`
	DiscrepancyField     string    `json:"discrepancy_field" db:"discrepancy_field"`
	ExistingClaimValue   string    `json:"existing_claim_value" db:"existing_claim_value"`
	OfficialClaimValue   string    `json:"official_claim_value" db:"official_claim_value"`
	DiscrepancyReason    string    `json:"discrepancy_reason" db:"discrepancy_reason"`
	ReviewStatus         string    `json:"review_status" db:"review_status"`
	Reviewer             *string   `json:"reviewer,omitempty" db:"reviewer"`
	ResolutionNotes      *string   `json:"resolution_notes,omitempty" db:"resolution_notes"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time `json:"updated_at" db:"updated_at"`
}
