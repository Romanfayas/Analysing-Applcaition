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
	ID                 int       `json:"id" db:"id"`
	SymbolID           int       `json:"symbol_id" db:"symbol_id"`
	ActionType         string    `json:"action_type" db:"action_type"`
	ExDate             time.Time `json:"ex_date" db:"ex_date"`
	RecordDate         *time.Time `json:"record_date,omitempty" db:"record_date"`
	Description        string    `json:"description,omitempty" db:"description"`
	OldRatio           *float64  `json:"old_ratio,omitempty" db:"old_ratio"`
	NewRatio           *float64  `json:"new_ratio,omitempty" db:"new_ratio"`
	DividendAmount     *float64  `json:"dividend_amount,omitempty" db:"dividend_amount"`
	DividendPercentage *float64  `json:"dividend_percentage,omitempty" db:"dividend_percentage"`
	Source             string    `json:"source" db:"source"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
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
