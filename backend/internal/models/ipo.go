package models

import "time"

// ==================================================
// IPO Models
// ==================================================

// IPO represents an Initial Public Offering with all associated data.
type IPO struct {
	ID                  int        `json:"id" db:"id"`
	CompanyName         string     `json:"company_name" db:"company_name"`
	Symbol              string     `json:"symbol,omitempty" db:"symbol"`
	Exchange            string     `json:"exchange" db:"exchange"`
	Sector              string     `json:"sector,omitempty" db:"sector"`
	Industry            string     `json:"industry,omitempty" db:"industry"`
	OpenDate            *time.Time `json:"open_date,omitempty" db:"open_date"`
	CloseDate           *time.Time `json:"close_date,omitempty" db:"close_date"`
	ListingDate         *time.Time `json:"listing_date,omitempty" db:"listing_date"`
	PriceBandLow        *float64   `json:"price_band_low,omitempty" db:"price_band_low"`
	PriceBandHigh       *float64   `json:"price_band_high,omitempty" db:"price_band_high"`
	IssuePrice          *float64   `json:"issue_price,omitempty" db:"issue_price"`
	ListingPrice        *float64   `json:"listing_price,omitempty" db:"listing_price"`
	ListingDayClose     *float64   `json:"listing_day_close,omitempty" db:"listing_day_close"`
	LotSize             *int       `json:"lot_size,omitempty" db:"lot_size"`
	MinInvestment       *float64   `json:"min_investment,omitempty" db:"min_investment"`
	IssueSize           *float64   `json:"issue_size,omitempty" db:"issue_size"`
	FreshIssue          *float64   `json:"fresh_issue,omitempty" db:"fresh_issue"`
	OFS                 *float64   `json:"ofs,omitempty" db:"ofs"`
	PreIPOShares        *int64     `json:"pre_ipo_shares,omitempty" db:"pre_ipo_shares"`
	PostIPOShares       *int64     `json:"post_ipo_shares,omitempty" db:"post_ipo_shares"`
	PromoterHoldingPre  *float64   `json:"promoter_holding_pre,omitempty" db:"promoter_holding_pre"`
	PromoterHoldingPost *float64   `json:"promoter_holding_post,omitempty" db:"promoter_holding_post"`
	MarketCapAtIssue    *float64   `json:"market_cap_at_issue,omitempty" db:"market_cap_at_issue"`
	Status              string     `json:"status" db:"status"`
	Source              string     `json:"source,omitempty" db:"source"`
	Metadata            any        `json:"metadata,omitempty" db:"metadata"`
	CreatedAt           time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at" db:"updated_at"`
}

// IPOFinancial stores financial data specific to an IPO.
type IPOFinancial struct {
	ID               int                 `json:"id" db:"id"`
	IPOID            int                 `json:"ipo_id" db:"ipo_id"`
	PeriodEnd        time.Time           `json:"period_end" db:"period_end"`
	PeriodType       FinancialPeriodType `json:"period_type" db:"period_type"`
	Revenue          *float64            `json:"revenue,omitempty" db:"revenue"`
	RevenueGrowth    *float64            `json:"revenue_growth,omitempty" db:"revenue_growth"`
	EBITDA           *float64            `json:"ebitda,omitempty" db:"ebitda"`
	EBITDAMargin     *float64            `json:"ebitda_margin,omitempty" db:"ebitda_margin"`
	PAT              *float64            `json:"pat,omitempty" db:"pat"`
	PATGrowth        *float64            `json:"pat_growth,omitempty" db:"pat_growth"`
	EPS              *float64            `json:"eps,omitempty" db:"eps"`
	ROE              *float64            `json:"roe,omitempty" db:"roe"`
	ROCE             *float64            `json:"roce,omitempty" db:"roce"`
	TotalDebt        *float64            `json:"total_debt,omitempty" db:"total_debt"`
	DebtToEquity     *float64            `json:"debt_to_equity,omitempty" db:"debt_to_equity"`
	OperatingCashFlow *float64           `json:"operating_cash_flow,omitempty" db:"operating_cash_flow"`
	FreeCashFlow     *float64            `json:"free_cash_flow,omitempty" db:"free_cash_flow"`
	Source           string              `json:"source,omitempty" db:"source"`
	CreatedAt        time.Time           `json:"created_at" db:"created_at"`
}

// IPOGMP stores Grey Market Premium tracking data.
// GMP is treated as unofficial market sentiment, NOT guaranteed listing price.
type IPOGMP struct {
	ID            int64     `json:"id" db:"id"`
	IPOID         int       `json:"ipo_id" db:"ipo_id"`
	GMPValue      float64   `json:"gmp_value" db:"gmp_value"`
	GMPPercentage *float64  `json:"gmp_percentage,omitempty" db:"gmp_percentage"`
	Source        string    `json:"source" db:"source"`
	SourceURL     string    `json:"source_url,omitempty" db:"source_url"`
	RecordedAt    time.Time `json:"recorded_at" db:"recorded_at"`
}

// IPOSubscription stores subscription data during an IPO.
type IPOSubscription struct {
	ID                   int       `json:"id" db:"id"`
	IPOID                int       `json:"ipo_id" db:"ipo_id"`
	DayNumber            int       `json:"day_number" db:"day_number"`
	RecordedAt           time.Time `json:"recorded_at" db:"recorded_at"`
	QIBSubscription      *float64  `json:"qib_subscription,omitempty" db:"qib_subscription"`
	NIISubscription      *float64  `json:"nii_subscription,omitempty" db:"nii_subscription"`
	RetailSubscription   *float64  `json:"retail_subscription,omitempty" db:"retail_subscription"`
	EmployeeSubscription *float64  `json:"employee_subscription,omitempty" db:"employee_subscription"`
	TotalSubscription    *float64  `json:"total_subscription,omitempty" db:"total_subscription"`
	Source               string    `json:"source,omitempty" db:"source"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
}

// IPOPrediction stores listing price predictions from different models.
// Predictions are NEVER described as guaranteed.
type IPOPrediction struct {
	ID                int       `json:"id" db:"id"`
	IPOID             int       `json:"ipo_id" db:"ipo_id"`
	ModelName         string    `json:"model_name" db:"model_name"`
	PredictedPrice    float64   `json:"predicted_price" db:"predicted_price"`
	PredictedReturn   *float64  `json:"predicted_return,omitempty" db:"predicted_return"`
	BullCase          *float64  `json:"bull_case,omitempty" db:"bull_case"`
	BaseCase          *float64  `json:"base_case,omitempty" db:"base_case"`
	BearCase          *float64  `json:"bear_case,omitempty" db:"bear_case"`
	ConfidenceScore   *float64  `json:"confidence_score,omitempty" db:"confidence_score"`
	PredictedRangeLow *float64  `json:"predicted_range_low,omitempty" db:"predicted_range_low"`
	PredictedRangeHigh *float64 `json:"predicted_range_high,omitempty" db:"predicted_range_high"`
	PredictedMidpoint *float64  `json:"predicted_midpoint,omitempty" db:"predicted_midpoint"`
	Factors           any       `json:"factors,omitempty" db:"factors"`
	PredictedAt       time.Time `json:"predicted_at" db:"predicted_at"`
}

// IPODecisionResult stores the decision engine output for an IPO.
// Shariah failure results in AVOID; REVIEW_REQUIRED if data insufficient.
type IPODecisionResult struct {
	ID                int           `json:"id" db:"id"`
	IPOID             int           `json:"ipo_id" db:"ipo_id"`
	Decision          IPODecision   `json:"decision" db:"decision"`
	OverallScore      float64       `json:"overall_score" db:"overall_score"`
	FundamentalScore  *float64      `json:"fundamental_score,omitempty" db:"fundamental_score"`
	GrowthScore       *float64      `json:"growth_score,omitempty" db:"growth_score"`
	ProfitabilityScore *float64     `json:"profitability_score,omitempty" db:"profitability_score"`
	DebtScore         *float64      `json:"debt_score,omitempty" db:"debt_score"`
	ValuationScore    *float64      `json:"valuation_score,omitempty" db:"valuation_score"`
	GMPScore          *float64      `json:"gmp_score,omitempty" db:"gmp_score"`
	SubscriptionScore *float64      `json:"subscription_score,omitempty" db:"subscription_score"`
	MarketScore       *float64      `json:"market_score,omitempty" db:"market_score"`
	ShariahStatus     ShariahStatus `json:"shariah_status" db:"shariah_status"`
	Explanation       string        `json:"explanation,omitempty" db:"explanation"`
	Factors           any           `json:"factors,omitempty" db:"factors"`
	DecidedAt         time.Time     `json:"decided_at" db:"decided_at"`
}
