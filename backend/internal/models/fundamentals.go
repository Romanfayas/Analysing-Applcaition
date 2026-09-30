package models

import "time"

// ==================================================
// Financial Statement & Fundamental Models
// ==================================================

const (
	PITExact       = "PIT_EXACT"
	PITEstimated   = "PIT_ESTIMATED"
	PITUnavailable = "PIT_UNAVAILABLE"
)

// FinancialStatement stores raw financial data for a company.
// FinancialStatement represents a single financial period's data.
type FinancialStatement struct {
	ID                int                 `json:"id" db:"id"`
	SymbolID          int                 `json:"symbol_id" db:"symbol_id"`
	PeriodType        FinancialPeriodType `json:"period_type" db:"period_type"`
	PeriodEnd         time.Time           `json:"period_end" db:"period_end"`
	PublicationDate   *time.Time          `json:"publication_date,omitempty" db:"publication_date"`
	PITMode           string              `json:"pit_mode" db:"pit_mode"` // PIT_EXACT, PIT_ESTIMATED, PIT_UNAVAILABLE
	FiscalYear        *int                `json:"fiscal_year,omitempty" db:"fiscal_year"`
	// Income Statement
	Revenue           *float64 `json:"revenue,omitempty" db:"revenue"`
	RevenueGrowth     *float64 `json:"revenue_growth,omitempty" db:"revenue_growth"`
	COGS              *float64 `json:"cost_of_goods_sold,omitempty" db:"cost_of_goods_sold"`
	GrossProfit       *float64 `json:"gross_profit,omitempty" db:"gross_profit"`
	OperatingExpenses *float64 `json:"operating_expenses,omitempty" db:"operating_expenses"`
	EBITDA            *float64 `json:"ebitda,omitempty" db:"ebitda"`
	EBITDAMargin      *float64 `json:"ebitda_margin,omitempty" db:"ebitda_margin"`
	Depreciation      *float64 `json:"depreciation,omitempty" db:"depreciation"`
	EBIT              *float64 `json:"ebit,omitempty" db:"ebit"`
	InterestExpense   *float64 `json:"interest_expense,omitempty" db:"interest_expense"`
	InterestIncome    *float64 `json:"interest_income,omitempty" db:"interest_income"`
	OtherIncome       *float64 `json:"other_income,omitempty" db:"other_income"`
	ExceptionalItems  *float64 `json:"exceptional_items,omitempty" db:"exceptional_items"`
	ProfitBeforeTax   *float64 `json:"profit_before_tax,omitempty" db:"profit_before_tax"`
	TaxExpense        *float64 `json:"tax_expense,omitempty" db:"tax_expense"`
	PAT               *float64 `json:"pat,omitempty" db:"pat"`
	PATGrowth         *float64 `json:"pat_growth,omitempty" db:"pat_growth"`
	// Balance Sheet
	TotalAssets       *float64 `json:"total_assets,omitempty" db:"total_assets"`
	TotalLiabilities  *float64 `json:"total_liabilities,omitempty" db:"total_liabilities"`
	TotalEquity       *float64 `json:"total_equity,omitempty" db:"total_equity"`
	TotalDebt         *float64 `json:"total_debt,omitempty" db:"total_debt"`
	LongTermDebt      *float64 `json:"long_term_debt,omitempty" db:"long_term_debt"`
	ShortTermDebt     *float64 `json:"short_term_debt,omitempty" db:"short_term_debt"`
	Cash              *float64 `json:"cash_and_equivalents,omitempty" db:"cash_and_equivalents"`
	TotalReceivables  *float64 `json:"total_receivables,omitempty" db:"total_receivables"`
	Inventory         *float64 `json:"inventory,omitempty" db:"inventory"`
	// Cash Flow
	OperatingCashFlow *float64 `json:"operating_cash_flow,omitempty" db:"operating_cash_flow"`
	InvestingCashFlow *float64 `json:"investing_cash_flow,omitempty" db:"investing_cash_flow"`
	FinancingCashFlow *float64 `json:"financing_cash_flow,omitempty" db:"financing_cash_flow"`
	CapEx             *float64 `json:"capex,omitempty" db:"capex"`
	FreeCashFlow      *float64 `json:"free_cash_flow,omitempty" db:"free_cash_flow"`
	// Per Share
	EPS               *float64 `json:"eps,omitempty" db:"eps"`
	EPSGrowth         *float64 `json:"eps_growth,omitempty" db:"eps_growth"`
	BookValuePerShare *float64 `json:"book_value_per_share,omitempty" db:"book_value_per_share"`
	DividendPerShare  *float64 `json:"dividend_per_share,omitempty" db:"dividend_per_share"`
	SharesOutstanding *int64   `json:"shares_outstanding,omitempty" db:"shares_outstanding"`
	// Meta
	Source        string            `json:"source" db:"source"`
	RetrievedAt   time.Time         `json:"retrieved_at" db:"retrieved_at"`
	QualityStatus DataQualityStatus `json:"quality_status" db:"quality_status"`
	CreatedAt     time.Time         `json:"created_at" db:"created_at"`
}

// FinancialRatio stores calculated financial ratios.
type FinancialRatio struct {
	ID                    int                 `json:"id" db:"id"`
	SymbolID              int                 `json:"symbol_id" db:"symbol_id"`
	PeriodEnd             time.Time           `json:"period_end" db:"period_end"`
	PeriodType            FinancialPeriodType `json:"period_type" db:"period_type"`
	ROE                   *float64            `json:"roe,omitempty" db:"roe"`
	ROCE                  *float64            `json:"roce,omitempty" db:"roce"`
	GrossMargin           *float64            `json:"gross_margin,omitempty" db:"gross_margin"`
	OperatingMargin       *float64            `json:"operating_margin,omitempty" db:"operating_margin"`
	NetMargin             *float64            `json:"net_margin,omitempty" db:"net_margin"`
	DebtToEquity          *float64            `json:"debt_to_equity,omitempty" db:"debt_to_equity"`
	InterestCoverage      *float64            `json:"interest_coverage,omitempty" db:"interest_coverage"`
	CurrentRatio          *float64            `json:"current_ratio,omitempty" db:"current_ratio"`
	PERatio               *float64            `json:"pe_ratio,omitempty" db:"pe_ratio"`
	PBRatio               *float64            `json:"pb_ratio,omitempty" db:"pb_ratio"`
	PEGRatio              *float64            `json:"peg_ratio,omitempty" db:"peg_ratio"`
	EVToEBITDA            *float64            `json:"ev_to_ebitda,omitempty" db:"ev_to_ebitda"`
	MarketCapToSales      *float64            `json:"market_cap_to_sales,omitempty" db:"market_cap_to_sales"`
	DividendYield         *float64            `json:"dividend_yield,omitempty" db:"dividend_yield"`
	MarketCap             *float64            `json:"market_cap,omitempty" db:"market_cap"`
	EnterpriseValue       *float64            `json:"enterprise_value,omitempty" db:"enterprise_value"`
	HasExceptionalIncome  bool                `json:"has_exceptional_income" db:"has_exceptional_income"`
	HasOneTimeGains       bool                `json:"has_one_time_gains" db:"has_one_time_gains"`
	DeterioratingCashFlow bool                `json:"deteriorating_cash_flow" db:"deteriorating_cash_flow"`
	RisingDebt            bool                `json:"rising_debt" db:"rising_debt"`
	MarginCompression     bool                `json:"margin_compression" db:"margin_compression"`
	CalculatedAt          time.Time           `json:"calculated_at" db:"calculated_at"`
}

// ShareholdingPattern stores promoter, FII, DII holdings.
type ShareholdingPattern struct {
	ID                   int       `json:"id" db:"id"`
	SymbolID             int       `json:"symbol_id" db:"symbol_id"`
	PeriodEnd            time.Time `json:"period_end" db:"period_end"`
	PromoterHolding      *float64  `json:"promoter_holding,omitempty" db:"promoter_holding"`
	PromoterPledge       *float64  `json:"promoter_pledge,omitempty" db:"promoter_pledge"`
	FIIHolding           *float64  `json:"fii_holding,omitempty" db:"fii_holding"`
	DIIHolding           *float64  `json:"dii_holding,omitempty" db:"dii_holding"`
	PublicHolding        *float64  `json:"public_holding,omitempty" db:"public_holding"`
	PromoterPledgeChange *float64  `json:"promoter_pledge_change,omitempty" db:"promoter_pledge_change"`
	Source               string    `json:"source" db:"source"`
	CreatedAt            time.Time `json:"created_at" db:"created_at"`
}

// FundamentalAnomaly records detected anomalies in financial data.
type FundamentalAnomaly struct {
	ID               int       `json:"id" db:"id"`
	SymbolID         int       `json:"symbol_id" db:"symbol_id"`
	PeriodEnd        time.Time `json:"period_end" db:"period_end"`
	AnomalyType      string    `json:"anomaly_type" db:"anomaly_type"`
	Severity         string    `json:"severity" db:"severity"`
	Description      string    `json:"description" db:"description"`
	MetricName       string    `json:"metric_name,omitempty" db:"metric_name"`
	MetricValue      *float64  `json:"metric_value,omitempty" db:"metric_value"`
	ExpectedRangeLow *float64  `json:"expected_range_low,omitempty" db:"expected_range_low"`
	ExpectedRangeHigh *float64 `json:"expected_range_high,omitempty" db:"expected_range_high"`
	DetectedAt       time.Time `json:"detected_at" db:"detected_at"`
}
