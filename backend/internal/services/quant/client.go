package quant

import (
	"github.com/halal-equity/backend/internal/services/breakouts"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	TotalAssets        *float64 `json:"total_assets,omitempty"`
	TotalEquity        *float64 `json:"total_equity,omitempty"`
	TotalDebt          *float64 `json:"total_debt,omitempty"`
	CurrentAssets      *float64 `json:"current_assets,omitempty"`
	CurrentLiabilities *float64 `json:"current_liabilities,omitempty"`
	CashAndEquivalents *float64 `json:"cash_and_equivalents,omitempty"`
	Inventory          *float64 `json:"inventory,omitempty"`
	Receivables        *float64 `json:"receivables,omitempty"`
	Revenue            *float64 `json:"revenue,omitempty"`
	OperatingProfit    *float64 `json:"operating_profit,omitempty"`
	NetProfit          *float64 `json:"net_profit,omitempty"`
	Eps                *float64 `json:"eps,omitempty"`
	MarketCap          *float64 `json:"market_cap,omitempty"`
	CurrentPrice       *float64 `json:"current_price,omitempty"`
	PrevRevenue        *float64 `json:"prev_revenue,omitempty"`
	PrevNetProfit      *float64 `json:"prev_net_profit,omitempty"`
	PrevEps            *float64 `json:"prev_eps,omitempty"`
	PrevTotalAssets    *float64 `json:"prev_total_assets,omitempty"`
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
	MarketDataStatus   string   `json:"market_data_status"`
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
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("quant engine returned error status %d: %s", resp.StatusCode, string(respBody))
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
	InitialCapital     float64           `json:"initial_capital"`
	FinalCapital       float64           `json:"final_capital"`
	TotalReturnPercent float64           `json:"total_return_percent"`
	MaxDrawdownPercent float64           `json:"max_drawdown_percent"`
	WinRatePercent     float64           `json:"win_rate_percent"`
	TotalTrades        int               `json:"total_trades"`
	Trades             []Trade           `json:"trades"`
	EquityCurve        []PortfolioEquity `json:"equity_curve"`
	AdvancedMetrics    *AdvancedMetrics  `json:"advanced_metrics"`
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
	Timestamp                string   `json:"timestamp"`
	Open                     float64  `json:"open"`
	High                     float64  `json:"high"`
	Low                      float64  `json:"low"`
	Close                    float64  `json:"close"`
	Volume                   int64    `json:"volume"`
	TechnicalScore           *float64 `json:"technical_score,omitempty"`
	FundamentalScore         *float64 `json:"fundamental_score,omitempty"`
	Revenue                  *float64 `json:"revenue,omitempty"`
	PAT                      *float64 `json:"pat,omitempty"`
	TotalDebt                *float64 `json:"total_debt,omitempty"`
	TotalEquity              *float64 `json:"total_equity,omitempty"`
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

// BreakoutAnalyzeRequest represents a single stock breakout request.
type BreakoutAnalyzeRequest struct {
	Symbol          string    `json:"symbol"`
	Open            []float64 `json:"open"`
	High            []float64 `json:"high"`
	Low             []float64 `json:"low"`
	Close           []float64 `json:"close"`
	Volume          []int64   `json:"volume"`
	ShariahStatus   string    `json:"shariah_status"`
	BenchmarkCloses []float64 `json:"benchmark_closes,omitempty"`
	DataQuality     string    `json:"data_quality"`
}

// BreakoutResult represents the complete analysis of a stock's breakout setup.
type BreakoutResult struct {
	Symbol                  string    `json:"symbol"`
	Stage                   string    `json:"stage"`
	CurrentPrice            float64   `json:"current_price"`
	ResistanceLevel         float64   `json:"resistance_level"`
	SupportLevel            float64   `json:"support_level"`
	DistanceToResistancePct float64   `json:"distance_to_resistance_pct"`
	RVOL                    float64   `json:"rvol"`
	ATR                     float64   `json:"atr"`
	BollingerBandWidth      float64   `json:"bollinger_band_width"`
	ConsolidationBars       int       `json:"consolidation_bars"`
	VolumeDemandProxy       float64   `json:"volume_demand_proxy"`
	OrderBookDepthStatus    string    `json:"order_book_depth_status"`
	EntryZone               []float64 `json:"entry_zone"`
	InvalidationLevel       float64   `json:"invalidation_level"`
	Target1                 float64   `json:"target_1"`
	Target2                 float64   `json:"target_2"`
	RiskRewardRatio         float64   `json:"risk_reward_ratio"`
	OverallScore            float64   `json:"overall_score"`
	// StructureScore is the price-structure score before the Shariah gate zeroes OverallScore; nil when there is too
	// little history to score at all.
	StructureScore         *float64           `json:"structure_score"`
	SetupType              string             `json:"setup_type,omitempty"`
	MarketRegime           string             `json:"market_regime,omitempty"`
	LiquidityADTCr         float64            `json:"liquidity_adt_cr,omitempty"`
	ScoreBreakdown         map[string]float64 `json:"score_breakdown"`
	ShariahStatus          string             `json:"shariah_status"`
	IsShariahEligible      bool               `json:"is_shariah_eligible"`
	Reasons                []string           `json:"reasons"`
	InvalidationConditions []string           `json:"invalidation_conditions"`
	DataQuality            string             `json:"data_quality"`
	Sector                 string             `json:"sector,omitempty"`
	// Live says where the latest price stands against these levels. The levels themselves come from daily bars.
	Live *breakouts.Position `json:"live,omitempty"`
}

// StockScanItem represents a stock item in a universe scan.
type StockScanItem struct {
	Symbol        string    `json:"symbol"`
	Sector        string    `json:"sector"`
	Open          []float64 `json:"open"`
	High          []float64 `json:"high"`
	Low           []float64 `json:"low"`
	Close         []float64 `json:"close"`
	Volume        []int64   `json:"volume"`
	ShariahStatus string    `json:"shariah_status"`
	DataQuality   string    `json:"data_quality"`
}

// BreakoutScanRequest represents a universe breakout scan request.
type BreakoutScanRequest struct {
	Stocks          []StockScanItem `json:"stocks"`
	BenchmarkCloses []float64       `json:"benchmark_closes,omitempty"`
}

// SectorHeatmapItem represents aggregate sector score.
type SectorHeatmapItem struct {
	Sector       string  `json:"sector"`
	AverageScore float64 `json:"average_score"`
	Count        int     `json:"count"`
	Strength     string  `json:"strength"`
}

// BreakoutScanResponse represents the universe scan result.
type BreakoutScanResponse struct {
	TotalScreened         int                 `json:"total_screened"`
	StageCounts           map[string]int      `json:"stage_counts"`
	BullishBreadthPercent float64             `json:"bullish_breadth_percent"`
	SectorHeatmap         []SectorHeatmapItem `json:"sector_heatmap"`
	Candidates            []BreakoutResult    `json:"candidates"`
}

// AnalyzeBreakout calls the breakout analysis endpoint.
func (c *Client) AnalyzeBreakout(ctx context.Context, req BreakoutAnalyzeRequest) (*BreakoutResult, error) {
	var resp BreakoutResult
	if err := c.doReq(ctx, "breakout/analyze", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// ScanBreakouts calls the universe breakout scan endpoint.
func (c *Client) ScanBreakouts(ctx context.Context, req BreakoutScanRequest) (*BreakoutScanResponse, error) {
	var resp BreakoutScanResponse
	if err := c.doReq(ctx, "breakout/scan", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// PositionSizeRequest represents request payload for risk engine position sizing.
type PositionSizeRequest struct {
	Capital        float64  `json:"capital"`
	RiskPercentage float64  `json:"risk_percentage"`
	EntryPrice     float64  `json:"entry_price"`
	StopLoss       float64  `json:"stop_loss"`
	TargetPrice    *float64 `json:"target_price,omitempty"`
}

// PositionSizeResponse represents position sizing output from risk engine.
type PositionSizeResponse struct {
	Shares          int     `json:"shares"`
	MaxRiskAmount   float64 `json:"max_risk_amount"`
	PositionValue   float64 `json:"position_value"`
	RiskPerShare    float64 `json:"risk_per_share"`
	RiskRewardRatio float64 `json:"risk_reward_ratio"`
}

// CalculatePositionSize calls the risk engine position size endpoint.
func (c *Client) CalculatePositionSize(ctx context.Context, req PositionSizeRequest) (*PositionSizeResponse, error) {
	var resp PositionSizeResponse
	if err := c.doReq(ctx, "risk/position_size", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IndicatorResult is one calculated technical indicator series.
type IndicatorResult struct {
	Indicator  string                 `json:"indicator"`
	Parameters map[string]interface{} `json:"parameters"`
	Values     interface{}            `json:"values"`
	Signal     *string                `json:"signal"`
}

// IndicatorsResponse is the technical indicators response.
type IndicatorsResponse struct {
	Indicators []IndicatorResult `json:"indicators"`
}

// CalculateIndicators calls the technical indicators endpoint.
func (c *Client) CalculateIndicators(ctx context.Context, ohlcv OHLCVInput, indicators []string) (*IndicatorsResponse, error) {
	req := struct {
		OHLCV      OHLCVInput `json:"ohlcv"`
		Indicators []string   `json:"indicators"`
	}{ohlcv, indicators}
	var resp IndicatorsResponse
	if err := c.doReq(ctx, "technical/indicators", req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// RiskMetricsResponse mirrors the Python risk metrics response.
type RiskMetricsResponse struct {
	VolatilityDaily  *float64 `json:"volatility_daily"`
	VolatilityAnnual *float64 `json:"volatility_annual"`
	MaxDrawdown      *float64 `json:"max_drawdown"`
	VaR95            *float64 `json:"var_95"`
	SharpeRatio      *float64 `json:"sharpe_ratio"`
}

// CalculateRiskMetrics calls the risk metrics endpoint with a series of periodic returns.
func (c *Client) CalculateRiskMetrics(ctx context.Context, returns []float64) (*RiskMetricsResponse, error) {
	var resp RiskMetricsResponse
	if err := c.doReq(ctx, "risk/metrics", struct {
		Returns []float64 `json:"returns"`
	}{returns}, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// StrategyGovernance is the quant engine's runtime-introspected frozen-strategy attestation.
type StrategyGovernance struct {
	RuntimeHash   string   `json:"runtime_hash"`
	PinnedHash    string   `json:"pinned_hash"`
	MatchesPinned bool     `json:"matches_pinned"`
	Problems      []string `json:"problems"`
}

// GetStrategyGovernance fetches the frozen-strategy attestation from the quant engine.
func (c *Client) GetStrategyGovernance(ctx context.Context) (*StrategyGovernance, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/governance/strategy", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("quant engine request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("quant engine returned error status %d", resp.StatusCode)
	}
	var g StrategyGovernance
	if err := json.NewDecoder(resp.Body).Decode(&g); err != nil {
		return nil, fmt.Errorf("failed to decode strategy governance: %w", err)
	}
	return &g, nil
}

// predictionHTTP has a long timeout: walk-forward training re-fits the model once per test day.
var predictionHTTP = &http.Client{Timeout: 10 * time.Minute}

// PredictionCall invokes a prediction-engine operation ("train", "generate" or "resolve") and returns its summary.
func (c *Client) PredictionCall(ctx context.Context, op string) (map[string]interface{}, error) {
	switch op {
	case "train", "generate", "resolve":
	default:
		return nil, fmt.Errorf("unknown prediction operation %q", op)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/prediction/"+op, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := predictionHTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("prediction %s failed: %w", op, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("prediction %s returned %d: %s", op, resp.StatusCode, string(body))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("prediction %s: bad response: %w", op, err)
	}
	return out, nil
}

// ScreenSymbol asks the quant engine to fetch reported financials for a stored symbol and store a fresh Shariah
// screening. It returns the resulting status (PASS, FAIL or DATA_INSUFFICIENT).
func (c *Client) ScreenSymbol(ctx context.Context, ticker string) (string, error) {
	var out struct {
		Status string `json:"status"`
	}
	if err := c.doReq(ctx, "shariah/screen-symbol/"+ticker, nil, &out); err != nil {
		return "", err
	}
	return out.Status, nil
}

// PerformanceReport asks the quant engine for bootstrap-backed statistics over closed trades.
func (c *Client) PerformanceReport(ctx context.Context, body any) (map[string]interface{}, error) {
	var out map[string]interface{}
	if err := c.doReq(ctx, "risk/performance", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}


// RefreshBreakoutEvidence re-runs the breakout study on the exchange series. It takes about a minute.
func (c *Client) RefreshBreakoutEvidence(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/research/breakout-evidence", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := predictionHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("breakout evidence returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// RefreshLearning runs the governed learning loop: new lessons, the walk-forward test of the strategy menu, and the promotion /
// probation decision. It takes about a minute and writes only to the append-only learning tables.
func (c *Client) RefreshLearning(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/research/learning/run", bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := predictionHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("learning run returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
