package corporateactions

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// DBTX is the common interface satisfied by *sql.DB and *sql.Tx.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Reconciler executes deterministic reconciliation between official documents and corporate-action records.
type Reconciler struct {
	db DBTX
}

// NewReconciler initializes a corporate actions reconciliation service.
func NewReconciler(db DBTX) *Reconciler {
	return &Reconciler{db: db}
}

// OfficialEventClaim represents an extracted claim from an official exchange document.
type OfficialEventClaim struct {
	EventID           string
	Symbol            string
	ActionType        string // 'CASH_DIVIDEND', 'STOCK_SPLIT', 'BONUS_ISSUE', 'DEMERGER', 'SYMBOL_RENAME'
	AnnouncementDate  *time.Time
	ExDate            time.Time
	RecordDate        *time.Time
	EffectiveDate     *time.Time
	Currency          string
	DividendAmount    *float64
	SplitRatioOld     *float64
	SplitRatioNew     *float64
	BonusRatioBonus   *float64
	BonusRatioHeld    *float64
	TaxCostAllocPct   *float64
	IsPriceAdjustment bool
	OfficialDocRef    string
	SourceArtifactID  int
}

// ReconciliationOutcome details discrepancies and generated review items.
type ReconciliationOutcome struct {
	EventID            string
	Symbol             string
	IsReconciled       bool
	HasDiscrepancy     bool
	DiscrepancyItems   []models.CorporateActionReconciliationReview
	ReconciliationNote string
}

// ReconcileEvent compares an official claim against stored corporate action records in the database.
func (r *Reconciler) ReconcileEvent(ctx context.Context, claim OfficialEventClaim) (*ReconciliationOutcome, error) {
	outcome := &ReconciliationOutcome{
		EventID:      claim.EventID,
		Symbol:       claim.Symbol,
		IsReconciled: true,
	}

	// 1. Lineage & Tax-Cost Allocation Rule:
	// Demerger tax cost allocation and lineage renames must NEVER be treated as price adjustments.
	if claim.ActionType == "DEMERGER" || claim.ActionType == "SYMBOL_RENAME" {
		if claim.IsPriceAdjustment {
			rev := models.CorporateActionReconciliationReview{
				EventID:             claim.EventID,
				Symbol:              claim.Symbol,
				SourceArtifactID:    &claim.SourceArtifactID,
				OfficialDocumentRef: &claim.OfficialDocRef,
				DiscrepancyField:    "IS_PRICE_ADJUSTMENT",
				ExistingClaimValue:  "true",
				OfficialClaimValue:  "false",
				DiscrepancyReason:   "Demerger tax-cost allocation and symbology lineage cannot be applied as price adjustment factors (fail-closed rule)",
				ReviewStatus:        "PENDING_REVIEW",
			}
			outcome.HasDiscrepancy = true
			outcome.IsReconciled = false
			outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
			_ = r.persistReview(ctx, rev)
		}
	}

	// 2. Fetch Stored Corporate Action Event
	var stored models.CorporateActionEvent
	query := `
		SELECT id, event_id, symbol_id, symbol, action_type, ex_date,
		       dividend_amount, split_ratio_old, split_ratio_new, bonus_ratio_bonus,
		       bonus_ratio_held, verification_status
		FROM corporate_action_events
		WHERE event_id = $1;
	`
	err := r.db.QueryRowContext(ctx, query, claim.EventID).Scan(
		&stored.ID, &stored.EventID, &stored.SymbolID, &stored.Symbol, &stored.ActionType,
		&stored.ExDate, &stored.DividendAmount, &stored.SplitRatioOld, &stored.SplitRatioNew,
		&stored.BonusRatioBonus, &stored.BonusRatioHeld, &stored.VerificationStatus,
	)

	if err == sql.ErrNoRows {
		// Event not registered yet: create new unverified candidate review
		rev := models.CorporateActionReconciliationReview{
			EventID:             claim.EventID,
			Symbol:              claim.Symbol,
			SourceArtifactID:    &claim.SourceArtifactID,
			OfficialDocumentRef: &claim.OfficialDocRef,
			DiscrepancyField:    "EVENT_UNREGISTERED",
			ExistingClaimValue:  "NONE",
			OfficialClaimValue:  claim.ActionType,
			DiscrepancyReason:   "Official corporate action announced but not present in database registry",
			ReviewStatus:        "PENDING_REVIEW",
		}
		outcome.HasDiscrepancy = true
		outcome.IsReconciled = false
		outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
		_ = r.persistReview(ctx, rev)
		outcome.ReconciliationNote = "Unregistered event claim queued for ingestion and review."
		return outcome, nil
	} else if err != nil {
		return nil, fmt.Errorf("failed to fetch event %s: %w", claim.EventID, err)
	}

	// 3. Compare Action Type
	if stored.ActionType != claim.ActionType {
		rev := models.CorporateActionReconciliationReview{
			EventID:             claim.EventID,
			SymbolID:            stored.SymbolID,
			Symbol:              claim.Symbol,
			SourceArtifactID:    &claim.SourceArtifactID,
			OfficialDocumentRef: &claim.OfficialDocRef,
			DiscrepancyField:    "ACTION_TYPE",
			ExistingClaimValue:  stored.ActionType,
			OfficialClaimValue:  claim.ActionType,
			DiscrepancyReason:   fmt.Sprintf("Action type mismatch: stored %s vs official %s", stored.ActionType, claim.ActionType),
			ReviewStatus:        "PENDING_REVIEW",
		}
		outcome.HasDiscrepancy = true
		outcome.IsReconciled = false
		outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
		_ = r.persistReview(ctx, rev)
	}

	// 4. Compare Ex-Date
	if !stored.ExDate.Equal(claim.ExDate) && stored.ExDate.Format("2006-01-02") != claim.ExDate.Format("2006-01-02") {
		rev := models.CorporateActionReconciliationReview{
			EventID:             claim.EventID,
			SymbolID:            stored.SymbolID,
			Symbol:              claim.Symbol,
			SourceArtifactID:    &claim.SourceArtifactID,
			OfficialDocumentRef: &claim.OfficialDocRef,
			DiscrepancyField:    "EX_DATE",
			ExistingClaimValue:  stored.ExDate.Format("2006-01-02"),
			OfficialClaimValue:  claim.ExDate.Format("2006-01-02"),
			DiscrepancyReason:   fmt.Sprintf("Ex-date mismatch: stored %s vs official %s", stored.ExDate.Format("2006-01-02"), claim.ExDate.Format("2006-01-02")),
			ReviewStatus:        "PENDING_REVIEW",
		}
		outcome.HasDiscrepancy = true
		outcome.IsReconciled = false
		outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
		_ = r.persistReview(ctx, rev)
	}

	// 5. Compare Terms: Cash Dividend Amount
	if claim.ActionType == "CASH_DIVIDEND" && claim.DividendAmount != nil {
		diff := math.Abs(*stored.DividendAmount - *claim.DividendAmount)
		if diff > 0.0001 {
			rev := models.CorporateActionReconciliationReview{
				EventID:             claim.EventID,
				SymbolID:            stored.SymbolID,
				Symbol:              claim.Symbol,
				SourceArtifactID:    &claim.SourceArtifactID,
				OfficialDocumentRef: &claim.OfficialDocRef,
				DiscrepancyField:    "DIVIDEND_AMOUNT",
				ExistingClaimValue:  fmt.Sprintf("%.4f", *stored.DividendAmount),
				OfficialClaimValue:  fmt.Sprintf("%.4f", *claim.DividendAmount),
				DiscrepancyReason:   fmt.Sprintf("Dividend amount discrepancy: stored ₹%.2f vs official ₹%.2f", *stored.DividendAmount, *claim.DividendAmount),
				ReviewStatus:        "PENDING_REVIEW",
			}
			outcome.HasDiscrepancy = true
			outcome.IsReconciled = false
			outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
			_ = r.persistReview(ctx, rev)
		}
	}

	// 6. Compare Terms: Stock Split Ratio
	if claim.ActionType == "STOCK_SPLIT" && claim.SplitRatioOld != nil && claim.SplitRatioNew != nil {
		if math.Abs(*stored.SplitRatioOld-*claim.SplitRatioOld) > 0.0001 || math.Abs(*stored.SplitRatioNew-*claim.SplitRatioNew) > 0.0001 {
			rev := models.CorporateActionReconciliationReview{
				EventID:             claim.EventID,
				SymbolID:            stored.SymbolID,
				Symbol:              claim.Symbol,
				SourceArtifactID:    &claim.SourceArtifactID,
				OfficialDocumentRef: &claim.OfficialDocRef,
				DiscrepancyField:    "SPLIT_RATIO",
				ExistingClaimValue:  fmt.Sprintf("%.1f:%.1f", *stored.SplitRatioOld, *stored.SplitRatioNew),
				OfficialClaimValue:  fmt.Sprintf("%.1f:%.1f", *claim.SplitRatioOld, *claim.SplitRatioNew),
				DiscrepancyReason:   "Split ratio terms discrepancy",
				ReviewStatus:        "PENDING_REVIEW",
			}
			outcome.HasDiscrepancy = true
			outcome.IsReconciled = false
			outcome.DiscrepancyItems = append(outcome.DiscrepancyItems, rev)
			_ = r.persistReview(ctx, rev)
		}
	}

	if outcome.HasDiscrepancy {
		outcome.ReconciliationNote = fmt.Sprintf("Discrepancies flagged; %d review items queued. Both claims retained.", len(outcome.DiscrepancyItems))
	} else {
		outcome.ReconciliationNote = "Official claims match existing record exactly across all audited terms."
	}

	return outcome, nil
}

// persistReview saves a discrepancy review item with deduplication to prevent duplicate PENDING_REVIEW rows.
func (r *Reconciler) persistReview(ctx context.Context, rev models.CorporateActionReconciliationReview) error {
	// Deduplication: check if an identical pending review already exists
	var existingID int
	checkQuery := `
		SELECT id FROM corporate_action_reconciliation_reviews
		WHERE event_id = $1 AND discrepancy_field = $2 AND official_claim_value = $3 AND review_status = 'PENDING_REVIEW'
		LIMIT 1;
	`
	err := r.db.QueryRowContext(ctx, checkQuery, rev.EventID, rev.DiscrepancyField, rev.OfficialClaimValue).Scan(&existingID)
	if err == nil && existingID > 0 {
		return nil // Already tracked; avoid duplicate row accumulation
	}

	query := `
		INSERT INTO corporate_action_reconciliation_reviews (
			event_id, symbol_id, symbol, source_artifact_id, official_document_ref,
			discrepancy_field, existing_claim_value, official_claim_value, discrepancy_reason, review_status
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id;
	`
	var id int
	return r.db.QueryRowContext(
		ctx, query,
		rev.EventID, rev.SymbolID, rev.Symbol, rev.SourceArtifactID, rev.OfficialDocumentRef,
		rev.DiscrepancyField, rev.ExistingClaimValue, rev.OfficialClaimValue, rev.DiscrepancyReason, rev.ReviewStatus,
	).Scan(&id)
}
