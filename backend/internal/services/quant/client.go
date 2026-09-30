package quant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/halal-equity/backend/internal/metrics"
)

// Client is an HTTP client for the Python Quant Engine.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new Quant Engine client.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// OHLCVInput represents OHLCV data for technical and candlestick analysis.
type OHLCVInput struct {
	Timestamps []string  `json:"timestamps"`
	Open       []float64 `json:"open"`
	High       []float64 `json:"high"`
	Low        []float64 `json:"low"`
	Close      []float64 `json:"close"`
	Volume     []int64   `json:"volume"`
}

// FundamentalRequest matches the Python FundamentalRequest schema.
type FundamentalRequest struct {
	TotalAssets           *float64 `json:"total_assets,omitempty"`
	TotalEquity           *float64 `json:"total_equity,omitempty"`
	TotalDebt             *float64 `json:"total_debt,omitempty"`
	CurrentAssets         *float64 `json:"current_assets,omitempty"`
	CurrentLiabilities    *float64 `json:"current_liabilities,omitempty"`
	CashAndEquivalents    *float64 `json:"cash_and_equivalents,omitempty"`
	Inventory             *float64 `json:"inventory,omitempty"`
	Receivables           *float64 `json:"receivables,omitempty"`
	Revenue               *float64 `json:"revenue,omitempty"`
	OperatingProfit       *float64 `json:"operating_profit,omitempty"`
	NetProfit             *float64 `json:"net_profit,omitempty"`
	Eps                   *float64 `json:"eps,omitempty"`
	MarketCap             *float64 `json:"market_cap,omitempty"`
	CurrentPrice          *float64 `json:"current_price,omitempty"`
	PrevRevenue           *float64 `json:"prev_revenue,omitempty"`
	PrevNetProfit         *float64 `json:"prev_net_profit,omitempty"`
	PrevEps               *float64 `json:"prev_eps,omitempty"`
	PrevTotalAssets       *float64 `json:"prev_total_assets,omitempty"`
}

// ShariahRequest matches the Python ShariahRequest schema.
type ShariahRequest struct {
	TotalDebt               *float64 `json:"total_debt,omitempty"`
	CashAndEquivalents      *float64 `json:"cash_and_equivalents,omitempty"`
	InterestBearingDeposits *float64 `json:"interest_bearing_deposits,omitempty"`
	TotalReceivables        *float64 `json:"total_receivables,omitempty"`
	MarketCap               *float64 `json:"market_cap,omitempty"`
	TotalAssets             *float64 `json:"total_assets,omitempty"`
	TotalRevenue            *float64 `json:"total_revenue,omitempty"`
	InterestIncome          *float64 `json:"interest_income,omitempty"`
	NonCompliantRevenue     *float64 `json:"non_compliant_revenue,omitempty"`
	IsFinancialInstitution  bool     `json:"is_financial_institution"`
	IsProhibitedIndustry    bool     `json:"is_prohibited_industry"`
}

// SignalRequest matches the Python SignalRequest schema.
type SignalRequest struct {
	FundamentalScore   *float64 `json:"fundamental_score,omitempty"`
	TechnicalScore     *float64 `json:"technical_score,omitempty"`
	CandlestickScore   *float64 `json:"candlestick_score,omitempty"`
	MomentumScore      *float64 `json:"momentum_score,omitempty"`
	VolumeScore        *float64 `json:"volume_score,omitempty"`
	ValuationScore     *float64 `json:"valuation_score,omitempty"`
	MarketRegimeScore  *float64 `json:"market_regime_score,omitempty"`
	RiskScore          *float64 `json:"risk_score,omitempty"`
	QualityScore       *float64 `json:"quality_score,omitempty"`
	ShariahStatus      string   `json:"shariah_status"`
	FundamentalFactors []string `json:"fundamental_factors,omitempty"`
	TechnicalFactors   []string `json:"technical_factors,omitempty"`
	ConflictingFactors []string `json:"conflicting_factors,omitempty"`
}

// SignalResponse matches the Python SignalResponse schema.
type SignalResponse struct {
	Signal             string                   `json:"signal"`
	OverallScore       float64                  `json:"overall_score"`
	Dimensions         []map[string]interface{} `json:"dimensions"`
	ShariahStatus      string                   `json:"shariah_status"`
	ShariahOverride    bool                     `json:"shariah_override"`
	Explanation        string                   `json:"explanation"`
	SupportingFactors  []string                 `json:"supporting_factors"`
	ConflictingFactors []string                 `json:"conflicting_factors"`
	Confidence         float64                  `json:"confidence"`
	Disclaimer         string                   `json:"disclaimer"`
}

// doReq performs a JSON HTTP request and decodes the response.
func (c *Client) doReq(ctx context.Context, endpoint string, reqBody interface{}, respObj interface{}) error {
	url := fmt.Sprintf("%s/api/v1/%s", c.baseURL, endpoint)
	
	// Start timer for Prometheus
	start := time.Now()

	var body []byte
	var err error
	if reqBody != nil {
		body, err = json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("failed to encode request: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	
	// Record latency
	duration := time.Since(start).Milliseconds()
	metrics.QuantEngineLatency.WithLabelValues(endpoint).Observe(float64(duration))

	if err != nil {
		return fmt.Errorf("quant engine request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("quant engine returned error status: %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(respObj); err != nil {
		return fmt.Errorf("failed to decode quant engine response: %w", err)
	}

	return nil
}

// CalculateSignal calls the composite signal endpoint.
func (c *Client) CalculateSignal(ctx context.Context, req SignalRequest) (*SignalResponse, error) {
	var resp SignalResponse
	if err := c.doReq(ctx, "signal/calculate", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// DetectCandlestickPatterns calls the candlestick pattern detection endpoint.
func (c *Client) DetectCandlestickPatterns(ctx context.Context, req OHLCVInput) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "candlestick/detect", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CalculateFundamentals calls the fundamental metrics endpoint.
func (c *Client) CalculateFundamentals(ctx context.Context, req FundamentalRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "fundamental/metrics", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ScreenShariah calls the Shariah screening endpoint.
func (c *Client) ScreenShariah(ctx context.Context, req ShariahRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "shariah/screen", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// IPOAnalyzeRequest matches the Python IPOAnalyzeRequest schema.
type IPOAnalyzeRequest struct {
	IssuePrice         float64  `json:"issue_price"`
	GMP                *float64 `json:"gmp,omitempty"`
	QIBSubscription    *float64 `json:"qib_subscription,omitempty"`
	NIISubscription    *float64 `json:"nii_subscription,omitempty"`
	RetailSubscription *float64 `json:"retail_subscription,omitempty"`
	IssueSizeCrores    *float64 `json:"issue_size_crores,omitempty"`
	ShariahStatus      string   `json:"shariah_status"`
}

// IPOResponse matches the Python IPO Analysis response schema.
type IPOResponse struct {
	Decision            string  `json:"decision"`
	BaseListingPrice    float64 `json:"base_listing_price"`
	BullListingPrice    float64 `json:"bull_listing_price"`
	BearListingPrice    float64 `json:"bear_listing_price"`
	ExpectedGainPercent float64 `json:"expected_gain_percent"`
	ShariahOverride     bool    `json:"shariah_override"`
	Explanation         string  `json:"explanation"`
}

// AnalyzeIPO calls the IPO analysis endpoint.
func (c *Client) AnalyzeIPO(ctx context.Context, req IPOAnalyzeRequest) (*IPOResponse, error) {
	var resp IPOResponse
	if err := c.doReq(ctx, "ipo/analyze", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// BacktestDailyData matches the Python DailyData schema.
type BacktestDailyData struct {
	Timestamp   string  `json:"timestamp"`
	Open        float64 `json:"open"`
	High        float64 `json:"high"`
	Low         float64 `json:"low"`
	Close       float64 `json:"close"`
	Volume      int64   `json:"volume"`
	SignalScore float64 `json:"signal_score"`
}

// Trade matches the Python Trade schema.
type Trade struct {
	Symbol         string  `json:"symbol"`
	EntryTimestamp string  `json:"entry_date"`
	ExitTimestamp  string  `json:"exit_date"`
	EntryPrice     float64 `json:"entry_price"`
	ExitPrice      float64 `json:"exit_price"`
	Quantity       float64 `json:"shares"`
	Direction      string  `json:"direction"`
	PnL            float64 `json:"pnl"`
	ReturnPercent  float64 `json:"pnl_percent"`
	ExitReason     string  `json:"exit_reason"`
}

type PortfolioEquity struct {
	Date          string  `json:"date"`
	TotalEquity   float64 `json:"total_equity"`
	Cash          float64 `json:"cash"`
	InvestedValue float64 `json:"invested_value"`
	Drawdown      float64 `json:"drawdown"`
}

type AdvancedMetrics struct {
	CAGRPercent       float64  `json:"cagr_percent"`
	VolatilityPercent float64  `json:"volatility_percent"`
	SharpeRatio       float64  `json:"sharpe_ratio"`
	SortinoRatio      float64  `json:"sortino_ratio"`
	CalmarRatio       float64  `json:"calmar_ratio"`
	VaR95Percent      float64  `json:"var_95_percent"`
	CVaR95Percent     float64  `json:"cvar_95_percent"`
	AlphaPercent      *float64 `json:"alpha_percent"`
	Beta              *float64 `json:"beta"`
}

// BacktestResult matches the Python BacktestResult schema.
type BacktestResult struct {
	InitialCapital     float64            `json:"initial_capital"`
	FinalCapital       float64            `json:"final_capital"`
	TotalReturnPercent float64            `json:"total_return_percent"`
	MaxDrawdownPercent float64            `json:"max_drawdown_percent"`
	WinRatePercent     float64            `json:"win_rate_percent"`
	TotalTrades        int                `json:"total_trades"`
	Trades             []Trade            `json:"trades"`
	EquityCurve        []PortfolioEquity  `json:"equity_curve"`
	AdvancedMetrics    *AdvancedMetrics   `json:"advanced_metrics"`
}

type WalkForwardWindow struct {
	TrainStart string         `json:"train_start"`
	TrainEnd   string         `json:"train_end"`
	TestStart  string         `json:"test_start"`
	TestEnd    string         `json:"test_end"`
	TestResult BacktestResult `json:"test_result"`
}

type WalkForwardResult struct {
	TotalOOSReturnPercent float64             `json:"total_oos_return_percent"`
	AverageOOSWinRate     float64             `json:"average_oos_win_rate"`
	AverageOOSMaxDrawdown float64             `json:"average_oos_max_drawdown"`
	Windows               []WalkForwardWindow `json:"windows"`
}

// BacktestRequest matches the Python BacktestRequest schema.
type BacktestRequest struct {
	InitialCapital        float64             `json:"initial_capital"`
	SlippagePercent       float64             `json:"slippage_percent"`
	TransactionFeePercent float64             `json:"transaction_fee_percent"`
	PositionSizePercent   float64             `json:"position_size_percent"`
	BuyThreshold          float64             `json:"buy_threshold"`
	SellThreshold         float64             `json:"sell_threshold"`
	HistoricalData        []BacktestDailyData `json:"historical_data"`
}

// RunBacktest calls the backtesting engine.
func (c *Client) RunBacktest(ctx context.Context, req BacktestRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "backtesting/run", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// HistoricalPoint matches the Python HistoricalPoint schema.
type HistoricalPoint struct {
	Timestamp         string   `json:"timestamp"`
	Open              float64  `json:"open"`
	High              float64  `json:"high"`
	Low               float64  `json:"low"`
	Close             float64  `json:"close"`
	Volume            int64    `json:"volume"`
	TechnicalScore    *float64 `json:"technical_score,omitempty"`
	FundamentalScore  *float64 `json:"fundamental_score,omitempty"`
	Revenue           *float64 `json:"revenue,omitempty"`
	PAT               *float64 `json:"pat,omitempty"`
	TotalDebt         *float64 `json:"total_debt,omitempty"`
	TotalEquity       *float64 `json:"total_equity,omitempty"`
	OperatingCashFlow        *float64 `json:"operating_cash_flow,omitempty"`
	FundamentalDataAvailable bool     `json:"fundamental_data_available"`
	FundamentalPITMode       string   `json:"fundamental_pit_mode"`
	ShariahStatus            string   `json:"shariah_status"`
	BenchmarkClose           *float64 `json:"benchmark_close,omitempty"`
}

type CostModelConfig struct {
	ModelVersion        string  `json:"model_version"`
	BrokerageFlat       float64 `json:"brokerage_flat"`
	STTPercent          float64 `json:"stt_percent"`
	ExchangeTxnPercent  float64 `json:"exchange_txn_percent"`
	SEBITurnoverPercent float64 `json:"sebi_turnover_percent"`
	GSTPercent          float64 `json:"gst_percent"`
	StampDutyPercent    float64 `json:"stamp_duty_percent"`
}

// HistoricalBacktestRequest matches the Python HistoricalBacktestRequest schema.
type HistoricalBacktestRequest struct {
	InitialCapital            float64                      `json:"initial_capital"`
	SlippagePercent           float64                      `json:"slippage_percent"`
	PositionSizeATRMultiplier float64                      `json:"position_size_atr_multiplier"`
	RiskPerTradePercent       float64                      `json:"risk_per_trade_percent"`
	BuyThreshold              float64                      `json:"buy_threshold"`
	SellThreshold             float64                      `json:"sell_threshold"`
	CostModel                 CostModelConfig              `json:"cost_model"`
	HistoricalData            map[string][]HistoricalPoint `json:"historical_data"`
}

type WalkForwardRequest struct {
	Request   HistoricalBacktestRequest `json:"request"`
	TrainDays int                       `json:"train_days"`
	TestDays  int                       `json:"test_days"`
	StepDays  int                       `json:"step_days"`
}

// RunHistoricalBacktest calls the historical point-in-time backtesting engine.
func (c *Client) RunHistoricalBacktest(ctx context.Context, req HistoricalBacktestRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "backtesting/run_historical", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RunWalkForwardBacktest calls the walk-forward optimization endpoint.
func (c *Client) RunWalkForwardBacktest(ctx context.Context, req WalkForwardRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "backtesting/run_walk_forward", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PositionSizeRequest matches the Python PositionSizeRequest schema.
type PositionSizeRequest struct {
	Capital        float64  `json:"capital"`
	RiskPercentage float64  `json:"risk_percentage"`
	EntryPrice     float64  `json:"entry_price"`
	StopLoss       float64  `json:"stop_loss"`
	TargetPrice    *float64 `json:"target_price,omitempty"`
}

// CalculatePositionSize calls the position size endpoint.
func (c *Client) CalculatePositionSize(ctx context.Context, req PositionSizeRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "risk/position_size", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RiskMetricsRequest matches the Python RiskMetricsRequest schema.
type RiskMetricsRequest struct {
	Returns []float64 `json:"returns"`
}

// CalculateRiskMetrics calls the portfolio risk metrics endpoint.
func (c *Client) CalculateRiskMetrics(ctx context.Context, req RiskMetricsRequest) (map[string]interface{}, error) {
	var resp map[string]interface{}
	if err := c.doReq(ctx, "risk/metrics", req, &resp); err != nil {
		return nil, err
	}
	return resp, nil
}

