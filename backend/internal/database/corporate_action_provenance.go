package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// InsertSourceArtifact stores an immutable corporate action source document.
// If an identical artifact already exists, returns the existing ID without modifying stored content.
func (db *DB) InsertSourceArtifact(ctx context.Context, artifact *models.CorporateActionSourceArtifact) (int, error) {
	query := `
		INSERT INTO corporate_action_source_artifacts (
			artifact_id, issuing_authority, source_url, document_identifier,
			document_type, publication_timestamp, retrieval_timestamp,
			raw_payload, sha256_checksum, parser_version, validation_status,
			reviewer, reviewed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
		ON CONFLICT (artifact_id) DO NOTHING
		RETURNING id;
	`
	var id int
	err := db.QueryRowContext(
		ctx, query,
		artifact.ArtifactID, artifact.IssuingAuthority, artifact.SourceURL, artifact.DocumentIdentifier,
		artifact.DocumentType, artifact.PublicationTimestamp, artifact.RetrievalTimestamp,
		artifact.RawPayload, artifact.SHA256Checksum, artifact.ParserVersion, artifact.ValidationStatus,
		artifact.Reviewer, artifact.ReviewedAt,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// Artifact already exists; fetch existing ID
		selectQuery := `SELECT id FROM corporate_action_source_artifacts WHERE artifact_id = $1;`
		if err := db.QueryRowContext(ctx, selectQuery, artifact.ArtifactID).Scan(&id); err != nil {
			return 0, fmt.Errorf("failed to fetch existing artifact ID for %s: %w", artifact.ArtifactID, err)
		}
		return id, nil
	} else if err != nil {
		return 0, fmt.Errorf("insert source artifact failed: %w", err)
	}

	return id, nil
}

// GetSourceArtifactByChecksum retrieves an artifact by its cryptographic checksum.
func (db *DB) GetSourceArtifactByChecksum(ctx context.Context, checksum string) (*models.CorporateActionSourceArtifact, error) {
	query := `
		SELECT 
			id, artifact_id, issuing_authority, source_url, document_identifier,
			document_type, publication_timestamp, retrieval_timestamp,
			raw_payload, sha256_checksum, parser_version, validation_status,
			reviewer, reviewed_at, created_at
		FROM corporate_action_source_artifacts
		WHERE sha256_checksum = $1
		ORDER BY id DESC
		LIMIT 1;
	`
	var art models.CorporateActionSourceArtifact
	err := db.QueryRowContext(ctx, query, checksum).Scan(
		&art.ID, &art.ArtifactID, &art.IssuingAuthority, &art.SourceURL, &art.DocumentIdentifier,
		&art.DocumentType, &art.PublicationTimestamp, &art.RetrievalTimestamp,
		&art.RawPayload, &art.SHA256Checksum, &art.ParserVersion, &art.ValidationStatus,
		&art.Reviewer, &art.ReviewedAt, &art.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("get source artifact by checksum failed: %w", err)
	}
	return &art, nil
}

// InsertCorporateActionEvent inserts a normalized corporate action event.
// Idempotent: does not duplicate events on repeated runs.
func (db *DB) InsertCorporateActionEvent(ctx context.Context, ev *models.CorporateActionEvent) (int, error) {
	detailsJSON, _ := json.Marshal(ev.Details)
	rawPayloadJSON, _ := json.Marshal(ev.RawPayload)

	query := `
		INSERT INTO corporate_action_events (
			event_id, symbol_id, symbol, source_artifact_id, action_type,
			source_subject, announcement_date, ex_date, record_date,
			effective_date, payment_date, currency, dividend_amount,
			split_ratio_old, split_ratio_new, bonus_ratio_bonus, bonus_ratio_held,
			rights_subscription_price, rights_ratio_new, rights_ratio_old,
			tax_cost_allocation_pct, verification_status, coverage_status,
			details, raw_payload
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13,
			$14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25
		)
		ON CONFLICT (symbol_id, ex_date, action_type, event_id) DO NOTHING
		RETURNING id;
	`

	var id int
	err := db.QueryRowContext(
		ctx, query,
		ev.EventID, ev.SymbolID, ev.Symbol, ev.SourceArtifactID, ev.ActionType,
		ev.SourceSubject, ev.AnnouncementDate, ev.ExDate, ev.RecordDate,
		ev.EffectiveDate, ev.PaymentDate, ev.Currency, ev.DividendAmount,
		ev.SplitRatioOld, ev.SplitRatioNew, ev.BonusRatioBonus, ev.BonusRatioHeld,
		ev.RightsSubscriptionPrice, ev.RightsRatioNew, ev.RightsRatioOld,
		ev.TaxCostAllocationPct, ev.VerificationStatus, ev.CoverageStatus,
		detailsJSON, rawPayloadJSON,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// Already exists; fetch existing ID
		selectQuery := `SELECT id FROM corporate_action_events WHERE event_id = $1;`
		if err := db.QueryRowContext(ctx, selectQuery, ev.EventID).Scan(&id); err != nil {
			return 0, fmt.Errorf("failed to fetch existing event ID for %s: %w", ev.EventID, err)
		}
		return id, nil
	} else if err != nil {
		return 0, fmt.Errorf("insert corporate action event failed for %s: %w", ev.EventID, err)
	}

	return id, nil
}

// GetCorporateActionEvents retrieves all normalized events for a symbol.
func (db *DB) GetCorporateActionEvents(ctx context.Context, symbol string) ([]models.CorporateActionEvent, error) {
	query := `
		SELECT 
			id, event_id, symbol_id, symbol, source_artifact_id, action_type,
			source_subject, announcement_date, ex_date, record_date,
			effective_date, payment_date, currency, dividend_amount,
			split_ratio_old, split_ratio_new, bonus_ratio_bonus, bonus_ratio_held,
			rights_subscription_price, rights_ratio_new, rights_ratio_old,
			tax_cost_allocation_pct, verification_status, coverage_status,
			created_at, updated_at
		FROM corporate_action_events
		WHERE symbol = $1
		ORDER BY ex_date DESC, id DESC;
	`
	rows, err := db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("get corporate action events failed for %s: %w", symbol, err)
	}
	defer rows.Close()

	var events []models.CorporateActionEvent
	for rows.Next() {
		var ev models.CorporateActionEvent
		if err := rows.Scan(
			&ev.ID, &ev.EventID, &ev.SymbolID, &ev.Symbol, &ev.SourceArtifactID, &ev.ActionType,
			&ev.SourceSubject, &ev.AnnouncementDate, &ev.ExDate, &ev.RecordDate,
			&ev.EffectiveDate, &ev.PaymentDate, &ev.Currency, &ev.DividendAmount,
			&ev.SplitRatioOld, &ev.SplitRatioNew, &ev.BonusRatioBonus, &ev.BonusRatioHeld,
			&ev.RightsSubscriptionPrice, &ev.RightsRatioNew, &ev.RightsRatioOld,
			&ev.TaxCostAllocationPct, &ev.VerificationStatus, &ev.CoverageStatus,
			&ev.CreatedAt, &ev.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan corporate action event failed: %w", err)
		}
		events = append(events, ev)
	}
	return events, nil
}

// GetActiveNominalPriceFactors queries ca.v_active_nominal_price_factors to retrieve only
// nominal-price-altering actions (splits, bonuses, rights with dilution). Cash dividends are strictly excluded.
func (db *DB) GetActiveNominalPriceFactors(ctx context.Context, symbol string) ([]models.CorporateActionAdjustmentFactor, error) {
	query := `
		SELECT 
			symbol, action_type, factor_category, event_id, revision_number,
			ex_date, record_date, effective_date, price_factor,
			volume_adjustment_factor, is_verified, dispute_status, source_reference
		FROM ca.v_active_nominal_price_factors
		WHERE symbol = $1
		ORDER BY ex_date ASC;
	`
	rows, err := db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to query ca.v_active_nominal_price_factors: %w", err)
	}
	defer rows.Close()

	var factors []models.CorporateActionAdjustmentFactor
	for rows.Next() {
		var f models.CorporateActionAdjustmentFactor
		var sym string
		var priceFactor float64
		if err := rows.Scan(
			&sym, &f.ActionType, &f.FactorCategory, &f.EventID, &f.RevisionNumber,
			&f.ExDate, &f.RecordDate, &f.EffectiveDate, &priceFactor,
			&f.VolumeAdjustmentFactor, &f.IsVerified, &f.DisputeStatus, &f.SourceReference,
		); err != nil {
			return nil, fmt.Errorf("failed to scan nominal factor: %w", err)
		}
		f.NominalPriceFactor = priceFactor
		f.AdjustmentFactor = priceFactor
		f.PriceFactorApplicable = true
		factors = append(factors, f)
	}
	return factors, nil
}

// GetActiveTotalReturnFactors queries ca.v_active_total_return_factors to retrieve all
// total-return-altering actions (splits, bonuses, and cash dividends).
func (db *DB) GetActiveTotalReturnFactors(ctx context.Context, symbol string) ([]models.CorporateActionAdjustmentFactor, error) {
	query := `
		SELECT 
			symbol, action_type, factor_category, event_id, revision_number,
			ex_date, record_date, effective_date, price_factor,
			volume_adjustment_factor, is_verified, dispute_status, source_reference
		FROM ca.v_active_total_return_factors
		WHERE symbol = $1
		ORDER BY ex_date ASC;
	`
	rows, err := db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to query ca.v_active_total_return_factors: %w", err)
	}
	defer rows.Close()

	var factors []models.CorporateActionAdjustmentFactor
	for rows.Next() {
		var f models.CorporateActionAdjustmentFactor
		var sym string
		var priceFactor float64
		if err := rows.Scan(
			&sym, &f.ActionType, &f.FactorCategory, &f.EventID, &f.RevisionNumber,
			&f.ExDate, &f.RecordDate, &f.EffectiveDate, &priceFactor,
			&f.VolumeAdjustmentFactor, &f.IsVerified, &f.DisputeStatus, &f.SourceReference,
		); err != nil {
			return nil, fmt.Errorf("failed to scan total return factor: %w", err)
		}
		f.TotalReturnFactor = priceFactor
		f.AdjustmentFactor = priceFactor
		f.PriceFactorApplicable = true
		factors = append(factors, f)
	}
	return factors, nil
}

// HarvestingSymbolCoverage summarizes corporate action evidence and events for a single symbol.
type HarvestingSymbolCoverage struct {
	SymbolID            int        `json:"symbol_id"`
	Symbol              string     `json:"symbol"`
	IsActive            bool       `json:"is_active"`
	InstrumentCategory  string     `json:"instrument_category"` // "CORE_EQUITY", "LINEAGE_RENAME", "LINEAGE_DEMERGER"
	ArtifactsCount      int        `json:"artifacts_count"`
	LatestArtifactDate  *time.Time `json:"latest_artifact_date"`
	EventsCount         int        `json:"events_count"`
	DividendsCount      int        `json:"dividends_count"`
	SplitsCount         int        `json:"splits_count"`
	BonusesCount        int        `json:"bonuses_count"`
	RightsCount         int        `json:"rights_count"`
	DemergersCount      int        `json:"demergers_count"`
	RenamesCount        int        `json:"renames_count"`
	VerifiedEventsCount int        `json:"verified_events_count"`
	EarliestEventDate   *time.Time `json:"earliest_event_date"`
	LatestEventDate     *time.Time `json:"latest_event_date"`
	CoverageStatus      string     `json:"coverage_status"`       // "HARVESTED_EXCHANGE_PRIMARY", "NO_EVENT_CONFIRMED", "NOT_YET_HARVESTED"
	DataReadinessState  string     `json:"data_readiness_state"`  // 7-state hierarchy: SOURCE_MISSING, ACCESS_DENIED, ARTIFACT_DEPOSITED, PARSE_VALIDATED, PRIMARY_SOURCE_VERIFIED, FACTOR_VALIDATED, ELIGIBLE_FOR_SPECIFIC_USE
}

// GetUniverseHarvestingCoverage returns the complete coverage matrix across all 52 universe symbols.
func (db *DB) GetUniverseHarvestingCoverage(ctx context.Context) ([]HarvestingSymbolCoverage, error) {
	query := `
		SELECT 
			s.id,
			s.symbol,
			s.is_active,
			CASE 
				WHEN s.symbol IN ('LTIM', 'LTM') THEN 'LINEAGE_RENAME'
				WHEN s.symbol IN ('TATAMOTORS', 'TMPV', 'TMCV') THEN 'LINEAGE_DEMERGER'
				ELSE 'CORE_EQUITY'
			END AS instrument_category,
			COALESCE(art.art_count, 0) AS artifacts_count,
			art.latest_art_date,
			COALESCE(ev.ev_count, 0) AS events_count,
			COALESCE(ev.div_count, 0) AS dividends_count,
			COALESCE(ev.split_count, 0) AS splits_count,
			COALESCE(ev.bonus_count, 0) AS bonuses_count,
			COALESCE(ev.rights_count, 0) AS rights_count,
			COALESCE(ev.dem_count, 0) AS demergers_count,
			COALESCE(ev.ren_count, 0) AS renames_count,
			COALESCE(ev.ver_count, 0) AS verified_events_count,
			ev.earliest_date,
			ev.latest_date,
			CASE
				WHEN COALESCE(act.active_fac_count, 0) > 0 THEN 'ELIGIBLE_FOR_SPECIFIC_USE'
				WHEN COALESCE(fac.ver_fac_count, 0) > 0 THEN 'FACTOR_VALIDATED'
				WHEN COALESCE(ev.ver_count, 0) > 0 THEN 'PRIMARY_SOURCE_VERIFIED'
				WHEN COALESCE(ev.ev_count, 0) > 0 THEN 'PARSE_VALIDATED'
				WHEN COALESCE(art.art_count, 0) > 0 THEN 'ARTIFACT_DEPOSITED'
				ELSE 'SOURCE_MISSING'
			END AS data_readiness_state
		FROM symbols s
		LEFT JOIN (
			SELECT 
				SPLIT_PART(document_identifier, '_', 3) as symbol,
				count(*) as art_count,
				max(retrieval_timestamp) as latest_art_date
			FROM corporate_action_source_artifacts
			GROUP BY SPLIT_PART(document_identifier, '_', 3)
		) art ON s.symbol = art.symbol
		LEFT JOIN (
			SELECT 
				symbol_id,
				count(*) as ev_count,
				count(*) FILTER (WHERE action_type = 'CASH_DIVIDEND') as div_count,
				count(*) FILTER (WHERE action_type = 'STOCK_SPLIT') as split_count,
				count(*) FILTER (WHERE action_type = 'BONUS_ISSUE') as bonus_count,
				count(*) FILTER (WHERE action_type = 'RIGHTS_ISSUE') as rights_count,
				count(*) FILTER (WHERE action_type = 'DEMERGER') as dem_count,
				count(*) FILTER (WHERE action_type = 'SYMBOL_CHANGE') as ren_count,
				count(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY') as ver_count,
				min(ex_date) as earliest_date,
				max(ex_date) as latest_date
			FROM corporate_action_events
			GROUP BY symbol_id
		) ev ON s.id = ev.symbol_id
		LEFT JOIN (
			SELECT
				symbol_id,
				count(*) as fac_count,
				count(*) FILTER (WHERE is_verified = true) as ver_fac_count
			FROM corporate_action_adjustment_factors
			GROUP BY symbol_id
		) fac ON s.id = fac.symbol_id
		LEFT JOIN (
			SELECT 
				s2.id as symbol_id,
				count(*) as active_fac_count
			FROM (
				SELECT symbol FROM ca.v_active_nominal_price_factors
				UNION ALL
				SELECT symbol FROM ca.v_active_total_return_factors
			) v
			JOIN symbols s2 ON v.symbol = s2.symbol
			GROUP BY s2.id
		) act ON s.id = act.symbol_id
		WHERE COALESCE(s.metadata->>'user_added', 'false') <> 'true'
		ORDER BY s.symbol ASC;
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query universe harvesting coverage: %w", err)
	}
	defer rows.Close()

	var coverage []HarvestingSymbolCoverage
	for rows.Next() {
		var c HarvestingSymbolCoverage
		if err := rows.Scan(
			&c.SymbolID, &c.Symbol, &c.IsActive, &c.InstrumentCategory, &c.ArtifactsCount, &c.LatestArtifactDate,
			&c.EventsCount, &c.DividendsCount, &c.SplitsCount, &c.BonusesCount,
			&c.RightsCount, &c.DemergersCount, &c.RenamesCount, &c.VerifiedEventsCount,
			&c.EarliestEventDate, &c.LatestEventDate, &c.DataReadinessState,
		); err != nil {
			return nil, fmt.Errorf("failed to scan coverage row: %w", err)
		}

		if c.EventsCount > 0 {
			c.CoverageStatus = "HARVESTED_EXCHANGE_PRIMARY"
		} else if c.ArtifactsCount > 0 {
			c.CoverageStatus = "NO_EVENT_CONFIRMED"
		} else {
			c.CoverageStatus = "NOT_YET_HARVESTED"
		}

		coverage = append(coverage, c)
	}
	return coverage, nil
}

// ApplyVerifiedCorporateActionAdjustment safely applies a verified corporate-action adjustment factor
// to historical bars in the verified window [startDate, exDate) for the specified symbol.
// Fail-closed invariants enforced:
// ApplyVerifiedCorporateActionAdjustment safely applies a verified corporate-action adjustment factor
// to historical bars in the verified window [startDate, exDate) for the specified symbol.
// Fail-closed invariants enforced:
// 1. The eventID must exist in corporate_action_events and have verification_status = 'VERIFIED_EXCHANGE_PRIMARY'.
// 2. The factor must be strictly positive and match verified factor in corporate_action_adjustment_factors.
// 3. Executes ca.apply_verified_corporate_action stored procedure within the database trust boundary.
// 4. Updates ohlcv_daily with source = 'NSE_CORPORATE_ACTION' so that trigger and upsert guards preserve it.
// 5. Quarantined NULL bars outside the verified window remain strictly untouched.
func (db *DB) ApplyVerifiedCorporateActionAdjustment(ctx context.Context, symbol string, startDate, exDate time.Time, factor float64, eventID string) (int64, error) {
	if factor <= 0 {
		return 0, fmt.Errorf("adjustment factor must be positive, got %f", factor)
	}

	var rowsUpdated int64
	err := db.QueryRowContext(ctx, `
		SELECT ca.apply_verified_corporate_action($1, $2::date, $3::date, $4::numeric, $5);
	`, symbol, startDate.Format("2006-01-02"), exDate.Format("2006-01-02"), factor, eventID).Scan(&rowsUpdated)
	if err != nil {
		return 0, fmt.Errorf("apply verified corporate action failed: %w", err)
	}

	return rowsUpdated, nil
}


