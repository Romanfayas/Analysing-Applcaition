// Package marketdata defines the MarketDataProvider interface.
// All external data sources must implement this interface.
// The application NEVER hardcodes to one provider.
package marketdata

import (
	"context"
	"errors"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

var (
	ErrNoData       = errors.New("DATA_UNAVAILABLE")
	ErrStaleData    = errors.New("STALE_DATA")
	ErrRateLimited  = errors.New("RATE_LIMITED")
	ErrProviderFail = errors.New("PROVIDER_ERROR")
)

// DataPoint wraps any data value with mandatory provenance metadata.
// Every data point must record: source, retrieval timestamp, period, data quality status.
type DataPoint[T any] struct {
	Data          T                       `json:"data"`
	Source        string                  `json:"source"`
	RetrievedAt   time.Time               `json:"retrieved_at"`
	Period        string                  `json:"period,omitempty"`
	QualityStatus models.DataQualityStatus `json:"quality_status"`
}

// CandleInterval represents the timeframe for candle data.
type CandleInterval string

const (
	Interval1Min  CandleInterval = "1m"
	Interval5Min  CandleInterval = "5m"
	Interval15Min CandleInterval = "15m"
	Interval30Min CandleInterval = "30m"
	Interval1Hour CandleInterval = "1h"
	IntervalDaily CandleInterval = "1d"
	IntervalWeek  CandleInterval = "1w"
	IntervalMonth CandleInterval = "1M"
)

// SearchResult represents a symbol search result from a provider.
type SearchResult struct {
	Symbol   string `json:"symbol"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
	ISIN     string `json:"isin,omitempty"`
	Sector   string `json:"sector,omitempty"`
	Industry string `json:"industry,omitempty"`
}

// IndexConstituent represents a stock in a market index.
type IndexConstituent struct {
	Symbol string  `json:"symbol"`
	Name   string  `json:"name"`
	Weight float64 `json:"weight,omitempty"`
}

// FinancialData represents financial statement data from a provider.
type FinancialData struct {
	PeriodEnd        time.Time `json:"period_end"`
	PeriodType       string    `json:"period_type"`
	Revenue          *float64  `json:"revenue,omitempty"`
	EBITDA           *float64  `json:"ebitda,omitempty"`
	PAT              *float64  `json:"pat,omitempty"`
	EPS              *float64  `json:"eps,omitempty"`
	TotalDebt        *float64  `json:"total_debt,omitempty"`
	TotalEquity      *float64  `json:"total_equity,omitempty"`
	TotalAssets      *float64  `json:"total_assets,omitempty"`
	OperatingCashFlow *float64 `json:"operating_cash_flow,omitempty"`
	FreeCashFlow     *float64  `json:"free_cash_flow,omitempty"`
	InterestExpense  *float64  `json:"interest_expense,omitempty"`
	InterestIncome   *float64  `json:"interest_income,omitempty"`
	Cash             *float64  `json:"cash,omitempty"`
	Receivables      *float64  `json:"receivables,omitempty"`
	SharesOutstanding *int64   `json:"shares_outstanding,omitempty"`
}

// MarketDataProvider is the interface that all data providers must implement.
// It provides methods to fetch market data from external sources.
//
// Implementation guidelines:
// - Return nil data with an appropriate error if data is unavailable
// - NEVER silently replace missing data with zero values
// - Always populate Source and RetrievedAt in DataPoint
// - Handle rate limits and retries internally
// - Validate data before returning (e.g., High >= Low for OHLCV)
type MarketDataProvider interface {
	Name() string
	Capabilities() ProviderCapabilities

	// SearchSymbols searches for symbols matching the query.
	SearchSymbols(ctx context.Context, query string, exchange string) ([]SearchResult, error)

	// GetSymbolInfo retrieves detailed information for a symbol.
	GetSymbolInfo(ctx context.Context, symbol string, exchange string) (*DataPoint[SearchResult], error)

	// GetDailyCandles retrieves daily OHLCV data for a date range.
	GetDailyCandles(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*DataPoint[[]models.OHLCV], error)

	// GetIntradayCandles retrieves intraday OHLCV data.
	GetIntradayCandles(ctx context.Context, symbol string, exchange string, interval CandleInterval, from time.Time, to time.Time) (*DataPoint[[]models.OHLCV], error)

	// GetIndexConstituents retrieves constituents of a market index.
	GetIndexConstituents(ctx context.Context, indexName string) (*DataPoint[[]IndexConstituent], error)

	// GetCurrentPrice retrieves the latest price for a symbol.
	GetCurrentPrice(ctx context.Context, symbol string, exchange string) (*DataPoint[float64], error)
}

// FundamentalDataProvider defines methods for fundamental data ingestion.
type FundamentalDataProvider interface {
	Name() string
	GetFinancialStatements(ctx context.Context, symbol string, exchange string, periodType string) (*DataPoint[[]models.FinancialStatement], error)
}

// CorporateActionsProvider defines methods for corporate actions ingestion.
type CorporateActionsProvider interface {
	Name() string
	GetCorporateActions(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*DataPoint[[]models.CorporateAction], error)
}

// ShareholdingProvider defines methods for shareholding ingestion.
type ShareholdingProvider interface {
	Name() string
	GetShareholding(ctx context.Context, symbol string, exchange string) (*DataPoint[[]models.ShareholdingPattern], error)
}

// IPODataProvider defines methods for IPO data ingestion.
type IPODataProvider interface {
	Name() string
	GetUpcomingIPOs(ctx context.Context) (*DataPoint[[]models.IPO], error)
	GetIPODetails(ctx context.Context, ipoID string) (*DataPoint[models.IPO], error)
	GetIPOSubscriptions(ctx context.Context, ipoID string) (*DataPoint[[]models.IPOSubscription], error)
}

// GMPDataProvider defines methods for Grey Market Premium ingestion.
type GMPDataProvider interface {
	Name() string
	GetGMP(ctx context.Context, ipoID string) (*DataPoint[models.IPOGMP], error)
}

// BenchmarkProvider defines methods for benchmark ingestion.
type BenchmarkProvider interface {
	Name() string
	GetBenchmarkOHLCV(ctx context.Context, symbol string, from time.Time, to time.Time) (*DataPoint[[]models.OHLCV], error)
}

// ProviderCapabilities indicates which features a provider supports.
type ProviderCapabilities struct {
	Symbols          bool `json:"symbols"`
	DailyOHLCV       bool `json:"daily_ohlcv"`
	IntradayOHLCV    bool `json:"intraday_ohlcv"`
	CorporateActions bool `json:"corporate_actions"`
	Indices          bool `json:"indices"`
	Financials       bool `json:"financials"`
	Shareholding     bool `json:"shareholding"`
	IPOData          bool `json:"ipo_data"`
	GMPData          bool `json:"gmp_data"`
	RealTimeQuotes   bool `json:"real_time_quotes"`
}

// ProviderRegistry manages multiple data providers and selects the best
// one for each data request based on capabilities and priority.
type ProviderRegistry struct {
	providers map[string]MarketDataProvider
	priority  []string // ordered list of provider names by priority
}

// NewProviderRegistry creates a new provider registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]MarketDataProvider),
		priority:  make([]string, 0),
	}
}

// Register adds a provider to the registry.
func (r *ProviderRegistry) Register(provider MarketDataProvider, priority int) {
	name := provider.Name()
	r.providers[name] = provider

	// Insert at correct priority position
	if priority >= len(r.priority) {
		r.priority = append(r.priority, name)
	} else {
		r.priority = append(r.priority[:priority+1], r.priority[priority:]...)
		r.priority[priority] = name
	}
}

// Get returns a specific provider by name.
func (r *ProviderRegistry) Get(name string) (MarketDataProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// GetByCapability returns the highest-priority provider supporting a capability.
func (r *ProviderRegistry) GetByCapability(capability string) MarketDataProvider {
	for _, name := range r.priority {
		p := r.providers[name]
		caps := p.Capabilities()
		switch capability {
		case "daily_ohlcv":
			if caps.DailyOHLCV {
				return p
			}
		case "intraday_ohlcv":
			if caps.IntradayOHLCV {
				return p
			}
		case "financials":
			if caps.Financials {
				return p
			}
		case "corporate_actions":
			if caps.CorporateActions {
				return p
			}
		case "indices":
			if caps.Indices {
				return p
			}
		case "symbols":
			if caps.Symbols {
				return p
			}
		}
	}
	return nil
}

// All returns all registered providers.
func (r *ProviderRegistry) All() map[string]MarketDataProvider {
	return r.providers
}
