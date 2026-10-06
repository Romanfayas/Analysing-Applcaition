package yahoofinance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"sync"
	"time"

	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/marketdata"
)

// Provider implements MarketDataProvider using Yahoo Finance (free, no API key).
// Yahoo Finance is the primary free data source for Indian equities.
// NSE symbols are accessed with ".NS" suffix, BSE with ".BO".
type Provider struct {
	client  *http.Client
	baseURL string
	crumb   string
	mu      sync.Mutex
}

// NewProvider creates a new Yahoo Finance provider.
func NewProvider() *Provider {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
	}

	return &Provider{
		client:  client,
		baseURL: "https://query1.finance.yahoo.com/v8/finance",
	}
}

func (p *Provider) getCrumb() (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.crumb != "" {
		return p.crumb, nil
	}

	// 1. Visit fc.yahoo.com to get cookies
	reqCookie, _ := http.NewRequest("GET", "https://fc.yahoo.com", nil)
	reqCookie.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	p.client.Do(reqCookie)

	// 2. Fetch crumb
	reqCrumb, _ := http.NewRequest("GET", "https://query2.finance.yahoo.com/v1/test/getcrumb", nil)
	reqCrumb.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	
	resp, err := p.client.Do(reqCrumb)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get crumb, status: %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	p.crumb = string(body)
	return p.crumb, nil
}

// Capabilities returns which data types this provider supports.
func (p *Provider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{
		Symbols:          true,
		DailyOHLCV:       true,
		IntradayOHLCV:    false,
		CorporateActions: false,
		Indices:          false,
		Financials:       true,
		Shareholding:     false,
		IPOData:          false,
		GMPData:          false,
		RealTimeQuotes:   true,
	}
}

// OHLCV represents a single OHLCV candle.
type OHLCV struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    int64     `json:"volume"`
	AdjClose  float64   `json:"adj_close"`
}

// Quote represents a real-time quote.
type Quote struct {
	Symbol             string  `json:"symbol"`
	RegularMarketPrice float64 `json:"regular_market_price"`
	RegularMarketOpen  float64 `json:"regular_market_open"`
	RegularMarketHigh  float64 `json:"regular_market_high"`
	RegularMarketLow   float64 `json:"regular_market_low"`
	RegularMarketVol   int64   `json:"regular_market_volume"`
	MarketCap          int64   `json:"market_cap"`
	FiftyTwoWeekHigh   float64 `json:"fifty_two_week_high"`
	FiftyTwoWeekLow    float64 `json:"fifty_two_week_low"`
	TrailingPE         float64 `json:"trailing_pe"`
	ForwardPE          float64 `json:"forward_pe"`
	PriceToBook        float64 `json:"price_to_book"`
	DividendYield      float64 `json:"dividend_yield"`
}

// chartResponse represents Yahoo Finance chart API response.
type chartResponse struct {
	Chart struct {
		Result []struct {
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Open   []float64 `json:"open"`
					High   []float64 `json:"high"`
					Low    []float64 `json:"low"`
					Close  []float64 `json:"close"`
					Volume []int64   `json:"volume"`
				} `json:"quote"`
				AdjClose []struct {
					AdjClose []float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// toNSESymbol converts a plain symbol to Yahoo Finance NSE format.
func toNSESymbol(symbol string) string {
	return symbol + ".NS"
}

// GetHistoricalData fetches OHLCV candles for a given symbol and date range.
// interval: "1d", "1wk", "1mo"
func (p *Provider) GetHistoricalData(symbol string, from, to time.Time, interval string) ([]OHLCV, error) {
	yahooSymbol := toNSESymbol(symbol)

	url := fmt.Sprintf(
		"%s/chart/%s?period1=%d&period2=%d&interval=%s&includeAdjustedClose=true",
		p.baseURL, yahooSymbol,
		from.Unix(), to.Unix(), interval,
	)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo finance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("yahoo finance returned %d: %s", resp.StatusCode, string(body))
	}

	var chart chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&chart); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if chart.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo finance error: %s - %s",
			chart.Chart.Error.Code, chart.Chart.Error.Description)
	}

	if len(chart.Chart.Result) == 0 {
		return nil, fmt.Errorf("no data returned for %s", symbol)
	}

	result := chart.Chart.Result[0]
	quotes := result.Indicators.Quote
	if len(quotes) == 0 {
		return nil, fmt.Errorf("no quote data for %s", symbol)
	}

	q := quotes[0]
	var candles []OHLCV

	for i, ts := range result.Timestamp {
		// Skip candles with missing data (Yahoo returns null for holidays)
		if i >= len(q.Open) || i >= len(q.Close) {
			continue
		}

		candle := OHLCV{
			Timestamp: time.Unix(ts, 0).UTC(),
			Open:      q.Open[i],
			High:      q.High[i],
			Low:       q.Low[i],
			Close:     q.Close[i],
		}

		if i < len(q.Volume) {
			candle.Volume = q.Volume[i]
		}

		// Adjusted close for corporate action handling
		if len(result.Indicators.AdjClose) > 0 && i < len(result.Indicators.AdjClose[0].AdjClose) {
			candle.AdjClose = result.Indicators.AdjClose[0].AdjClose[i]
		}

		// Validate candle — never insert zero or negative prices
		if candle.Open <= 0 || candle.Close <= 0 || candle.High <= 0 || candle.Low <= 0 {
			continue
		}

		candles = append(candles, candle)
	}

	return candles, nil
}

// GetDailyCandles implements MarketDataProvider
func (p *Provider) GetDailyCandles(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	candles, err := p.GetHistoricalData(symbol, from, to, "1d")
	if err != nil {
		return nil, err
	}
	var res []models.OHLCV
	for _, c := range candles {
		res = append(res, models.OHLCV{
			Timestamp:     c.Timestamp,
			Open:          c.Open,
			High:          c.High,
			Low:           c.Low,
			Close:         c.Close,
			Volume:        c.Volume,
			Source:        p.Name(),
			RetrievedAt:   time.Now().UTC(),
			QualityStatus: models.DataQualityValid,
		})
	}
	return &marketdata.DataPoint[[]models.OHLCV]{
		Data:          res,
		Source:        p.Name(),
		RetrievedAt:   time.Now().UTC(),
		Period:        "daily",
		QualityStatus: models.DataQualityValid,
	}, nil
}

// GetIntradayCandles implements MarketDataProvider
func (p *Provider) GetIntradayCandles(ctx context.Context, symbol string, exchange string, interval marketdata.CandleInterval, from time.Time, to time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	return nil, fmt.Errorf("intraday candles not officially supported without rate limit risks on free API")
}

// GetBenchmarkOHLCV implements BenchmarkProvider
func (p *Provider) GetBenchmarkOHLCV(ctx context.Context, symbol string, from time.Time, to time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	// e.g. ^NSEI
	candles, err := p.GetHistoricalData(symbol, from, to, "1d")
	if err != nil {
		return nil, err
	}
	var res []models.OHLCV
	for _, c := range candles {
		res = append(res, models.OHLCV{
			Timestamp:     c.Timestamp,
			Open:          c.Open,
			High:          c.High,
			Low:           c.Low,
			Close:         c.Close,
			Volume:        c.Volume,
			Source:        p.Name(),
			RetrievedAt:   time.Now().UTC(),
			QualityStatus: models.DataQualityValid,
		})
	}
	return &marketdata.DataPoint[[]models.OHLCV]{
		Data:          res,
		Source:        p.Name(),
		RetrievedAt:   time.Now().UTC(),
		Period:        "daily",
		QualityStatus: models.DataQualityValid,
	}, nil
}

// GetIndexConstituents implements MarketDataProvider
func (p *Provider) GetIndexConstituents(ctx context.Context, indexName string) (*marketdata.DataPoint[[]marketdata.IndexConstituent], error) {
	return nil, fmt.Errorf("index constituents not supported by Yahoo Finance")
}

// GetQuote fetches the current quote for a symbol.
func (p *Provider) GetQuote(symbol string) (*Quote, error) {
	yahooSymbol := toNSESymbol(symbol)

	url := fmt.Sprintf(
		"%s/chart/%s?interval=1d&range=1d",
		p.baseURL, yahooSymbol,
	)

	resp, err := p.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("yahoo finance quote request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance returned %d", resp.StatusCode)
	}

	var chart chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&chart); err != nil {
		return nil, fmt.Errorf("failed to decode quote response: %w", err)
	}

	if chart.Chart.Error != nil {
		return nil, fmt.Errorf("yahoo quote error: %s", chart.Chart.Error.Description)
	}

	if len(chart.Chart.Result) == 0 || len(chart.Chart.Result[0].Indicators.Quote) == 0 {
		return nil, fmt.Errorf("no quote data for %s", symbol)
	}

	result := chart.Chart.Result[0]
	q := result.Indicators.Quote[0]
	n := len(q.Close)

	if n == 0 {
		return nil, fmt.Errorf("empty quote for %s", symbol)
	}

	return &Quote{
		Symbol:             symbol,
		RegularMarketPrice: q.Close[n-1],
		RegularMarketOpen:  q.Open[n-1],
		RegularMarketHigh:  q.High[n-1],
		RegularMarketLow:   q.Low[n-1],
		RegularMarketVol:   q.Volume[n-1],
	}, nil
}

// SearchSymbol searches for Indian equity symbols.
func (p *Provider) SearchSymbol(query string) ([]map[string]string, error) {
	url := fmt.Sprintf(
		"https://query2.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=10&newsCount=0&enableFuzzyQuery=false",
		query,
	)

	resp, err := p.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("yahoo search failed: %w", err)
	}
	defer resp.Body.Close()

	var searchResp struct {
		Quotes []struct {
			Symbol   string `json:"symbol"`
			Name     string `json:"shortname"`
			Exchange string `json:"exchange"`
			Type     string `json:"quoteType"`
		} `json:"quotes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search: %w", err)
	}

	var results []map[string]string
	for _, q := range searchResp.Quotes {
		// Filter for Indian equities only
		if q.Exchange == "NSI" || q.Exchange == "BSE" {
			results = append(results, map[string]string{
				"symbol":   q.Symbol,
				"name":     q.Name,
				"exchange": q.Exchange,
				"type":     q.Type,
			})
		}
	}

	return results, nil
}

// SearchSymbols implements MarketDataProvider
func (p *Provider) SearchSymbols(ctx context.Context, query string, exchange string) ([]marketdata.SearchResult, error) {
	rawResults, err := p.SearchSymbol(query)
	if err != nil {
		return nil, err
	}
	var res []marketdata.SearchResult
	for _, r := range rawResults {
		if exchange != "" && r["exchange"] != exchange {
			continue
		}
		res = append(res, marketdata.SearchResult{
			Symbol:   r["symbol"],
			Name:     r["name"],
			Exchange: r["exchange"],
		})
	}
	return res, nil
}

// GetSymbolInfo implements MarketDataProvider
func (p *Provider) GetSymbolInfo(ctx context.Context, symbol string, exchange string) (*marketdata.DataPoint[marketdata.SearchResult], error) {
	return nil, fmt.Errorf("detailed symbol info not natively supported without multiple requests")
}

// GetCurrentPrice implements MarketDataProvider
func (p *Provider) GetCurrentPrice(ctx context.Context, symbol string, exchange string) (*marketdata.DataPoint[float64], error) {
	q, err := p.GetQuote(symbol)
	if err != nil {
		return nil, err
	}
	return &marketdata.DataPoint[float64]{
		Data:          q.RegularMarketPrice,
		Source:        p.Name(),
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

// GetFinancialStatements implements FundamentalDataProvider
func (p *Provider) GetFinancialStatements(ctx context.Context, symbol string, exchange string, periodType string) (*marketdata.DataPoint[[]models.FinancialStatement], error) {
	crumb, err := p.getCrumb()
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve yahoo crumb: %w", err)
	}

	yahooSymbol := toNSESymbol(symbol)
	url := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=incomeStatementHistory,balanceSheetHistory,cashflowStatementHistory&crumb=%s", yahooSymbol, crumb)
	
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo finance fundamentals failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		// Crumb expired, clear it
		p.mu.Lock()
		p.crumb = ""
		p.mu.Unlock()
		return nil, fmt.Errorf("yahoo finance auth error (crumb expired)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo finance returned %d", resp.StatusCode)
	}
	
	body, _ := io.ReadAll(resp.Body)
	
	// Quick parse to avoid complex typed structs for quoteSummary
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("failed to decode fundamentals: %w", err)
	}
	
	// Safe navigate nested map
	quoteSummary, ok := parsed["quoteSummary"].(map[string]interface{})
	if !ok || quoteSummary["result"] == nil {
		return nil, marketdata.ErrNoData
	}
	
	results, ok := quoteSummary["result"].([]interface{})
	if !ok || len(results) == 0 {
		return nil, marketdata.ErrNoData
	}
	
	res := results[0].(map[string]interface{})
	
	// Extract sections
	var incomeStmts []interface{}
	if isH, ok := res["incomeStatementHistory"].(map[string]interface{}); ok {
		if stmts, ok := isH["incomeStatementHistory"].([]interface{}); ok {
			incomeStmts = stmts
		}
	}
	
	var balSheets []interface{}
	if bsH, ok := res["balanceSheetHistory"].(map[string]interface{}); ok {
		if stmts, ok := bsH["balanceSheetStatements"].([]interface{}); ok {
			balSheets = stmts
		}
	}
	
	var cashFlows []interface{}
	if cfH, ok := res["cashflowStatementHistory"].(map[string]interface{}); ok {
		if stmts, ok := cfH["cashflowStatements"].([]interface{}); ok {
			cashFlows = stmts
		}
	}
	
	if len(incomeStmts) == 0 {
		return nil, marketdata.ErrNoData
	}
	
	// Helper to extract raw float from nested Yahoo fmt object
	getRaw := func(obj map[string]interface{}, key string) *float64 {
		if val, ok := obj[key].(map[string]interface{}); ok && val != nil {
			if raw, ok := val["raw"].(float64); ok {
				return &raw
			}
		}
		return nil
	}

	getDate := func(obj map[string]interface{}, key string) time.Time {
		if val, ok := obj[key].(map[string]interface{}); ok && val != nil {
			if raw, ok := val["raw"].(float64); ok {
				return time.Unix(int64(raw), 0).UTC()
			}
		}
		return time.Time{}
	}
	
	var statements []models.FinancialStatement
	
	// Map income statements as base
	for i, is := range incomeStmts {
		incomeStmt, ok := is.(map[string]interface{})
		if !ok {
			continue
		}
		
		periodEnd := getDate(incomeStmt, "endDate")
		if periodEnd.IsZero() {
			continue
		}
		
		stmt := models.FinancialStatement{
			PeriodEnd: periodEnd,
			PeriodType: models.FinancialPeriodType(periodType),
			PITMode: models.PITEstimated,
			Source: "yahoo_finance",
			RetrievedAt: time.Now().UTC(),
			QualityStatus: models.DataQualityValid,
		}
		
		// Publication Date rule constraint: Delay by 45 days as Yahoo does not have real publication dates
		pubDate := periodEnd.AddDate(0, 0, 45)
		stmt.PublicationDate = &pubDate
		
		stmt.Revenue = getRaw(incomeStmt, "totalRevenue")
		stmt.EBITDA = getRaw(incomeStmt, "ebit")
		stmt.PAT = getRaw(incomeStmt, "netIncome")
		stmt.InterestExpense = getRaw(incomeStmt, "interestExpense")
		
		// Attempt to match Balance Sheet by same index (usually aligned by year)
		if i < len(balSheets) {
			bs, _ := balSheets[i].(map[string]interface{})
			if bs != nil {
				stmt.TotalAssets = getRaw(bs, "totalAssets")
				stmt.TotalDebt = getRaw(bs, "shortLongTermDebt")
				stmt.TotalEquity = getRaw(bs, "totalStockholderEquity")
				stmt.Cash = getRaw(bs, "cash")
				stmt.TotalReceivables = getRaw(bs, "netReceivables")
			}
		}
		
		// Attempt to match Cashflow by same index
		if i < len(cashFlows) {
			cf, _ := cashFlows[i].(map[string]interface{})
			if cf != nil {
				stmt.OperatingCashFlow = getRaw(cf, "totalCashFromOperatingActivities")
			}
		}
		
		statements = append(statements, stmt)
	}

	if len(statements) == 0 {
		return nil, marketdata.ErrNoData
	}

	return &marketdata.DataPoint[[]models.FinancialStatement]{
		Data:          statements,
		Source:        p.Name(),
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "yahoo_finance"
}

// SupportsRealtime returns false — Yahoo Finance has rate limits.
func (p *Provider) SupportsRealtime() bool {
	return false
}

// FormatPrice formats a price for display in INR.
func FormatPrice(price float64) string {
	return "₹" + strconv.FormatFloat(price, 'f', 2, 64)
}
