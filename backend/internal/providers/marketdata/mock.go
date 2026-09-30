package marketdata

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// MockProvider implements MarketDataProvider with realistic sample data
// for Indian equities. Used for development and testing.
type MockProvider struct{}

// NewMockProvider creates a new mock provider.
func NewMockProvider() *MockProvider {
	return &MockProvider{}
}

func (m *MockProvider) Name() string {
	return "mock"
}

func (m *MockProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Symbols:          true,
		DailyOHLCV:       true,
		IntradayOHLCV:    true,
		CorporateActions: true,
		Indices:          true,
		Financials:       true,
		RealTimeQuotes:   false,
	}
}

// mockStocks contains sample Indian equity data for development.
var mockStocks = []SearchResult{
	{Symbol: "RELIANCE", Name: "Reliance Industries Ltd", Exchange: "NSE", ISIN: "INE002A01018", Sector: "Energy", Industry: "Oil & Gas Refining"},
	{Symbol: "TCS", Name: "Tata Consultancy Services Ltd", Exchange: "NSE", ISIN: "INE467B01029", Sector: "Technology", Industry: "IT Services"},
	{Symbol: "HDFCBANK", Name: "HDFC Bank Ltd", Exchange: "NSE", ISIN: "INE040A01034", Sector: "Financial Services", Industry: "Banking"},
	{Symbol: "INFY", Name: "Infosys Ltd", Exchange: "NSE", ISIN: "INE009A01021", Sector: "Technology", Industry: "IT Services"},
	{Symbol: "HINDUNILVR", Name: "Hindustan Unilever Ltd", Exchange: "NSE", ISIN: "INE030A01027", Sector: "FMCG", Industry: "Consumer Goods"},
	{Symbol: "ITC", Name: "ITC Ltd", Exchange: "NSE", ISIN: "INE154A01025", Sector: "FMCG", Industry: "Tobacco & FMCG"},
	{Symbol: "BHARTIARTL", Name: "Bharti Airtel Ltd", Exchange: "NSE", ISIN: "INE397D01024", Sector: "Telecom", Industry: "Telecom Services"},
	{Symbol: "WIPRO", Name: "Wipro Ltd", Exchange: "NSE", ISIN: "INE075A01022", Sector: "Technology", Industry: "IT Services"},
	{Symbol: "BAJFINANCE", Name: "Bajaj Finance Ltd", Exchange: "NSE", ISIN: "INE296A01024", Sector: "Financial Services", Industry: "NBFC"},
	{Symbol: "TATAMOTORS", Name: "Tata Motors Ltd", Exchange: "NSE", ISIN: "INE155A01022", Sector: "Automobile", Industry: "Auto Manufacturer"},
}

func (m *MockProvider) SearchSymbols(ctx context.Context, query string, exchange string) ([]SearchResult, error) {
	var results []SearchResult
	for _, s := range mockStocks {
		if exchange != "" && s.Exchange != exchange {
			continue
		}
		if containsIgnoreCase(s.Symbol, query) || containsIgnoreCase(s.Name, query) {
			results = append(results, s)
		}
	}
	return results, nil
}

func (m *MockProvider) GetSymbolInfo(ctx context.Context, symbol string, exchange string) (*DataPoint[SearchResult], error) {
	for _, s := range mockStocks {
		if s.Symbol == symbol {
			return &DataPoint[SearchResult]{
				Data:          s,
				Source:        "mock",
				RetrievedAt:   time.Now().UTC(),
				QualityStatus: models.DataQualityValid,
			}, nil
		}
	}
	return nil, fmt.Errorf("symbol %s not found", symbol)
}

func (m *MockProvider) GetDailyCandles(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*DataPoint[[]models.OHLCV], error) {
	basePrice := getBasePrice(symbol)
	var candles []models.OHLCV

	current := from
	price := basePrice
	rng := rand.New(rand.NewSource(hashSymbol(symbol)))

	for current.Before(to) || current.Equal(to) {
		// Skip weekends
		if current.Weekday() == time.Saturday || current.Weekday() == time.Sunday {
			current = current.AddDate(0, 0, 1)
			continue
		}

		// Generate realistic price movement
		change := (rng.Float64() - 0.48) * 0.03 * price // slight upward bias
		price += change
		if price < 1 {
			price = basePrice * 0.5
		}

		high := price + rng.Float64()*0.02*price
		low := price - rng.Float64()*0.02*price
		open := low + rng.Float64()*(high-low)
		closePrice := low + rng.Float64()*(high-low)
		volume := int64(100000 + rng.Intn(5000000))

		candles = append(candles, models.OHLCV{
			Timestamp:     current,
			Open:          math.Round(open*100) / 100,
			High:          math.Round(high*100) / 100,
			Low:           math.Round(low*100) / 100,
			Close:         math.Round(closePrice*100) / 100,
			Volume:        volume,
			Source:        "mock",
			RetrievedAt:   time.Now().UTC(),
			QualityStatus: models.DataQualityValid,
		})

		price = closePrice
		current = current.AddDate(0, 0, 1)
	}

	return &DataPoint[[]models.OHLCV]{
		Data:          candles,
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		Period:        "daily",
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (m *MockProvider) GetIntradayCandles(ctx context.Context, symbol string, exchange string, interval CandleInterval, from time.Time, to time.Time) (*DataPoint[[]models.OHLCV], error) {
	// Generate intraday candles (simplified)
	return &DataPoint[[]models.OHLCV]{
		Data:          []models.OHLCV{},
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		Period:        string(interval),
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (m *MockProvider) GetCorporateActions(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*DataPoint[[]models.CorporateAction], error) {
	return &DataPoint[[]models.CorporateAction]{
		Data:          []models.CorporateAction{},
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (m *MockProvider) GetIndexConstituents(ctx context.Context, indexName string) (*DataPoint[[]IndexConstituent], error) {
	constituents := make([]IndexConstituent, len(mockStocks))
	for i, s := range mockStocks {
		constituents[i] = IndexConstituent{
			Symbol: s.Symbol,
			Name:   s.Name,
			Weight: 10.0,
		}
	}
	return &DataPoint[[]IndexConstituent]{
		Data:          constituents,
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (m *MockProvider) GetFinancialStatements(ctx context.Context, symbol string, exchange string, periodType string) (*DataPoint[[]models.FinancialStatement], error) {
	baseRevenue := getBasePrice(symbol) * 1000000
	rng := rand.New(rand.NewSource(hashSymbol(symbol)))

	var financials []models.FinancialStatement
	for year := 2020; year <= 2024; year++ {
		revenue := baseRevenue * (1 + 0.1*float64(year-2020) + rng.Float64()*0.05)
		ebitda := revenue * (0.2 + rng.Float64()*0.1)
		pat := ebitda * (0.5 + rng.Float64()*0.2)
		eps := pat / 1000000
		totalDebt := revenue * (0.1 + rng.Float64()*0.3)
		totalEquity := revenue * (0.5 + rng.Float64()*0.3)
		ocf := pat * (1.1 + rng.Float64()*0.3)
		fcf := ocf - revenue*0.05
		intExp := totalDebt * 0.08
		intInc := revenue * 0.01
		cash := revenue * 0.1
		recv := revenue * 0.15
		shares := int64(1000000)

		periodEnd := time.Date(year, 3, 31, 0, 0, 0, 0, time.UTC)
		pubDate := periodEnd.AddDate(0, 0, 45)

		financials = append(financials, models.FinancialStatement{
			PeriodEnd:          periodEnd,
			PublicationDate:    &pubDate,
			PeriodType:         models.FinancialPeriodType(periodType),
			Revenue:            &revenue,
			EBITDA:             &ebitda,
			PAT:                &pat,
			EPS:                &eps,
			TotalDebt:          &totalDebt,
			TotalEquity:        &totalEquity,
			TotalAssets:        ptr(totalDebt + totalEquity),
			OperatingCashFlow:  &ocf,
			FreeCashFlow:       &fcf,
			InterestExpense:    &intExp,
			InterestIncome:     &intInc,
			Cash:               &cash,
			TotalReceivables:   &recv,
			SharesOutstanding:  &shares,
			Source:             "mock",
			RetrievedAt:        time.Now().UTC(),
			QualityStatus:      models.DataQualityValid,
		})
	}

	return &DataPoint[[]models.FinancialStatement]{
		Data:          financials,
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (m *MockProvider) GetShareholding(ctx context.Context, symbol string, exchange string) (*DataPoint[[]models.ShareholdingPattern], error) {
	return &DataPoint[[]models.ShareholdingPattern]{
		Data:          []models.ShareholdingPattern{},
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityMissing,
	}, nil
}

func (m *MockProvider) GetUpcomingIPOs(ctx context.Context) (*DataPoint[[]models.IPO], error) {
	return &DataPoint[[]models.IPO]{
		Data:          []models.IPO{},
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityMissing,
	}, nil
}

func (m *MockProvider) GetIPODetails(ctx context.Context, ipoID string) (*DataPoint[models.IPO], error) {
	return nil, fmt.Errorf("mock IPO details not implemented")
}

func (m *MockProvider) GetIPOSubscriptions(ctx context.Context, ipoID string) (*DataPoint[[]models.IPOSubscription], error) {
	return nil, fmt.Errorf("mock IPO subscriptions not implemented")
}

func (m *MockProvider) GetGMP(ctx context.Context, ipoID string) (*DataPoint[models.IPOGMP], error) {
	return nil, fmt.Errorf("mock GMP not implemented")
}

func (m *MockProvider) GetCurrentPrice(ctx context.Context, symbol string, exchange string) (*DataPoint[float64], error) {
	price := getBasePrice(symbol)
	return &DataPoint[float64]{
		Data:          price,
		Source:        "mock",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

// Helper functions

func getBasePrice(symbol string) float64 {
	prices := map[string]float64{
		"RELIANCE":   2450.0,
		"TCS":        3800.0,
		"HDFCBANK":   1650.0,
		"INFY":       1520.0,
		"HINDUNILVR": 2380.0,
		"ITC":        440.0,
		"BHARTIARTL": 1180.0,
		"WIPRO":      480.0,
		"BAJFINANCE": 7200.0,
		"TATAMOTORS": 680.0,
	}
	if p, ok := prices[symbol]; ok {
		return p
	}
	return 500.0
}

func hashSymbol(symbol string) int64 {
	var h int64
	for _, c := range symbol {
		h = h*31 + int64(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			sc := s[i+j]
			qc := substr[j]
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if qc >= 'A' && qc <= 'Z' {
				qc += 32
			}
			if sc != qc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func ptr(f float64) *float64 {
	return &f
}
