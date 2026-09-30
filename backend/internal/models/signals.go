package models

import "time"

// ==================================================
// Signal, Shariah, and Risk Models
// ==================================================

// ShariahRuleSet represents a named collection of Shariah screening rules.
type ShariahRuleSet struct {
	ID          int       `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description,omitempty" db:"description"`
	IsActive    bool      `json:"is_active" db:"is_active"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// ShariahRuleVersion stores a versioned set of screening parameters.
// Rules are stored as JSON configuration, NEVER hardcoded in business logic.
type ShariahRuleVersion struct {
	ID            int       `json:"id" db:"id"`
	RuleSetID     int       `json:"rule_set_id" db:"rule_set_id"`
	Version       int       `json:"version" db:"version"`
	Rules         any       `json:"rules" db:"rules"` // JSONB
	EffectiveFrom time.Time `json:"effective_from" db:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty" db:"effective_to"`
	Notes         string    `json:"notes,omitempty" db:"notes"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}

// ShariahScreening stores the result of screening a stock against Shariah rules.
type ShariahScreening struct {
	ID                  int           `json:"id" db:"id"`
	SymbolID            int           `json:"symbol_id" db:"symbol_id"`
	RuleSetID           int           `json:"rule_set_id" db:"rule_set_id"`
	RuleVersionID       int           `json:"rule_version_id" db:"rule_version_id"`
	ScreeningDate       time.Time     `json:"screening_date" db:"screening_date"`
	Status              ShariahStatus `json:"status" db:"status"`
	DebtRatio           *float64      `json:"debt_ratio,omitempty" db:"debt_ratio"`
	DebtRatioPass       *bool         `json:"debt_ratio_pass,omitempty" db:"debt_ratio_pass"`
	InterestIncomeRatio *float64      `json:"interest_income_ratio,omitempty" db:"interest_income_ratio"`
	InterestIncomePass  *bool         `json:"interest_income_pass,omitempty" db:"interest_income_pass"`
	CashDepositRatio    *float64      `json:"cash_deposit_ratio,omitempty" db:"cash_deposit_ratio"`
	CashDepositPass     *bool         `json:"cash_deposit_pass,omitempty" db:"cash_deposit_pass"`
	ReceivablesRatio    *float64      `json:"receivables_ratio,omitempty" db:"receivables_ratio"`
	ReceivablesPass     *bool         `json:"receivables_pass,omitempty" db:"receivables_pass"`
	BusinessActivityPass *bool        `json:"business_activity_pass,omitempty" db:"business_activity_pass"`
	BusinessActivityNotes string      `json:"business_activity_notes,omitempty" db:"business_activity_notes"`
	PurificationPerShare *float64     `json:"purification_per_share,omitempty" db:"purification_per_share"`
	PurificationMethod   string       `json:"purification_method,omitempty" db:"purification_method"`
	Details              any          `json:"details,omitempty" db:"details"`
	Source               string       `json:"source,omitempty" db:"source"`
	ScreenedAt           time.Time    `json:"screened_at" db:"screened_at"`
}

// StockSignal stores the composite signal for a stock.
// If Shariah status is FAIL, signal must be AVOID regardless of score.
type StockSignal struct {
	ID                int           `json:"id" db:"id"`
	SymbolID          int           `json:"symbol_id" db:"symbol_id"`
	Signal            SignalType    `json:"signal" db:"signal"`
	OverallScore      float64       `json:"overall_score" db:"overall_score"`
	FundamentalScore  *float64      `json:"fundamental_score,omitempty" db:"fundamental_score"`
	TechnicalScore    *float64      `json:"technical_score,omitempty" db:"technical_score"`
	CandlestickScore  *float64      `json:"candlestick_score,omitempty" db:"candlestick_score"`
	MomentumScore     *float64      `json:"momentum_score,omitempty" db:"momentum_score"`
	VolumeScore       *float64      `json:"volume_score,omitempty" db:"volume_score"`
	ValuationScore    *float64      `json:"valuation_score,omitempty" db:"valuation_score"`
	MarketRegimeScore *float64      `json:"market_regime_score,omitempty" db:"market_regime_score"`
	RiskScore         *float64      `json:"risk_score,omitempty" db:"risk_score"`
	QualityScore      *float64      `json:"quality_score,omitempty" db:"quality_score"`
	ShariahStatus     ShariahStatus `json:"shariah_status" db:"shariah_status"`
	ShariahOverride   bool          `json:"shariah_override" db:"shariah_override"`
	WeightConfigID    *int          `json:"weight_config_id,omitempty" db:"weight_config_id"`
	Explanation       string        `json:"explanation,omitempty" db:"explanation"`
	SupportingFactors any           `json:"supporting_factors,omitempty" db:"supporting_factors"`
	ConflictingFactors any          `json:"conflicting_factors,omitempty" db:"conflicting_factors"`
	InvalidationFactors any         `json:"invalidation_factors,omitempty" db:"invalidation_factors"`
	CalculatedAt      time.Time     `json:"calculated_at" db:"calculated_at"`
}

// RiskCalculation stores position sizing and risk calculations.
// Never recommends a position larger than available capital.
type RiskCalculation struct {
	ID                    int       `json:"id" db:"id"`
	SymbolID              int       `json:"symbol_id" db:"symbol_id"`
	Capital               float64   `json:"capital" db:"capital"`
	RiskPercentage        float64   `json:"risk_percentage" db:"risk_percentage"`
	MaxRiskAmount         float64   `json:"max_risk_amount" db:"max_risk_amount"`
	EntryPrice            float64   `json:"entry_price" db:"entry_price"`
	StopLoss              float64   `json:"stop_loss" db:"stop_loss"`
	TargetPrice           *float64  `json:"target_price,omitempty" db:"target_price"`
	RiskPerShare          float64   `json:"risk_per_share" db:"risk_per_share"`
	PositionSize          int       `json:"position_size" db:"position_size"`
	PositionValue         float64   `json:"position_value" db:"position_value"`
	RiskRewardRatio       *float64  `json:"risk_reward_ratio,omitempty" db:"risk_reward_ratio"`
	PortfolioExposure     *float64  `json:"portfolio_exposure,omitempty" db:"portfolio_exposure"`
	SectorExposure        *float64  `json:"sector_exposure,omitempty" db:"sector_exposure"`
	VolatilityAdjustedSize *int     `json:"volatility_adjusted_size,omitempty" db:"volatility_adjusted_size"`
	MaxDrawdown           *float64  `json:"max_drawdown,omitempty" db:"max_drawdown"`
	CalculatedAt          time.Time `json:"calculated_at" db:"calculated_at"`
}

// SignalWeightConfig stores configurable weights for the scoring system.
type SignalWeightConfig struct {
	ID        int       `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	IsActive  bool      `json:"is_active" db:"is_active"`
	Weights   any       `json:"weights" db:"weights"` // JSONB
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	UpdatedAt time.Time `json:"updated_at" db:"updated_at"`
}
