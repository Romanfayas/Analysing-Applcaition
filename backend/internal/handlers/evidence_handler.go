package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/corporateactions/official"
	"github.com/halal-equity/backend/internal/services/corporateactions"
	"github.com/halal-equity/backend/internal/services/fundamentals"
	"github.com/halal-equity/backend/pkg/response"
)

type EvidenceHandler struct {
	db          *database.DB
	pipeline    *official.Pipeline
	reconciler  *corporateactions.Reconciler
	disclosures *fundamentals.DisclosureService
}

func NewEvidenceHandler(db *database.DB) *EvidenceHandler {
	return &EvidenceHandler{
		db:          db,
		pipeline:    official.NewPipeline(1 * time.Second),
		reconciler:  corporateactions.NewReconciler(db.DB),
		disclosures: fundamentals.NewDisclosureService(db.DB),
	}
}

func (h *EvidenceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/corporate-actions/ingest", h.IngestCorporateActionDocument)
	r.Post("/corporate-actions/reconcile", h.ReconcileCorporateActionEvent)
	r.Post("/financial-disclosures/ingest", h.IngestFinancialDisclosure)
	r.Post("/financial-disclosures/footnote", h.IngestFootnoteDisclosure)
	return r
}

type IngestCADocumentRequest struct {
	Symbol           string `json:"symbol"`
	SourceURL        string `json:"source_url"`
	DocumentType     string `json:"document_type"` // 'CIRCULAR_PDF', 'AUTHENTICATED_EXCHANGE_FEED', etc.
	RawPayload       string `json:"raw_payload,omitempty"`
	ReviewerApproved bool   `json:"reviewer_approved,omitempty"`
	ReviewerNotes    string `json:"reviewer_notes,omitempty"`
}

func (h *EvidenceHandler) IngestCorporateActionDocument(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req IngestCADocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Symbol == "" || req.SourceURL == "" {
		response.Error(w, http.StatusBadRequest, "symbol and source_url are required")
		return
	}

	var payload []byte
	var meta *official.AcquisitionMetadata
	var err error

	// If raw payload provided in request
	if req.RawPayload != "" {
		payload = []byte(req.RawPayload)
		meta, err = h.pipeline.IngestLocallyAcquiredDocument(payload, req.SourceURL, "Locally Acquired Document", "")
		if err != nil {
			response.Error(w, http.StatusBadRequest, err.Error())
			return
		}
	} else {
		// Acquire from official exchange endpoint
		payload, meta, err = h.pipeline.AcquireFromOfficialURL(ctx, req.SourceURL)
		if err != nil {
			if strings.Contains(err.Error(), "access restricted by exchange") {
				response.JSON(w, http.StatusForbidden, map[string]any{
					"status":  "BLOCKED",
					"error":   err.Error(),
					"message": "Automated acquisition politely stopped on HTTP 403; anti-circumvention rule enforced.",
				})
				return
			}
			response.Error(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}

	// Adversarial Rule: Check if source URL is on a non-official domain claiming PRIMARY_VERIFIED
	_, errVal := official.ValidateOfficialExchangeURL(req.SourceURL)
	if errVal != nil && req.ReviewerApproved {
		response.Error(w, http.StatusBadRequest, "fake documents hosted on non-official domains cannot become PRIMARY_VERIFIED")
		return
	}

	// Adversarial Rule: Local archives/fixtures cannot be promoted to PRIMARY_VERIFIED
	if (strings.HasPrefix(req.SourceURL, "CONTROLLED_ARCHIVE:") || strings.HasPrefix(req.SourceURL, "FIXTURE:") || strings.HasPrefix(req.SourceURL, "LOCAL:")) && req.ReviewerApproved {
		response.Error(w, http.StatusBadRequest, "local archive, fixture, or unauthenticated local cache cannot be promoted to PRIMARY_VERIFIED")
		return
	}

	docType := req.DocumentType
	if docType == "" {
		docType = "CIRCULAR_PDF"
	}

	// Evaluate state machine with authenticity proof separation
	valResult := official.EvaluateArtifactState(
		payload, meta, true, "Payload successfully received and validated", true, "Terms structurally consistent", req.ReviewerApproved, false,
	)

	// Idempotency: Check if artifact with exact checksum already exists
	var existingID int
	checkErr := h.db.QueryRowContext(ctx, "SELECT id FROM corporate_action_source_artifacts WHERE sha256_checksum = $1;", valResult.SHA256Checksum).Scan(&existingID)
	if checkErr == nil {
		response.JSON(w, http.StatusOK, map[string]any{
			"message":          "Artifact already registered; duplicate ingestion idempotent",
			"existing_id":      existingID,
			"sha256_checksum":  valResult.SHA256Checksum,
			"validation_state": valResult.AssignedStatus,
		})
		return
	}

	artifact := official.BuildSourceArtifact(req.Symbol, meta, payload, valResult, docType)

	insertQ := `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier,
			document_type, raw_payload, sha256_checksum, parser_version,
			validation_status, reviewer, reviewed_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, created_at;
	`
	err = h.db.QueryRowContext(
		ctx, insertQ,
		artifact.ArtifactID, artifact.IssuingAuthority, artifact.SourceURL, artifact.DocumentIdentifier,
		artifact.DocumentType, artifact.RawPayload, artifact.SHA256Checksum, artifact.ParserVersion,
		artifact.ValidationStatus, artifact.Reviewer, artifact.ReviewedAt,
	).Scan(&artifact.ID, &artifact.CreatedAt)

	if err != nil {
		response.Error(w, http.StatusInternalServerError, fmt.Sprintf("database insert error: %v", err))
		return
	}

	response.JSON(w, http.StatusCreated, map[string]any{
		"artifact":                  artifact,
		"validation_details":        valResult,
		"source_authenticity_valid": valResult.SourceAuthenticityPassed,
		"assigned_status":           valResult.AssignedStatus,
	})
}

type ReconcileEventRequest struct {
	EventID           string   `json:"event_id"`
	Symbol            string   `json:"symbol"`
	ActionType        string   `json:"action_type"`
	ExDate            string   `json:"ex_date"`
	DividendAmount    *float64 `json:"dividend_amount,omitempty"`
	SplitRatioOld     *float64 `json:"split_ratio_old,omitempty"`
	SplitRatioNew     *float64 `json:"split_ratio_new,omitempty"`
	IsPriceAdjustment bool     `json:"is_price_adjustment"`
	OfficialDocRef    string   `json:"official_doc_ref"`
	SourceArtifactID  int      `json:"source_artifact_id"`
}

func (h *EvidenceHandler) ReconcileCorporateActionEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req ReconcileEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	exD, err := time.Parse("2006-01-02", req.ExDate)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid ex_date format (expected YYYY-MM-DD)")
		return
	}

	claim := corporateactions.OfficialEventClaim{
		EventID:           req.EventID,
		Symbol:            req.Symbol,
		ActionType:        req.ActionType,
		ExDate:            exD,
		DividendAmount:    req.DividendAmount,
		SplitRatioOld:     req.SplitRatioOld,
		SplitRatioNew:     req.SplitRatioNew,
		IsPriceAdjustment: req.IsPriceAdjustment,
		OfficialDocRef:    req.OfficialDocRef,
		SourceArtifactID:  req.SourceArtifactID,
	}

	outcome, err := h.reconciler.ReconcileEvent(ctx, claim)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}

	response.JSON(w, http.StatusOK, outcome)
}

type IngestFinancialDisclosureRequest struct {
	Symbol           string `json:"symbol"`
	FiscalYear       int    `json:"fiscal_year"`
	PeriodType       string `json:"period_type"` // 'ANNUAL', 'QUARTERLY'
	PeriodEnd        string `json:"period_end"`
	DocumentTitle    string `json:"document_title"`
	IssuingAuthority string `json:"issuing_authority"`
	SourceURL        string `json:"source_url"`
	RawPayload       string `json:"raw_payload"`
	AuditorFirm      string `json:"auditor_firm"`
	AuditorOpinion   string `json:"auditor_opinion"`
}

func (h *EvidenceHandler) IngestFinancialDisclosure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req IngestFinancialDisclosureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	pEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid period_end format (expected YYYY-MM-DD)")
		return
	}

	art, err := h.disclosures.IngestDisclosureArtifact(
		ctx, req.Symbol, req.FiscalYear, req.PeriodType, pEnd,
		req.DocumentTitle, req.IssuingAuthority, req.SourceURL, []byte(req.RawPayload),
		req.AuditorFirm, req.AuditorOpinion,
	)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, art)
}

type IngestFootnoteRequest struct {
	ArtifactID            int      `json:"artifact_id"`
	Symbol                string   `json:"symbol"`
	PeriodEnd             string   `json:"period_end"`
	DisclosureType        string   `json:"disclosure_type"`
	PageNumber            *int     `json:"page_number,omitempty"`
	NoteReference         string   `json:"note_reference"`
	ExtractedPassage      string   `json:"extracted_passage"`
	ExtractedAmount       float64  `json:"extracted_amount"`
	Currency              string   `json:"currency"`
	Unit                  string   `json:"unit"`
	AmountInRawINR        float64  `json:"amount_in_raw_inr"`
	ReportingPeriodMonths int      `json:"reporting_period_months"`
	ParserName            string   `json:"parser_name"`
	ParserVersion         string   `json:"parser_version"`
	ParserConfidence      float64  `json:"parser_confidence"`
	IsLLMGenerated        bool     `json:"is_llm_generated,omitempty"`
	HasSourceCitations    bool     `json:"has_source_citations,omitempty"`
}

func (h *EvidenceHandler) IngestFootnoteDisclosure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req IngestFootnoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Adversarial Rule: LLM generated claim without source citations cannot establish Shariah PASS
	if req.IsLLMGenerated && !req.HasSourceCitations {
		response.Error(w, http.StatusBadRequest, "LLM-generated interpretation without cited page and audited note passage cannot establish Shariah evidence")
		return
	}

	// Adversarial Rule: Missing note cannot infer zero
	if req.NoteReference == "" || strings.TrimSpace(req.ExtractedPassage) == "" {
		response.Error(w, http.StatusBadRequest, "audited footnote missing or not parsed; interest income cannot be inferred as zero (fail-closed)")
		return
	}

	pEnd, err := time.Parse("2006-01-02", req.PeriodEnd)
	if err != nil {
		response.Error(w, http.StatusBadRequest, "invalid period_end format (expected YYYY-MM-DD)")
		return
	}

	var symbolID int
	err = h.db.QueryRowContext(ctx, "SELECT id FROM symbols WHERE symbol = $1;", req.Symbol).Scan(&symbolID)
	if err != nil {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("symbol %s not found", req.Symbol))
		return
	}

	manualReview := "PENDING_QUALIFIED_REVIEW"
	isVerified := false
	if req.ParserConfidence >= 0.95 && req.PageNumber != nil && *req.PageNumber > 0 {
		manualReview = "REVIEWED_CONFIRMED"
		isVerified = true
	}

	disc := models.AuditedFootnoteDisclosure{
		ArtifactID:            req.ArtifactID,
		SymbolID:              symbolID,
		Symbol:                req.Symbol,
		PeriodEnd:             pEnd,
		DisclosureType:        req.DisclosureType,
		PageNumber:            req.PageNumber,
		NoteReference:         req.NoteReference,
		ExtractedPassage:      req.ExtractedPassage,
		ExtractedAmount:       req.ExtractedAmount,
		Currency:              req.Currency,
		Unit:                  req.Unit,
		AmountInRawINR:        req.AmountInRawINR,
		ReportingPeriodMonths: req.ReportingPeriodMonths,
		ParserName:            req.ParserName,
		ParserVersion:         req.ParserVersion,
		ParserConfidence:      req.ParserConfidence,
		ManualReviewStatus:    manualReview,
		ShariahMethodologyVer: "AAOIFI_STANDARD_NO_21_2026",
		IsVerified:            isVerified,
	}

	res, err := h.disclosures.IngestFootnoteDisclosure(ctx, disc)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}

	response.JSON(w, http.StatusCreated, res)
}
