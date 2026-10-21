package models

import (
	"time"
)

// ==================================================
// Market Data Models
// ==================================================

// Symbol represents a stock or index listed on an exchange.
type Symbol struct {
	ID               int       `json:"id" db:"id"`
	Symbol           string    `json:"symbol" db:"symbol"`
	Name             string    `json:"name" db:"name"`
	Exchange         string    `json:"exchange" db:"exchange"`
	ISIN             string    `json:"isin,omitempty" db:"isin"`
	Sector           string    `json:"sector,omitempty" db:"sector"`
	Industry         string    `json:"industry,omitempty" db:"industry"`
	MarketCapCategory string   `json:"market_cap_category,omitempty" db:"market_cap_category"`
	IsActive         bool      `json:"is_active" db:"is_active"`
	ListingDate      *time.Time `json:"listing_date,omitempty" db:"listing_date"`
	Metadata         any       `json:"metadata,omitempty" db:"metadata"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time `json:"updated_at" db:"updated_at"`
}

// OHLCV represents a single candlestick data point.
// Every data point records source, retrieval timestamp, and quality status
// as required by the specification.
type OHLCV struct {
	SymbolID      int               `json:"symbol_id" db:"symbol_id"`
	Timestamp     time.Time         `json:"timestamp" db:"timestamp"`
	Open          float64           `json:"open" db:"open"`
	High          float64           `json:"high" db:"high"`
	Low           float64           `json:"low" db:"low"`
	Close         float64           `json:"close" db:"close"`
	Volume        int64             `json:"volume" db:"volume"`
	AdjustedClose *float64          `json:"adjusted_close,omitempty" db:"adjusted_close"`
	Source        string            `json:"source" db:"source"`
	RetrievedAt   time.Time         `json:"retrieved_at" db:"retrieved_at"`
	QualityStatus DataQualityStatus `json:"quality_status" db:"quality_status"`
}

// CorporateAction represents a stock split, bonus, dividend, etc.
type CorporateAction struct {
	ID                 int        `json:"id" db:"id"`
	SymbolID           int        `json:"symbol_id" db:"symbol_id"`
	ActionType         string     `json:"action_type" db:"action_type"`
	ExDate             time.Time  `json:"ex_date" db:"ex_date"`
	RecordDate         *time.Time `json:"record_date,omitempty" db:"record_date"`
	Description        string     `json:"description,omitempty" db:"description"`
	OldRatio           *float64   `json:"old_ratio,omitempty" db:"old_ratio"`
	NewRatio           *float64   `json:"new_ratio,omitempty" db:"new_ratio"`
	DividendAmount     *float64   `json:"dividend_amount,omitempty" db:"dividend_amount"`
	DividendPercentage *float64   `json:"dividend_percentage,omitempty" db:"dividend_percentage"`
	Source             string     `json:"source" db:"source"`
	CreatedAt          time.Time  `json:"created_at" db:"created_at"`
}

// CorporateActionAdjustmentFactor represents an audited, versioned factor in the corporate action ledger.
type CorporateActionAdjustmentFactor struct {
	ID                     int        `json:"id" db:"id"`
	SymbolID               int        `json:"symbol_id" db:"symbol_id"`
	ActionType             string     `json:"action_type" db:"action_type"`
	FactorCategory         string     `json:"factor_category" db:"factor_category"`
	EventID                string     `json:"event_id" db:"event_id"`
	ExDate                 time.Time  `json:"ex_date" db:"ex_date"`
	RecordDate             *time.Time `json:"record_date,omitempty" db:"record_date"`
	EffectiveDate          time.Time  `json:"effective_date" db:"effective_date"`
	SplitRatioOld          *float64   `json:"split_ratio_old,omitempty" db:"split_ratio_old"`
	SplitRatioNew          *float64   `json:"split_ratio_new,omitempty" db:"split_ratio_new"`
	DividendAmount         *float64   `json:"dividend_amount,omitempty" db:"dividend_amount"`
	CashDividendPerShare   *float64   `json:"cash_dividend_per_share,omitempty" db:"cash_dividend_per_share"`
	AdjustmentFactor       float64    `json:"adjustment_factor" db:"adjustment_factor"`
	CumulativeSplitFactor  float64    `json:"cumulative_split_factor" db:"cumulative_split_factor"`
	CumulativeDividendFac  float64    `json:"cumulative_dividend_factor" db:"cumulative_dividend_factor"`
	CumulativeTotalFactor  float64    `json:"cumulative_total_factor" db:"cumulative_total_factor"`
	NominalPriceFactor     float64    `json:"nominal_price_factor" db:"nominal_price_factor"`
	TotalReturnFactor      float64    `json:"total_return_factor" db:"total_return_factor"`
	VolumeAdjustmentFactor float64    `json:"volume_adjustment_factor" db:"volume_adjustment_factor"`
	PriceFactorApplicable  bool       `json:"price_factor_applicable" db:"price_factor_applicable"`
	TaxCostAllocationPct   *float64   `json:"tax_cost_allocation_pct,omitempty" db:"tax_cost_allocation_pct"`
	CumPriceReference      *float64   `json:"cum_price_reference,omitempty" db:"cum_price_reference"`
	ExPriceReference       *float64   `json:"ex_price_reference,omitempty" db:"ex_price_reference"`
	DisputeStatus          string     `json:"dispute_status" db:"dispute_status"`
	DisputeReason          *string    `json:"dispute_reason,omitempty" db:"dispute_reason"`
	SourceReference        string     `json:"source_reference" db:"source_reference"`
	RetrievalTimestamp     time.Time  `json:"retrieval_timestamp" db:"retrieval_timestamp"`
	RevisionNumber         int        `json:"revision_number" db:"revision_number"`
	IsVerified             bool       `json:"is_verified" db:"is_verified"`
	VerificationStatus     string     `json:"verification_status" db:"verification_status"`
	Details                any        `json:"details,omitempty" db:"details"`
	CreatedAt              time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at" db:"updated_at"`
}

// DataQualityLog records data quality checks.
type DataQualityLog struct {
	ID        int64             `json:"id" db:"id"`
	SymbolID  *int              `json:"symbol_id,omitempty" db:"symbol_id"`
	CheckType string            `json:"check_type" db:"check_type"`
	Status    DataQualityStatus `json:"status" db:"status"`
	Details   any               `json:"details,omitempty" db:"details"`
	Source    string            `json:"source,omitempty" db:"source"`
	CheckedAt time.Time         `json:"checked_at" db:"checked_at"`
}

// CorporateActionSourceArtifact stores immutable primary-source corporate action evidence.
type CorporateActionSourceArtifact struct {
	ID                   int        `json:"id" db:"id"`
	ArtifactID           string     `json:"artifact_id" db:"artifact_id"`
	IssuingAuthority     string     `json:"issuing_authority" db:"issuing_authority"`
	SourceURL            string     `json:"source_url" db:"source_url"`
	DocumentIdentifier   string     `json:"document_identifier" db:"document_identifier"`
	DocumentType         string     `json:"document_type" db:"document_type"`
	PublicationTimestamp *time.Time `json:"publication_timestamp,omitempty" db:"publication_timestamp"`
	RetrievalTimestamp   time.Time  `json:"retrieval_timestamp" db:"retrieval_timestamp"`
	RawPayload           string     `json:"raw_payload" db:"raw_payload"`
	SHA256Checksum       string     `json:"sha256_checksum" db:"sha256_checksum"`
	ParserVersion        string     `json:"parser_version" db:"parser_version"`
	ValidationStatus     string     `json:"validation_status" db:"validation_status"`
	Reviewer             *string    `json:"reviewer,omitempty" db:"reviewer"`
	ReviewedAt           *time.Time `json:"reviewed_at,omitempty" db:"reviewed_at"`
	CreatedAt            time.Time  `json:"created_at" db:"created_at"`
}

// CorporateActionEvent stores normalized event facts extracted from primary source artifacts.
type CorporateActionEvent struct {
	ID                      int        `json:"id" db:"id"`
	EventID                 string     `json:"event_id" db:"event_id"`
	SymbolID                int        `json:"symbol_id" db:"symbol_id"`
	Symbol                  string     `json:"symbol" db:"symbol"`
	SourceArtifactID        *int       `json:"source_artifact_id,omitempty" db:"source_artifact_id"`
	ActionType              string     `json:"action_type" db:"action_type"`
	SourceSubject           string     `json:"source_subject" db:"source_subject"`
	AnnouncementDate        *time.Time `json:"announcement_date,omitempty" db:"announcement_date"`
	ExDate                  time.Time  `json:"ex_date" db:"ex_date"`
	RecordDate              *time.Time `json:"record_date,omitempty" db:"record_date"`
	EffectiveDate           *time.Time `json:"effective_date,omitempty" db:"effective_date"`
	PaymentDate             *time.Time `json:"payment_date,omitempty" db:"payment_date"`
	Currency                string     `json:"currency" db:"currency"`
	DividendAmount          *float64   `json:"dividend_amount,omitempty" db:"dividend_amount"`
	SplitRatioOld           *float64   `json:"split_ratio_old,omitempty" db:"split_ratio_old"`
	SplitRatioNew           *float64   `json:"split_ratio_new,omitempty" db:"split_ratio_new"`
	BonusRatioBonus         *float64   `json:"bonus_ratio_bonus,omitempty" db:"bonus_ratio_bonus"`
	BonusRatioHeld          *float64   `json:"bonus_ratio_held,omitempty" db:"bonus_ratio_held"`
	RightsSubscriptionPrice *float64   `json:"rights_subscription_price,omitempty" db:"rights_subscription_price"`
	RightsRatioNew          *float64   `json:"rights_ratio_new,omitempty" db:"rights_ratio_new"`
	RightsRatioOld          *float64   `json:"rights_ratio_old,omitempty" db:"rights_ratio_old"`
	TaxCostAllocationPct    *float64   `json:"tax_cost_allocation_pct,omitempty" db:"tax_cost_allocation_pct"`
	VerificationStatus      string     `json:"verification_status" db:"verification_status"`
	CoverageStatus          string     `json:"coverage_status" db:"coverage_status"`
	Details                 any        `json:"details,omitempty" db:"details"`
	RawPayload              any        `json:"raw_payload,omitempty" db:"raw_payload"`
	CreatedAt               time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at" db:"updated_at"`
}

