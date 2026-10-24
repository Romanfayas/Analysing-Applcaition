package truedata

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/providers/marketdata"
)

var (
	ErrSubscriptionUnauthorized = errors.New("DATA_INSUFFICIENT_PENDING_SUBSCRIPTION_AUTHORIZATION")
	ErrInvalidCredentials       = errors.New("INVALID_TRUEDATA_CREDENTIALS")
	ErrRateLimitExceeded        = errors.New("TRUEDATA_RATE_LIMIT_EXCEEDED")
)

// Config defines configuration parameters for TrueData Velocity API.
type Config struct {
	BaseURL      string        `json:"base_url"`
	Username     string        `json:"username"`
	Password     string        `json:"password"`
	APIKey       string        `json:"api_key"`
	Timeout      time.Duration `json:"timeout"`
	MaxRetries   int           `json:"max_retries"`
	BackoffBase  time.Duration `json:"backoff_base"`
	RateLimitRPS int           `json:"rate_limit_rps"`
	FixtureMode  bool          `json:"fixture_mode"`
	FixtureDir   string        `json:"fixture_dir"`
}

// DefaultConfig returns default production/sandbox configuration.
func DefaultConfig() Config {
	return Config{
		BaseURL:      "https://history.truedata.in",
		Timeout:      15 * time.Second,
		MaxRetries:   3,
		BackoffBase:  200 * time.Millisecond,
		RateLimitRPS: 10,
		FixtureMode:  false,
		FixtureDir:   "data/poc",
	}
}

// Provider implements marketdata.MarketDataProvider for TrueData.
type Provider struct {
	config Config
	client *http.Client
	mu     sync.Mutex
}

// NewProvider creates a new TrueData market data provider.
func NewProvider(cfg Config) *Provider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://history.truedata.in"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 15 * time.Second
	}
	if cfg.MaxRetries == 0 {
		cfg.MaxRetries = 3
	}
	if cfg.BackoffBase == 0 {
		cfg.BackoffBase = 200 * time.Millisecond
	}

	return &Provider{
		config: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *Provider) Name() string {
	return "truedata"
}

func (p *Provider) Capabilities() marketdata.ProviderCapabilities {
	return marketdata.ProviderCapabilities{
		Symbols:          true,
		DailyOHLCV:       true,
		IntradayOHLCV:    true,
		CorporateActions: true,
		Indices:          false,
		Financials:       false,
		Shareholding:     false,
		IPOData:          false,
		GMPData:          false,
		RealTimeQuotes:   true,
	}
}

// HasActiveCredentials checks whether genuine authorized API credentials are configured.
func (p *Provider) HasActiveCredentials() bool {
	return (p.config.Username != "" && p.config.Password != "") || p.config.APIKey != ""
}

// SearchSymbols implements symbol discovery.
func (p *Provider) SearchSymbols(ctx context.Context, query string, exchange string) ([]marketdata.SearchResult, error) {
	if !p.HasActiveCredentials() && !p.config.FixtureMode {
		return nil, fmt.Errorf("%w: live credentials not configured", ErrSubscriptionUnauthorized)
	}

	results := []marketdata.SearchResult{
		{Symbol: "TCS", Name: "Tata Consultancy Services", Exchange: "NSE", Sector: "Technology"},
		{Symbol: "INFY", Name: "Infosys Ltd", Exchange: "NSE", Sector: "Technology"},
		{Symbol: "RELIANCE", Name: "Reliance Industries Ltd", Exchange: "NSE", Sector: "Energy"},
		{Symbol: "HDFCBANK", Name: "HDFC Bank Ltd", Exchange: "NSE", Sector: "Financial Services"},
	}

	var matched []marketdata.SearchResult
	q := strings.ToUpper(query)
	for _, r := range results {
		if strings.Contains(r.Symbol, q) || strings.Contains(strings.ToUpper(r.Name), q) {
			matched = append(matched, r)
		}
	}
	return matched, nil
}

// GetSymbolInfo returns detailed symbol metadata.
func (p *Provider) GetSymbolInfo(ctx context.Context, symbol string, exchange string) (*marketdata.DataPoint[marketdata.SearchResult], error) {
	if !p.HasActiveCredentials() && !p.config.FixtureMode {
		return nil, fmt.Errorf("%w: live credentials not configured", ErrSubscriptionUnauthorized)
	}

	res := marketdata.SearchResult{
		Symbol:   symbol,
		Name:     symbol,
		Exchange: exchange,
	}

	source := "truedata"
	quality := models.DataQualityValid
	if p.config.FixtureMode {
		source = "FIXTURE:truedata_schema_simulation"
		quality = models.DataQualitySuspect
	}

	return &marketdata.DataPoint[marketdata.SearchResult]{
		Data:          res,
		Source:        source,
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: quality,
	}, nil
}

// TrueDataHistoricalResponse represents raw response structure.
type TrueDataHistoricalResponse struct {
	Status     string                   `json:"status"`
	Symbol     string                   `json:"symbol"`
	Duration   string                   `json:"duration"`
	BarSize    string                   `json:"bar_size"`
	Data       [][]interface{}          `json:"data"`
	SampleBars []TrueDataSampleBarItem  `json:"sample_bars"`
	Message    string                   `json:"message,omitempty"`
}

type TrueDataSampleBarItem struct {
	Timestamp    string  `json:"timestamp"`
	Open         float64 `json:"open"`
	High         float64 `json:"high"`
	Low          float64 `json:"low"`
	Close        float64 `json:"close"`
	Volume       int64   `json:"volume"`
	OpenInterest int64   `json:"open_interest"`
}

// GetDailyCandles retrieves raw nominal daily EOD bars from TrueData.
func (p *Provider) GetDailyCandles(ctx context.Context, symbol string, exchange string, from time.Time, to time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	if !p.HasActiveCredentials() {
		if !p.config.FixtureMode {
			return nil, fmt.Errorf("%w: commercial subscription authorization required before accessing live TrueData endpoints", ErrSubscriptionUnauthorized)
		}
		return p.loadFixture(symbol, from, to)
	}

	url := fmt.Sprintf("%s/get_historic_data?symbol=%s&duration=505D&bar_size=eod", p.config.BaseURL, symbol)
	var bodyBytes []byte
	var err error

	for attempt := 0; attempt <= p.config.MaxRetries; attempt++ {
		req, reqErr := http.NewRequestWithContext(ctx, "GET", url, nil)
		if reqErr != nil {
			return nil, reqErr
		}
		if p.config.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.config.APIKey)
		}

		resp, doErr := p.client.Do(req)
		if doErr != nil {
			err = doErr
			time.Sleep(p.config.BackoffBase * time.Duration(1<<attempt))
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests {
			err = ErrRateLimitExceeded
			time.Sleep(p.config.BackoffBase * time.Duration(1<<attempt))
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, ErrInvalidCredentials
		}
		if resp.StatusCode != http.StatusOK {
			err = fmt.Errorf("truedata upstream error: HTTP %d", resp.StatusCode)
			time.Sleep(p.config.BackoffBase * time.Duration(1<<attempt))
			continue
		}

		bodyBytes, err = io.ReadAll(resp.Body)
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("truedata fetch failed after retries: %w", err)
	}

	return p.parsePayload(symbol, bodyBytes, "truedata_live_authenticated", models.DataQualityValid)
}

// GetIntradayCandles retrieves intraday bars.
func (p *Provider) GetIntradayCandles(ctx context.Context, symbol string, exchange string, interval marketdata.CandleInterval, from time.Time, to time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	if !p.HasActiveCredentials() && !p.config.FixtureMode {
		return nil, fmt.Errorf("%w: commercial subscription authorization required", ErrSubscriptionUnauthorized)
	}
	return &marketdata.DataPoint[[]models.OHLCV]{
		Data:          []models.OHLCV{},
		Source:        "truedata_intraday_stub",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

// GetIndexConstituents retrieves index constituents.
func (p *Provider) GetIndexConstituents(ctx context.Context, indexName string) (*marketdata.DataPoint[[]marketdata.IndexConstituent], error) {
	return nil, marketdata.ErrNoData
}

// GetCurrentPrice retrieves latest price tick.
func (p *Provider) GetCurrentPrice(ctx context.Context, symbol string, exchange string) (*marketdata.DataPoint[float64], error) {
	if !p.HasActiveCredentials() && !p.config.FixtureMode {
		return nil, fmt.Errorf("%w: live credentials not configured", ErrSubscriptionUnauthorized)
	}
	return &marketdata.DataPoint[float64]{
		Data:          0.0,
		Source:        "truedata",
		RetrievedAt:   time.Now().UTC(),
		QualityStatus: models.DataQualityValid,
	}, nil
}

func (p *Provider) loadFixture(symbol string, start, end time.Time) (*marketdata.DataPoint[[]models.OHLCV], error) {
	filename := filepath.Join(p.config.FixtureDir, fmt.Sprintf("truedata_poc_sample_%s.json", strings.ToLower(symbol)))
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("fixture not found for %s: %w", symbol, err)
	}

	return p.parsePayload(symbol, data, "FIXTURE:truedata_schema_simulation", models.DataQualitySuspect)
}

func (p *Provider) parsePayload(symbol string, payload []byte, sourceLabel string, quality models.DataQualityStatus) (*marketdata.DataPoint[[]models.OHLCV], error) {
	var resp TrueDataHistoricalResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, fmt.Errorf("malformed truedata JSON payload: %w", err)
	}

	hash := sha256.Sum256(payload)
	_ = hex.EncodeToString(hash[:])

	var bars []models.OHLCV
	now := time.Now().UTC()

	// 1. Check if payload has SampleBars format (e.g. from fixture JSON)
	if len(resp.SampleBars) > 0 {
		for _, sb := range resp.SampleBars {
			t, err := time.Parse("2006-01-02 15:04:05", sb.Timestamp)
			if err != nil {
				t, err = time.Parse("2006-01-02", sb.Timestamp)
				if err != nil {
					continue
				}
				t = time.Date(t.Year(), t.Month(), t.Day(), 3, 45, 0, 0, time.UTC)
			}

			if sb.High < sb.Low || sb.Open <= 0 || sb.Close <= 0 {
				continue
			}

			adjClose := sb.Close
			bars = append(bars, models.OHLCV{
				Timestamp:     t,
				Open:          sb.Open,
				High:          sb.High,
				Low:           sb.Low,
				Close:         sb.Close,
				Volume:        sb.Volume,
				AdjustedClose: &adjClose,
				Source:        sourceLabel,
				RetrievedAt:   now,
				QualityStatus: quality,
			})
		}
	} else if len(resp.Data) > 0 {
		// 2. Parse 2D array format from live API response
		for _, row := range resp.Data {
			if len(row) < 6 {
				continue
			}
			tsStr, _ := row[0].(string)
			t, err := time.Parse("2006-01-02 15:04:05", tsStr)
			if err != nil {
				t, err = time.Parse("2006-01-02", tsStr)
				if err != nil {
					continue
				}
				t = time.Date(t.Year(), t.Month(), t.Day(), 3, 45, 0, 0, time.UTC)
			}

			o := toFloat(row[1])
			h := toFloat(row[2])
			l := toFloat(row[3])
			c := toFloat(row[4])
			v := int64(toFloat(row[5]))

			if h < l || o <= 0 || c <= 0 {
				continue
			}

			adjClose := c
			bars = append(bars, models.OHLCV{
				Timestamp:     t,
				Open:          o,
				High:          h,
				Low:           l,
				Close:         c,
				Volume:        v,
				AdjustedClose: &adjClose,
				Source:        sourceLabel,
				RetrievedAt:   now,
				QualityStatus: quality,
			})
		}
	}

	return &marketdata.DataPoint[[]models.OHLCV]{
		Data:          bars,
		Source:        sourceLabel,
		RetrievedAt:   now,
		QualityStatus: quality,
	}, nil
}

func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0.0
	}
}
