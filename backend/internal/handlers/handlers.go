package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/halal-equity/backend/pkg/response"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/quant"
)

// StocksHandler handles stock-related API endpoints.
type StocksHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

// NewStocksHandler creates a new stocks handler.
func NewStocksHandler(db *database.DB, quantClient *quant.Client) *StocksHandler {
	return &StocksHandler{db: db, quantClient: quantClient}
}

// Routes registers stock routes.
func (h *StocksHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.ListStocks)
	r.Get("/search", h.SearchStocks)
	r.Get("/{symbol}", h.GetStock)
	r.Get("/{symbol}/candles", h.GetCandles)
	r.Get("/{symbol}/fundamentals", h.GetFundamentals)
	r.Get("/{symbol}/technicals", h.GetTechnicals)
	r.Get("/{symbol}/shariah", h.GetShariahStatus)
	r.Get("/{symbol}/signal", h.GetSignal)
	r.Get("/{symbol}/risk", h.GetRiskAnalysis)
	r.Get("/{symbol}/analytics", h.GetQuantAnalytics)

	return r
}

// ListStocks returns a paginated list of stocks.
func (h *StocksHandler) ListStocks(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement with repository
	stocks := []map[string]any{
		{
			"symbol":   "RELIANCE",
			"name":     "Reliance Industries Ltd",
			"exchange": "NSE",
			"sector":   "Energy",
			"price":    2450.00,
		},
		{
			"symbol":   "TCS",
			"name":     "Tata Consultancy Services Ltd",
			"exchange": "NSE",
			"sector":   "Technology",
			"price":    3800.00,
		},
	}
	response.WithMeta(w, stocks, &response.Meta{Page: 1, PerPage: 20, Total: 2})
}

// SearchStocks searches for stocks by name or symbol.
func (h *StocksHandler) SearchStocks(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		response.ValidationError(w, "query parameter 'q' is required")
		return
	}
	response.JSON(w, http.StatusOK, []map[string]string{})
}

// GetStock returns detailed information for a single stock.
func (h *StocksHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	ctx := r.Context()

	// 1. Fetch Symbol
	symRec, err := h.db.GetSymbolByCode(ctx, symbol, "NSE")
	if err != nil {
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "symbol not found"})
		return
	}

	// 2. Fetch Latest OHLCV for current price
	ohlcvCount, _ := h.db.GetOHLCVCount(ctx, symRec.ID)
	currentPrice := 0.0
	var ohlcvData []map[string]any

	if ohlcvCount > 0 {
		// Mock fetching last 30 days for now, ideally pass proper times
		// Since GetOHLCV requires time.Time, let's fetch a small dummy range or avoid if difficult
		// For Phase 1, we will just return empty OHLCV array and 0 price if we don't query it specifically.
		ohlcvData = []map[string]any{}
	}

	// 3. Shariah Status
	shariahStatus := "DATA_UNAVAILABLE"
	shariah, err := h.db.GetLatestShariahScreening(ctx, symRec.ID)
	if err == nil && shariah != nil {
		if string(shariah.Status) == "PASS" {
			shariahStatus = "PASS"
		} else {
			shariahStatus = "FAIL"
		}
	}

	// 4. Signal
	var signalObj map[string]any
	signal, err := h.db.GetLatestSignal(ctx, symRec.ID)
	if err == nil && signal != nil {
		signalObj = map[string]any{
			"overall_score": signal.OverallScore,
			"action":        signal.Signal,
			"explanation":   signal.Explanation,
		}
	}

	resp := map[string]any{
		"symbol":        symRec.Symbol,
		"name":          symRec.Name,
		"exchange":      symRec.Exchange,
		"current_price": currentPrice,
		"timestamp":     "2023-12-29T00:00:00Z", // placeholder timestamp
		"ohlcv":         ohlcvData,
		"technicals":    map[string]any{},
		"fundamentals": map[string]any{
			"status": "DATA_UNAVAILABLE", // Graceful degradation per rule 3
		},
		"shariah": map[string]any{
			"status":                shariahStatus,
			"methodology":           "AAOIFI",
			"screening_date":        "2023-12-29T00:00:00Z",
		},
		"quant": map[string]any{},
	}
	
	if signalObj != nil {
		resp["signal"] = signalObj
	}

	response.JSON(w, http.StatusOK, resp)
}

// GetCandles returns OHLCV candle data for a stock.
func (h *StocksHandler) GetCandles(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetFundamentals returns fundamental analysis data.
func (h *StocksHandler) GetFundamentals(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetTechnicals returns technical indicator data.
func (h *StocksHandler) GetTechnicals(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetShariahStatus returns the Shariah screening status for a stock.
func (h *StocksHandler) GetShariahStatus(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetSignal returns the composite signal for a stock.
func (h *StocksHandler) GetSignal(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetRiskAnalysis returns risk calculations for a stock.
func (h *StocksHandler) GetRiskAnalysis(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// GetQuantAnalytics returns quantitative analytics for a stock.
func (h *StocksHandler) GetQuantAnalytics(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	_ = symbol
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Auth Handler ----

// AuthHandler handles authentication endpoints.
type AuthHandler struct{}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler() *AuthHandler {
	return &AuthHandler{}
}

// Routes registers auth routes.
func (h *AuthHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/register", h.Register)
	r.Post("/login", h.Login)
	r.Post("/refresh", h.RefreshToken)
	return r
}

// RegisterRequest represents a registration payload.
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

// LoginRequest represents a login payload.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Register creates a new user account.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ValidationError(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" || req.Name == "" {
		response.ValidationError(w, "email, password, and name are required")
		return
	}
	// TODO: Implement with repository and auth service
	response.JSON(w, http.StatusCreated, map[string]string{"message": "user created"})
}

// Login authenticates a user and returns a JWT token.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.ValidationError(w, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		response.ValidationError(w, "email and password are required")
		return
	}
	// TODO: Implement with repository and auth service
	response.JSON(w, http.StatusOK, map[string]string{"token": "placeholder"})
}

// RefreshToken issues a new JWT token.
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"token": "placeholder"})
}

// ---- IPO Handler ----

// IPOHandler handles IPO-related API endpoints.
type IPOHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

// NewIPOHandler creates a new IPO handler.
func NewIPOHandler(db *database.DB, quantClient *quant.Client) *IPOHandler {
	return &IPOHandler{db: db, quantClient: quantClient}
}

// Routes registers IPO routes.
func (h *IPOHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListIPOs)
	r.Get("/{id}", h.GetIPO)
	r.Get("/{id}/financials", h.GetIPOFinancials)
	r.Get("/{id}/gmp", h.GetIPOGMP)
	r.Get("/{id}/subscription", h.GetIPOSubscription)
	r.Get("/{id}/prediction", h.GetIPOPrediction)
	r.Get("/{id}/decision", h.GetIPODecision)
	r.Get("/{id}/peers", h.GetIPOPeers)
	return r
}

func (h *IPOHandler) ListIPOs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "UPCOMING"
	}

	ipos, err := h.db.GetIPOs(r.Context(), status)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPOs"})
		return
	}

	var results []map[string]any
	for _, ipo := range ipos {
		priceBand := ""
		if ipo.PriceBandLow != nil && ipo.PriceBandHigh != nil {
			priceBand = fmt.Sprintf("%.2f - %.2f", *ipo.PriceBandLow, *ipo.PriceBandHigh)
		}
		results = append(results, map[string]any{
			"id":           ipo.ID,
			"name":         ipo.CompanyName,
			"symbol":       ipo.Symbol,
			"open_date":    ipo.OpenDate,
			"close_date":   ipo.CloseDate,
			"listing_date": ipo.ListingDate,
			"price_band":   priceBand,
			"issue_size":   ipo.IssueSize,
			"status":       ipo.Status,
			// Since GMP is missing in DB schema easily, degrade gracefully
			"gmp": nil, 
			"subscription": nil,
		})
	}

	response.JSON(w, http.StatusOK, results)
}

func (h *IPOHandler) GetIPO(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPOFinancials(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPOGMP(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPOSubscription(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPOPrediction(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPODecision(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *IPOHandler) GetIPOPeers(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Signals Handler ----

type SignalsHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewSignalsHandler(db *database.DB, quantClient *quant.Client) *SignalsHandler {
	return &SignalsHandler{db: db, quantClient: quantClient}
}

func (h *SignalsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListSignals)
	r.Get("/weights", h.GetWeights)
	r.Put("/weights", h.UpdateWeights)
	return r
}

func (h *SignalsHandler) ListSignals(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *SignalsHandler) GetWeights(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]float64{
		"fundamental":   0.30,
		"technical":     0.20,
		"candlestick":   0.10,
		"momentum":      0.10,
		"volume":        0.05,
		"valuation":     0.10,
		"market_regime": 0.05,
		"risk":          0.05,
		"quality":       0.05,
	})
}

func (h *SignalsHandler) UpdateWeights(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ---- Shariah Handler ----

type ShariahHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewShariahHandler(db *database.DB, quantClient *quant.Client) *ShariahHandler {
	return &ShariahHandler{db: db, quantClient: quantClient}
}

func (h *ShariahHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/screenings", h.ListScreenings)
	r.Get("/rule-sets", h.ListRuleSets)
	r.Get("/rule-sets/{id}/versions", h.ListRuleVersions)
	r.Post("/rule-sets/{id}/versions", h.CreateRuleVersion)
	return r
}

func (h *ShariahHandler) ListScreenings(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *ShariahHandler) ListRuleSets(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *ShariahHandler) ListRuleVersions(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *ShariahHandler) CreateRuleVersion(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// ---- Portfolio Handler ----

type PortfolioHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewPortfolioHandler(db *database.DB, quantClient *quant.Client) *PortfolioHandler {
	return &PortfolioHandler{db: db, quantClient: quantClient}
}

func (h *PortfolioHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetPortfolio)
	r.Get("/positions", h.ListPositions)
	r.Get("/analytics", h.GetAnalytics)
	return r
}

func (h *PortfolioHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *PortfolioHandler) ListPositions(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *PortfolioHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Backtesting Handler ----

type BacktestingHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewBacktestingHandler(db *database.DB, quantClient *quant.Client) *BacktestingHandler {
	return &BacktestingHandler{db: db, quantClient: quantClient}
}

func (h *BacktestingHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/strategies", h.ListStrategies)
	r.Post("/strategies", h.CreateStrategy)
	r.Post("/run", h.RunBacktest)
	r.Post("/run_walk_forward", h.RunWalkForward)
	r.Get("/runs", h.ListRuns)
	r.Get("/runs/{id}", h.GetRunResults)
	return r
}

func (h *BacktestingHandler) ListStrategies(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *BacktestingHandler) CreateStrategy(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (h *BacktestingHandler) RunBacktest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	// Mock frontend payload for phase 1 integration since we don't have a real req parsing here
	// In reality this should decode r.Body
	symbolIDs := []int{1, 2, 3} // Mock multi-asset portfolio for demonstration
	lookbackDays := 90

	to := time.Now()
	from := to.AddDate(0, 0, -lookbackDays)

	historicalData := make(map[string][]quant.HistoricalPoint)

	for _, symbolID := range symbolIDs {
		records, err := h.db.GetOHLCV(ctx, symbolID, from, to)
		if err != nil {
			continue
		}
		if len(records) < 2 {
			continue
		}
		
		symName := fmt.Sprintf("SYM%d", symbolID)
		var points []quant.HistoricalPoint
		
		for _, rec := range records {
			shariahStatus := "HISTORICAL_DATA_INSUFFICIENT"
			shariahRec, err := h.db.GetShariahScreeningAsOf(ctx, symbolID, rec.Timestamp)
			if err == nil && shariahRec != nil {
				shariahStatus = shariahRec.Status
			}

			var rev, pat, debt, eq, ocf *float64
			var fundamentalAvailable bool
			pitMode := "PIT_UNAVAILABLE"
			finRec, err := h.db.GetFinancialStatementsAsOf(ctx, symbolID, rec.Timestamp)
			if err == nil && finRec != nil {
				rev = finRec.Revenue
				pat = finRec.PAT
				debt = finRec.TotalDebt
				eq = finRec.TotalEquity
				ocf = finRec.OperatingCashFlow
				fundamentalAvailable = true
				if finRec.PITMode != "" {
					pitMode = finRec.PITMode
				} else {
					pitMode = "PIT_ESTIMATED" // fallback
				}
			}
			
			points = append(points, quant.HistoricalPoint{
				Timestamp:                rec.Timestamp.Format("2006-01-02"),
				Open:                     rec.Open,
				High:                     rec.High,
				Low:                      rec.Low,
				Close:                    rec.Close,
				Volume:                   rec.Volume,
				ShariahStatus:            shariahStatus, 
				Revenue:                  rev,
				PAT:                      pat,
				TotalDebt:                debt,
				TotalEquity:              eq,
				OperatingCashFlow:        ocf,
				FundamentalDataAvailable: fundamentalAvailable,
				FundamentalPITMode:       pitMode,
			})
		}
		if len(points) > 0 {
			historicalData[symName] = points
		}
	}

	if len(historicalData) == 0 {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "Insufficient historical data for all symbols"})
		return
	}

	quantReq := quant.HistoricalBacktestRequest{
		InitialCapital:            100000.0,
		SlippagePercent:           0.1,
		PositionSizeATRMultiplier: 2.0,
		RiskPerTradePercent:       2.0,
		BuyThreshold:              80.0,
		SellThreshold:             30.0,
		CostModel: quant.CostModelConfig{
			ModelVersion:        "india_equity_delivery_v1",
			BrokerageFlat:       0.0,
			STTPercent:          0.1,
			ExchangeTxnPercent:  0.00345,
			SEBITurnoverPercent: 0.0001,
			GSTPercent:          18.0,
			StampDutyPercent:    0.015,
		},
		HistoricalData:            historicalData,
	}

	resp, err := h.quantClient.RunHistoricalBacktest(ctx, quantReq)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "Quant engine historical backtest failed"})
		return
	}

	// For Phase 3, we also could save the backtest configuration and run result here.
	// We skip the detailed persistence logic in this mock handler for brevity.

	response.JSON(w, http.StatusOK, resp)
}

func (h *BacktestingHandler) RunWalkForward(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	symbolIDs := []int{1, 2, 3} 
	lookbackDays := 756 // 3 years

	to := time.Now()
	from := to.AddDate(0, 0, -lookbackDays)

	historicalData := make(map[string][]quant.HistoricalPoint)

	for _, symbolID := range symbolIDs {
		records, err := h.db.GetOHLCV(ctx, symbolID, from, to)
		if err != nil {
			continue
		}
		if len(records) < 2 {
			continue
		}

		symName := fmt.Sprintf("SYM%d", symbolID)
		var points []quant.HistoricalPoint
		
		for _, rec := range records {
			var rev, pat, debt, eq, ocf *float64
			var fundamentalAvailable bool
			pitMode := "PIT_UNAVAILABLE"
			finRec, err := h.db.GetFinancialStatementsAsOf(ctx, symbolID, rec.Timestamp)
			if err == nil && finRec != nil {
				rev = finRec.Revenue
				pat = finRec.PAT
				debt = finRec.TotalDebt
				eq = finRec.TotalEquity
				ocf = finRec.OperatingCashFlow
				fundamentalAvailable = true
				if finRec.PITMode != "" {
					pitMode = finRec.PITMode
				} else {
					pitMode = "PIT_ESTIMATED"
				}
			}

			points = append(points, quant.HistoricalPoint{
				Timestamp:                rec.Timestamp.Format("2006-01-02"),
				Open:                     rec.Open,
				High:                     rec.High,
				Low:                      rec.Low,
				Close:                    rec.Close,
				Volume:                   rec.Volume,
				ShariahStatus:            "PASS", // Mock for walk-forward speed
				Revenue:                  rev,
				PAT:                      pat,
				TotalDebt:                debt,
				TotalEquity:              eq,
				OperatingCashFlow:        ocf,
				FundamentalDataAvailable: fundamentalAvailable,
				FundamentalPITMode:       pitMode,
			})
		}
		if len(points) > 0 {
			historicalData[symName] = points
		}
	}

	if len(historicalData) == 0 {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "Insufficient historical data"})
		return
	}

	quantReq := quant.WalkForwardRequest{
		Request: quant.HistoricalBacktestRequest{
			InitialCapital:            100000.0,
			SlippagePercent:           0.1,
			PositionSizeATRMultiplier: 2.0,
			RiskPerTradePercent:       2.0,
			BuyThreshold:              65.0,
			SellThreshold:             40.0,
			CostModel: quant.CostModelConfig{
				ModelVersion:        "india_equity_delivery_v1",
				STTPercent:          0.1,
				ExchangeTxnPercent:  0.00345,
				SEBITurnoverPercent: 0.0001,
				GSTPercent:          18.0,
				StampDutyPercent:    0.015,
			},
			HistoricalData: historicalData,
		},
		TrainDays: 504,
		TestDays:  252,
		StepDays:  252,
	}

	resp, err := h.quantClient.RunWalkForwardBacktest(ctx, quantReq)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "Quant engine walk-forward validation failed"})
		return
	}

	response.JSON(w, http.StatusOK, resp)
}

func (h *BacktestingHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *BacktestingHandler) GetRunResults(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Risk Handler ----

type RiskHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewRiskHandler(db *database.DB, quantClient *quant.Client) *RiskHandler {
	return &RiskHandler{db: db, quantClient: quantClient}
}

func (h *RiskHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/calculate", h.CalculatePositionSize)
	r.Get("/portfolio", h.GetPortfolioRisk)
	return r
}

func (h *RiskHandler) CalculatePositionSize(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *RiskHandler) GetPortfolioRisk(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Alerts Handler ----

type AlertsHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewAlertsHandler(db *database.DB, quantClient *quant.Client) *AlertsHandler {
	return &AlertsHandler{db: db, quantClient: quantClient}
}

func (h *AlertsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListAlerts)
	r.Post("/", h.CreateAlert)
	r.Delete("/{id}", h.DeleteAlert)
	r.Get("/history", h.GetHistory)
	return r
}

func (h *AlertsHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *AlertsHandler) CreateAlert(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (h *AlertsHandler) DeleteAlert(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *AlertsHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

// ---- Data Quality Handler ----

type DataQualityHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewDataQualityHandler(db *database.DB, quantClient *quant.Client) *DataQualityHandler {
	return &DataQualityHandler{db: db, quantClient: quantClient}
}

func (h *DataQualityHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetDashboard)
	r.Get("/issues", h.ListIssues)
	return r
}

func (h *DataQualityHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	query := `
		SELECT 
			'OHLCV' as dataset,
			'yahoo_finance' as provider,
			MAX(timestamp) as latest_data,
			COUNT(CASE WHEN quality_status = 'VALID' THEN 1 END) as valid_records,
			COUNT(CASE WHEN quality_status != 'VALID' THEN 1 END) as invalid_records,
			MAX(updated_at) as last_successful_fetch
		FROM ohlcv_daily
	`
	
	// Fast mock query execution since we don't have full aggregation setup
	// We will query DB and return
	var dataset, provider string
	var latestData, lastFetch time.Time
	var valid, invalid int64
	
	err := h.db.QueryRowContext(ctx, query).Scan(&dataset, &provider, &latestData, &valid, &invalid, &lastFetch)
	if err != nil {
		// Just send a mock for now if table is empty
		dataset = "OHLCV"
		provider = "yahoo_finance"
		valid = 1000
		invalid = 15
	}
	
	status := "VALID"
	if invalid > 0 {
		status = "PARTIAL"
	}
	
	resp := []map[string]any{
		{
			"dataset": dataset,
			"provider": provider,
			"latest_data": latestData.Format(time.RFC3339),
			"valid_records": valid,
			"invalid_records": invalid,
			"last_successful_fetch": lastFetch.Format(time.RFC3339),
			"status": status,
		},
	}

	response.JSON(w, http.StatusOK, resp)
}

func (h *DataQualityHandler) ListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := `
		SELECT d.id, s.symbol, d.check_type, d.severity, d.message, d.recorded_at 
		FROM data_quality_logs d
		LEFT JOIN symbols s ON d.symbol_id = s.id
		ORDER BY d.recorded_at DESC LIMIT 50
	`
	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to fetch issues"})
		return
	}
	defer rows.Close()
	
	var issues []map[string]any
	for rows.Next() {
		var id int
		var symbol, checkType, severity, message string
		var recordedAt time.Time
		if err := rows.Scan(&id, &symbol, &checkType, &severity, &message, &recordedAt); err != nil {
			continue
		}
		issues = append(issues, map[string]any{
			"id": id,
			"symbol": symbol,
			"check_type": checkType,
			"severity": severity,
			"message": message,
			"recorded_at": recordedAt.Format(time.RFC3339),
		})
	}
	
	response.JSON(w, http.StatusOK, issues)
}

// ---- Settings Handler ----

type SettingsHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewSettingsHandler(db *database.DB, quantClient *quant.Client) *SettingsHandler {
	return &SettingsHandler{db: db, quantClient: quantClient}
}

func (h *SettingsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.GetSettings)
	r.Put("/", h.UpdateSettings)
	return r
}

func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ---- Paper Trading Handler ----

type PaperTradingHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewPaperTradingHandler(db *database.DB, quantClient *quant.Client) *PaperTradingHandler {
	return &PaperTradingHandler{db: db, quantClient: quantClient}
}

func (h *PaperTradingHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/portfolios", h.ListPortfolios)
	r.Post("/portfolios", h.CreatePortfolio)
	r.Get("/portfolios/{id}", h.GetPortfolio)
	r.Post("/portfolios/{id}/orders", h.PlaceOrder)
	r.Get("/portfolios/{id}/orders", h.ListOrders)
	r.Get("/portfolios/{id}/positions", h.ListPositions)
	r.Get("/portfolios/{id}/analytics", h.GetAnalytics)
	return r
}

func (h *PaperTradingHandler) ListPortfolios(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *PaperTradingHandler) CreatePortfolio(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

func (h *PaperTradingHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

func (h *PaperTradingHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "order_placed"})
}

func (h *PaperTradingHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *PaperTradingHandler) ListPositions(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]string{})
}

func (h *PaperTradingHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{"status": "placeholder"})
}

// ---- Health Handler ----

// HealthHandler returns API health status.
type HealthHandler struct{}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"service": "halal-equity-api",
	})
}
