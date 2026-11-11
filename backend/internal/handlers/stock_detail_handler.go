package handlers

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/pkg/response"
)

const (
	defaultCandleLimit = 250
	maxCandleLimit     = 1000
	maxSearchLen       = 50
	searchResultLimit  = 20
	// A return series needs at least this many observations before volatility,
	// drawdown and VaR mean anything.
	minReturnsForRisk = 20
	// Window of research-eligible daily bars fed to the quant engine.
	analysisWindow = 250
)

// indicatorPlan pairs an indicator spec with the minimum number of bars it needs to produce a value.
type indicatorPlan struct {
	Spec    string
	MinBars int
}

// technicalPlan is ordered by ascending MinBars.
var technicalPlan = []indicatorPlan{
	{"sma_5", 5},
	{"sma_10", 10},
	{"rsi_14", 15},
	{"atr_14", 15},
	{"sma_20", 20},
	{"bollinger_20", 20},
	{"macd", 35},
	{"sma_50", 50},
}

// planIndicators splits the technical plan into what can be computed from n bars and what cannot.
func planIndicators(n int) (compute []string, skipped []map[string]any) {
	for _, p := range technicalPlan {
		if n >= p.MinBars {
			compute = append(compute, p.Spec)
		} else {
			skipped = append(skipped, map[string]any{"indicator": p.Spec, "min_bars": p.MinBars})
		}
	}
	if skipped == nil {
		skipped = []map[string]any{}
	}
	return compute, skipped
}

// simpleReturns returns the period-over-period simple returns of a price series.
// Non-positive prices yield no return for that step rather than a fabricated value.
func simpleReturns(prices []float64) []float64 {
	out := make([]float64, 0, len(prices))
	for i := 1; i < len(prices); i++ {
		if prices[i-1] > 0 && prices[i] > 0 {
			out = append(out, prices[i]/prices[i-1]-1)
		}
	}
	return out
}

// latestScalar returns the last non-null numeric value of an indicator series, or nil.
func latestScalar(values interface{}) *float64 {
	series, ok := values.([]interface{})
	if !ok {
		return nil
	}
	for i := len(series) - 1; i >= 0; i-- {
		if f, ok := series[i].(float64); ok && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return &f
		}
	}
	return nil
}

func (h *StocksHandler) lookupSymbol(w http.ResponseWriter, r *http.Request) (*database.SymbolRecord, bool) {
	code := strings.ToUpper(strings.TrimSpace(symbolParam(r)))
	sym, err := h.db.GetSymbolByCode(r.Context(), code, "NSE")
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "symbol")
		return nil, false
	}
	if err != nil {
		log.Error().Err(err).Str("symbol", code).Msg("symbol lookup failed")
		response.Error(w, http.StatusInternalServerError, "failed to look up symbol")
		return nil, false
	}
	return sym, true
}

// SearchStocks searches active instruments by ticker or name.
func (h *StocksHandler) SearchStocks(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		response.ValidationError(w, "query parameter 'q' is required")
		return
	}
	if len(q) > maxSearchLen {
		response.ValidationError(w, "query parameter 'q' is too long")
		return
	}
	syms, err := h.db.SearchActiveSymbols(r.Context(), q, searchResultLimit)
	if err != nil {
		log.Error().Err(err).Msg("symbol search failed")
		response.Error(w, http.StatusInternalServerError, "search failed")
		return
	}
	results := make([]map[string]any, 0, len(syms))
	for _, s := range syms {
		results = append(results, map[string]any{
			"symbol": s.Symbol, "name": s.Name, "exchange": s.Exchange, "sector": s.Sector,
		})
	}
	response.JSON(w, http.StatusOK, results)
}

// GetCandles returns stored OHLCV bars. Daily bars carry a research_eligible flag so that
// quarantined history can be displayed without ever being mistaken for verified data.
func (h *StocksHandler) GetCandles(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	limit := defaultCandleLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxCandleLimit {
			response.ValidationError(w, "limit must be an integer between 1 and 1000")
			return
		}
		limit = n
	}
	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "1d"
	}

	switch interval {
	case "1d":
		bars, err := h.db.GetDailyBars(r.Context(), sym.ID, limit, r.URL.Query().Get("eligible_only") == "true")
		if err != nil {
			log.Error().Err(err).Msg("daily bars failed")
			response.Error(w, http.StatusInternalServerError, "failed to load candles")
			return
		}
		eligible := 0
		for _, b := range bars {
			if b.ResearchEligible {
				eligible++
			}
		}
		response.JSON(w, http.StatusOK, map[string]any{
			"symbol": sym.Symbol, "interval": "1d", "count": len(bars),
			"research_eligible_count": eligible, "quarantined_count": len(bars) - eligible,
			"candles": bars,
		})
	case "intraday":
		bars, err := h.db.GetIntradayBars(r.Context(), sym.ID, limit)
		if err != nil {
			log.Error().Err(err).Msg("intraday bars failed")
			response.Error(w, http.StatusInternalServerError, "failed to load candles")
			return
		}
		response.JSON(w, http.StatusOK, map[string]any{
			"symbol": sym.Symbol, "interval": "intraday", "count": len(bars), "candles": bars,
		})
	default:
		response.ValidationError(w, "interval must be '1d' or 'intraday'")
	}
}

// fundamentalsPayload renders stored fundamentals, or an explicit unavailable marker. Missing is never zero.
func (h *StocksHandler) fundamentalsPayload(ctx context.Context, symbolID int) map[string]any {
	fund, err := h.db.GetLatestFundamentals(ctx, symbolID)
	if err != nil || fund == nil {
		return map[string]any{
			"status": "DATA_UNAVAILABLE",
			"reason": "No verified financial statement is stored for this instrument.",
		}
	}
	return map[string]any{
		"status":       "AVAILABLE",
		"revenue":      fund.Revenue,
		"pat":          fund.PAT,
		"eps":          fund.EPS,
		"total_debt":   fund.TotalDebt,
		"total_assets": fund.TotalAssets,
		"period_end":   fund.PeriodEnd.Format(time.RFC3339),
		"pit_mode":     fund.PITMode,
	}
}

// GetFundamentals returns the latest stored fundamentals.
func (h *StocksHandler) GetFundamentals(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	payload := h.fundamentalsPayload(r.Context(), sym.ID)
	payload["symbol"] = sym.Symbol
	response.JSON(w, http.StatusOK, payload)
}

// analysisBars loads the research-eligible daily window used by technicals and risk.
func (h *StocksHandler) analysisBars(w http.ResponseWriter, r *http.Request, symbolID int) ([]database.DailyBar, int, bool) {
	bars, err := h.db.GetDailyBars(r.Context(), symbolID, analysisWindow, true)
	if err != nil {
		log.Error().Err(err).Msg("eligible bars failed")
		response.Error(w, http.StatusInternalServerError, "failed to load price history")
		return nil, 0, false
	}
	cov, err := h.db.GetBarCoverage(r.Context(), symbolID)
	if err != nil {
		log.Error().Err(err).Msg("coverage failed")
		response.Error(w, http.StatusInternalServerError, "failed to load price history")
		return nil, 0, false
	}
	return bars, cov.DailyQuarantined, true
}

func (h *StocksHandler) quantAvailable(w http.ResponseWriter) bool {
	if h.quantClient == nil {
		response.Error(w, http.StatusServiceUnavailable, "quant engine is not configured")
		return false
	}
	return true
}

func basisOf(bars []database.DailyBar, quarantined int) map[string]any {
	basis := map[string]any{
		"series": "daily", "research_eligible_only": true,
		"eligible_bars": len(bars), "quarantined_bars_excluded": quarantined,
		"last_bar": nil,
	}
	if len(bars) > 0 {
		basis["last_bar"] = bars[len(bars)-1].Timestamp.Format(time.RFC3339)
	}
	return basis
}

// GetTechnicals computes technical indicators from research-eligible bars only. Indicators that
// need more history than exists are reported as skipped, never estimated.
func (h *StocksHandler) GetTechnicals(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	bars, quarantined, ok := h.analysisBars(w, r, sym.ID)
	if !ok {
		return
	}
	compute, skipped := planIndicators(len(bars))
	resp := map[string]any{"symbol": sym.Symbol, "basis": basisOf(bars, quarantined), "skipped": skipped}

	if len(compute) == 0 {
		resp["status"] = "DATA_INSUFFICIENT"
		resp["required_bars"] = technicalPlan[0].MinBars
		resp["indicators"] = []any{}
		response.JSON(w, http.StatusOK, resp)
		return
	}
	if !h.quantAvailable(w) {
		return
	}

	in := quant.OHLCVInput{}
	for _, b := range bars {
		in.Timestamps = append(in.Timestamps, b.Timestamp.Format(time.RFC3339))
		in.Open = append(in.Open, b.Open)
		in.High = append(in.High, b.High)
		in.Low = append(in.Low, b.Low)
		in.Close = append(in.Close, b.Close)
		in.Volume = append(in.Volume, b.Volume)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	out, err := h.quantClient.CalculateIndicators(ctx, in, compute)
	if err != nil {
		log.Error().Err(err).Str("symbol", sym.Symbol).Msg("quant indicators failed")
		response.Error(w, http.StatusBadGateway, "quant engine unavailable")
		return
	}
	indicators := make([]map[string]any, 0, len(out.Indicators))
	for _, ind := range out.Indicators {
		indicators = append(indicators, map[string]any{
			"indicator": ind.Indicator, "parameters": ind.Parameters, "signal": ind.Signal,
			"latest": latestScalar(ind.Values), "values": ind.Values,
		})
	}
	resp["status"] = "AVAILABLE"
	if len(skipped) > 0 {
		resp["status"] = "PARTIAL"
	}
	resp["indicators"] = indicators
	response.JSON(w, http.StatusOK, resp)
}

// GetRiskAnalysis computes volatility, drawdown, VaR and Sharpe from research-eligible daily returns.
func (h *StocksHandler) GetRiskAnalysis(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	bars, quarantined, ok := h.analysisBars(w, r, sym.ID)
	if !ok {
		return
	}
	prices := make([]float64, 0, len(bars))
	for _, b := range bars {
		prices = append(prices, *b.AdjustedClose)
	}
	rets := simpleReturns(prices)
	resp := map[string]any{"symbol": sym.Symbol, "basis": basisOf(bars, quarantined), "observations": len(rets)}

	if len(rets) < minReturnsForRisk {
		resp["status"] = "DATA_INSUFFICIENT"
		resp["required_observations"] = minReturnsForRisk
		resp["metrics"] = nil
		response.JSON(w, http.StatusOK, resp)
		return
	}
	if !h.quantAvailable(w) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	m, err := h.quantClient.CalculateRiskMetrics(ctx, rets)
	if err != nil {
		log.Error().Err(err).Str("symbol", sym.Symbol).Msg("quant risk metrics failed")
		response.Error(w, http.StatusBadGateway, "quant engine unavailable")
		return
	}
	resp["status"] = "AVAILABLE"
	resp["metrics"] = m
	resp["disclaimer"] = response.AlphaBetaDisclaimer.Message
	response.JSON(w, http.StatusOK, resp)
}

// GetQuantAnalytics reports how much verified history exists for the instrument and therefore
// which analytics are available. It performs no calculation of its own.
func (h *StocksHandler) GetQuantAnalytics(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	cov, err := h.db.GetBarCoverage(r.Context(), sym.ID)
	if err != nil {
		log.Error().Err(err).Msg("coverage failed")
		response.Error(w, http.StatusInternalServerError, "failed to load coverage")
		return
	}
	available, unavailable := []string{}, []map[string]any{}
	for _, p := range technicalPlan {
		if cov.DailyEligible >= p.MinBars {
			available = append(available, p.Spec)
		} else {
			unavailable = append(unavailable, map[string]any{"indicator": p.Spec, "min_bars": p.MinBars})
		}
	}
	riskReady := cov.DailyEligible-1 >= minReturnsForRisk
	readiness := "LIMITED"
	if cov.DailyEligible == 0 {
		readiness = "NO_ELIGIBLE_HISTORY"
	} else if riskReady && len(unavailable) == 0 {
		readiness = "READY"
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":                 sym.Symbol,
		"readiness":              readiness,
		"coverage":               cov,
		"available_indicators":   available,
		"unavailable_indicators": unavailable,
		"risk_metrics_available": riskReady,
		"note":                   "Quarantined bars are excluded from every calculation until their eligibility is established.",
	})
}
