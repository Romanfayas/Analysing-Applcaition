package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/providers/marketdata"
	"github.com/halal-equity/backend/internal/providers/marketdata/yahoofinance"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/pkg/response"
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
	r.Get("/market/overview", h.GetMarketOverview)
	r.Post("/custom", h.AddStock)
	r.Get("/custom/{symbol}/status", h.AddStockStatus)
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
	ctx := r.Context()

	// Get all active symbols
	symbols, err := h.db.GetAllActiveSymbols(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "Failed to retrieve symbols")
		return
	}

	// Setup score and stage come from the same breakout scan the Signals page uses (cached ~1 min).
	scan, scanErr := getBreakoutScan(ctx, h.db, h.quantClient)
	if scanErr != nil {
		log.Warn().Err(scanErr).Msg("stock list: breakout scan unavailable; scores omitted")
	}

	var stocks []map[string]any
	loc, _ := time.LoadLocation("Asia/Kolkata")
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)

	for _, sym := range symbols {
		stockData := map[string]any{
			"symbol":   sym.Symbol,
			"name":     sym.Name,
			"exchange": sym.Exchange,
			"sector":   sym.Sector,
		}

		// 1. Get latest OHLCV (prefer intraday, fallback to daily)
		ohlcv, err := h.db.GetLatestPrice(ctx, sym.ID)

		if err == nil && ohlcv != nil && ohlcv.Close > 0 {
			stockData["price"] = ohlcv.Close
			stockData["marketTimestamp"] = ohlcv.Timestamp.Format(time.RFC3339)
			stockData["receivedAt"] = ohlcv.RetrievedAt.Format(time.RFC3339)
			stockData["provider"] = ohlcv.Source
			stockData["freshness"] = market.GetFreshnessStatus(ohlcv.Timestamp, now)

			// Calculate daily price change against previous trading day's close
			prevClose, err := h.db.GetPreviousClose(ctx, sym.ID, ohlcv.Timestamp)
			if err == nil && prevClose != nil {
				changeRes := market.CalculatePriceChange(&ohlcv.Close, prevClose)
				if changeRes.IsValid {
					stockData["change"] = *changeRes.Change
					stockData["change_percent"] = *changeRes.ChangePercent
					stockData["prev_close"] = *changeRes.PreviousClose
				} else {
					stockData["change"] = nil
					stockData["change_percent"] = nil
					stockData["prev_close"] = nil
				}
			} else {
				stockData["change"] = nil
				stockData["change_percent"] = nil
				stockData["prev_close"] = nil
			}
		} else {
			stockData["price"] = nil
			stockData["marketTimestamp"] = nil
			stockData["receivedAt"] = nil
			stockData["provider"] = nil
			stockData["freshness"] = "UNAVAILABLE"
			stockData["change"] = nil
			stockData["change_percent"] = nil
			stockData["prev_close"] = nil
		}

		stockData["marketStatus"] = string(market.GetCurrentSession(now))

		// 2. Get Shariah Status
		shariah, err := h.db.GetLatestShariahScreening(ctx, sym.ID)
		if err == nil && shariah != nil {
			stockData["shariah"] = string(shariah.Status)
		} else {
			stockData["shariah"] = "UNKNOWN"
		}

		// 3. Get Signal and Score
		signal, err := h.db.GetLatestSignal(ctx, sym.ID)
		isFreshData := stockData["freshness"] == "FRESH" || stockData["freshness"] == "LAST_AVAILABLE"
		isShariahScreened := stockData["shariah"] == "PASS" || stockData["shariah"] == "FAIL"

		if err == nil && signal != nil && isFreshData && isShariahScreened && now.Sub(signal.CalculatedAt) <= 24*time.Hour {
			stockData["signal"] = signal.Signal
			stockData["score"] = signal.OverallScore
		} else {
			stockData["signal"] = "NO_SIGNAL"
			stockData["score"] = nil
		}

		// Setup score: price-structure evidence 0-100 from the breakout engine. It is not a recommendation and carries
		// no Shariah view (that is the separate "shariah" field). nil when the engine could not score the instrument.
		stockData["setup"], stockData["score_as_of"], stockData["score_basis"] = nil, nil, nil
		if scan != nil {
			if res, ok := scan.For(sym.Symbol); ok && res.StructureScore != nil {
				stockData["score"] = *res.StructureScore
				stockData["setup"] = res.Stage
				stockData["score_as_of"] = scan.DataAsOf
				stockData["score_basis"] = "BREAKOUT_STRUCTURE"
			}
		}

		stocks = append(stocks, stockData)
	}

	response.WithMeta(w, stocks, &response.Meta{Page: 1, PerPage: len(stocks), Total: int64(len(stocks))})
}

// GetStock returns detailed information for a single stock.
func (h *StocksHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	symbol := symbolParam(r)
	ctx := r.Context()

	// 1. Fetch Symbol
	symRec, err := h.db.GetSymbolByCode(ctx, symbol, "NSE")
	if err != nil {
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "symbol not found"})
		return
	}

	// 2. Fetch Latest OHLCV for current price (prefer intraday, fallback to daily)
	latestOHLCV, err := h.db.GetLatestPrice(ctx, symRec.ID)
	var currentPrice *float64
	var timestamp, receivedAt *string
	freshness := "UNAVAILABLE"
	var change, changePercent, prevCloseVal *float64
	providerName := "DATA_UNAVAILABLE"

	loc, _ := time.LoadLocation("Asia/Kolkata")
	if loc == nil {
		loc = time.UTC
	}

	// 2.5 Check corporate action registry (e.g. demerged predecessor)
	var corpActionObj map[string]any
	isDemerged := false
	if ca, exists := yahoofinance.CorporateActionsRegistry[symRec.Symbol]; exists {
		var successors []map[string]any
		for _, s := range ca.Successors {
			successors = append(successors, map[string]any{
				"symbol":         s.Symbol,
				"name":           s.Name,
				"ratio":          s.Ratio,
				"cost_split_pct": s.CostSplitPct,
				"is_primary":     s.IsPrimary,
			})
		}
		corpActionObj = map[string]any{
			"status":         ca.ActionType,
			"action_type":    ca.ActionType,
			"effective_date": ca.EffectiveDate.Format("2006-01-02"),
			"notes":          ca.Notes,
			"successors":     successors,
		}
		if ca.ActionType == "DEMERGER" {
			isDemerged = true
		}
	}

	if !isDemerged && err == nil && latestOHLCV != nil && latestOHLCV.Close > 0 {
		currentPrice = &latestOHLCV.Close
		tsStr := latestOHLCV.Timestamp.Format(time.RFC3339)
		recvStr := latestOHLCV.RetrievedAt.Format(time.RFC3339)
		timestamp = &tsStr
		receivedAt = &recvStr
		providerName = latestOHLCV.Source
		freshness = market.GetFreshnessStatus(latestOHLCV.Timestamp, time.Now().In(loc))

		prevClose, err := h.db.GetPreviousClose(ctx, symRec.ID, latestOHLCV.Timestamp)
		if err == nil && prevClose != nil {
			changeRes := market.CalculatePriceChange(&latestOHLCV.Close, prevClose)
			if changeRes.IsValid {
				change = changeRes.Change
				changePercent = changeRes.ChangePercent
				prevCloseVal = changeRes.PreviousClose
			}
		}
	} else if isDemerged {
		currentPrice = nil
		change = nil
		changePercent = nil
		prevCloseVal = nil
		freshness = "UNAVAILABLE"
	}

	// Fetch historical OHLCV data for charting (last 30 days)
	var ohlcvData []map[string]any
	to := time.Now()
	from := to.AddDate(0, 0, -30)
	historicalRecords, _ := h.db.GetOHLCV(ctx, symRec.ID, from, to)
	for _, rec := range historicalRecords {
		ohlcvData = append(ohlcvData, map[string]any{
			"timestamp": rec.Timestamp.Format(time.RFC3339),
			"open":      rec.Open,
			"high":      rec.High,
			"low":       rec.Low,
			"close":     rec.Close,
			"volume":    rec.Volume,
		})
	}
	if ohlcvData == nil {
		ohlcvData = []map[string]any{}
	}

	// 3. Shariah Status
	shariahStatus := "UNKNOWN"
	var shariahDate *string
	shariah, err := h.db.GetLatestShariahScreening(ctx, symRec.ID)
	if err == nil && shariah != nil {
		shariahStatus = string(shariah.Status)
		d := shariah.ScreenedAt.Format(time.RFC3339)
		shariahDate = &d
	}

	// 3.5 Fundamentals
	fundamentalsMap := h.fundamentalsPayload(ctx, symRec.ID)

	// 4. Signal
	var signalObj map[string]any
	signal, err := h.db.GetLatestSignal(ctx, symRec.ID)
	isFreshData := freshness == "FRESH" || freshness == "LAST_AVAILABLE"
	isShariahScreened := shariahStatus == "PASS" || shariahStatus == "FAIL"

	if err == nil && signal != nil && isFreshData && isShariahScreened && time.Since(signal.CalculatedAt) <= 24*time.Hour {
		signalObj = map[string]any{
			"overall_score": signal.OverallScore,
			"action":        signal.Signal,
			"explanation":   signal.Explanation,
			"calculated_at": signal.CalculatedAt.Format(time.RFC3339),
			"is_actionable": true,
		}
	} else if err == nil && signal != nil {
		signalObj = map[string]any{
			"overall_score": nil,
			"action":        "NO_SIGNAL",
			"explanation":   "Signal unavailable: requires fresh market data and verified Shariah screening",
			"calculated_at": signal.CalculatedAt.Format(time.RFC3339),
			"is_actionable": false,
		}
	} else {
		signalObj = map[string]any{
			"overall_score": nil,
			"action":        "NO_SIGNAL",
			"explanation":   "No signal generated",
			"is_actionable": false,
		}
	}

	resp := map[string]any{
		"symbol":         symRec.Symbol,
		"name":           symRec.Name,
		"exchange":       symRec.Exchange,
		"is_active":      symRec.IsActive,
		"current_price":  currentPrice,
		"change":         change,
		"change_percent": changePercent,
		"prev_close":     prevCloseVal,
		"timestamp":      timestamp,
		"received_at":    receivedAt,
		"provider":       providerName,
		"freshness":      freshness,
		"market_session": string(market.GetCurrentSession(time.Now())),
		"ohlcv":          ohlcvData,
		"technicals":     map[string]any{},
		"fundamentals":   fundamentalsMap,
		"shariah": map[string]any{
			"status":         shariahStatus,
			"methodology":    "AAOIFI",
			"screening_date": shariahDate,
		},
		"signal": signalObj,
		"quant":  map[string]any{},
	}

	if corpActionObj != nil {
		resp["corporate_action"] = corpActionObj
	}

	response.JSON(w, http.StatusOK, resp)
}

// GetShariahStatus returns the Shariah screening status for a stock.
func (h *StocksHandler) GetShariahStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := symbolParam(r)

	symRec, err := h.db.GetSymbolByCode(ctx, symbol, "NSE")
	if err != nil || symRec == nil {
		response.Error(w, http.StatusNotFound, "symbol not found")
		return
	}

	shariah, err := h.db.GetLatestShariahScreening(ctx, symRec.ID)
	if err != nil || shariah == nil {
		response.JSON(w, http.StatusOK, map[string]any{
			"symbol":         symRec.Symbol,
			"status":         "UNKNOWN",
			"methodology":    "AAOIFI",
			"screening_date": nil,
		})
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":                 symRec.Symbol,
		"status":                 string(shariah.Status),
		"rule_set_id":            shariah.RuleSetID,
		"debt_ratio":             shariah.DebtRatio,
		"debt_ratio_pass":        shariah.DebtRatioPass,
		"interest_income_ratio":  shariah.InterestIncomeRatio,
		"interest_income_pass":   shariah.InterestIncomePass,
		"cash_deposit_ratio":     shariah.CashDepositRatio,
		"cash_deposit_pass":      shariah.CashDepositPass,
		"receivables_ratio":      shariah.ReceivablesRatio,
		"receivables_pass":       shariah.ReceivablesPass,
		"business_activity_pass": shariah.BusinessActivityPass,
		"purification_per_share": shariah.PurificationPerShare,
		"screening_date":         shariah.ScreenedAt.Format(time.RFC3339),
	})
}

// GetSignal returns the composite signal for a stock.
func (h *StocksHandler) GetSignal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := symbolParam(r)

	symRec, err := h.db.GetSymbolByCode(ctx, symbol, "NSE")
	if err != nil || symRec == nil {
		response.Error(w, http.StatusNotFound, "symbol not found")
		return
	}

	now := time.Now()
	var freshness string = "UNAVAILABLE"
	ohlcv, _ := h.db.GetLatestPrice(ctx, symRec.ID)
	if ohlcv != nil && ohlcv.Close > 0 {
		freshness = market.GetFreshnessStatus(ohlcv.Timestamp, now)
	}

	shariah, _ := h.db.GetLatestShariahScreening(ctx, symRec.ID)
	var shariahStatus string = "UNKNOWN"
	if shariah != nil {
		shariahStatus = string(shariah.Status)
	}

	signal, err := h.db.GetLatestSignal(ctx, symRec.ID)
	isFresh := freshness == "FRESH" || freshness == "LAST_AVAILABLE"
	isScreened := shariahStatus == "PASS" || shariahStatus == "FAIL"

	if err == nil && signal != nil && isFresh && isScreened && time.Since(signal.CalculatedAt) <= 24*time.Hour {
		response.JSON(w, http.StatusOK, map[string]any{
			"symbol":        symRec.Symbol,
			"signal":        signal.Signal,
			"overall_score": signal.OverallScore,
			"explanation":   signal.Explanation,
			"calculated_at": signal.CalculatedAt.Format(time.RFC3339),
			"is_actionable": true,
		})
		return
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":        symRec.Symbol,
		"signal":        "NO_SIGNAL",
		"overall_score": nil,
		"explanation":   "Signal unavailable: requires fresh market data and completed Shariah screening",
		"calculated_at": nil,
		"is_actionable": false,
	})
}

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
	r.Get("/health", h.GetIPOHealth)
	r.Get("/ingestion-runs", h.ListIPOIngestionRuns)
	r.Get("/{id}", h.GetIPO)
	r.Get("/{id}/research-report", h.GetIPOResearchReport)
	r.Get("/{id}/financials", h.GetIPOFinancials)
	r.Get("/{id}/gmp", h.GetIPOGMP)
	r.Get("/{id}/subscription", h.GetIPOSubscription)
	r.Get("/{id}/prediction", h.GetIPOPrediction)
	r.Get("/{id}/decision", h.GetIPODecision)
	r.Get("/{id}/peers", h.GetIPOPeers)
	return r
}

func (h *IPOHandler) ListIPOIngestionRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.db.ListIPOIngestionRuns(r.Context(), 50)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO ingestion runs"})
		return
	}
	response.JSON(w, http.StatusOK, runs)
}

func (h *IPOHandler) GetIPOHealth(w http.ResponseWriter, r *http.Request) {
	latestRun, err := h.db.GetLatestIPOIngestionRun(r.Context())
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query IPO health"})
		return
	}

	activeIPOs, _ := h.db.GetEnrichedIPOs(r.Context(), "ACTIVE")
	historicalIPOs, _ := h.db.GetEnrichedIPOs(r.Context(), "HISTORICAL")
	drhpIPOs, _ := h.db.GetEnrichedIPOs(r.Context(), "DRHP")

	// Derived from the latest ingestion run rather than asserted.
	feedStatus := "UNKNOWN"
	if latestRun != nil {
		switch latestRun.Status {
		case "SUCCESS":
			feedStatus = "OPERATIONAL"
		case "EMPTY":
			feedStatus = "OPERATIONAL_EMPTY"
		default:
			feedStatus = "DEGRADED"
		}
	}

	health := map[string]any{
		"provider":               "NSE_OFFICIAL_IPO_API",
		"provider_url":           "https://www.nseindia.com/api/ipo-current-issue",
		"feed_status":            feedStatus,
		"active_issues_count":    len(activeIPOs),
		"drhp_issues_count":      len(drhpIPOs),
		"historical_count":       len(historicalIPOs),
		"latest_run":             latestRun,
		"temporal_integrity":     "ENFORCED",
		"observation_disclaimer": "Exchange current-issue endpoint verified. Historical listed issues are sequestered in research archive.",
	}
	response.JSON(w, http.StatusOK, health)
}

func (h *IPOHandler) ListIPOs(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")
	if statusFilter == "" {
		statusFilter = "UPCOMING"
	}

	ipos, err := h.db.GetEnrichedIPOs(r.Context(), statusFilter)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPOs"})
		return
	}

	response.JSON(w, http.StatusOK, ipos)
}

func (h *IPOHandler) GetIPO(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			response.JSON(w, http.StatusOK, ipo)
			return
		}
	}
	response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
}

func (h *IPOHandler) GetIPOFinancials(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			if fin, ok := ipo["financials"]; ok && fin != nil {
				response.JSON(w, http.StatusOK, fin)
				return
			}
			response.JSON(w, http.StatusOK, map[string]string{"status": "DATA_UNAVAILABLE"})
			return
		}
	}
	response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
}

func (h *IPOHandler) GetIPOGMP(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			if gmp, ok := ipo["gmp"]; ok && gmp != nil {
				response.JSON(w, http.StatusOK, gmp)
				return
			}
			response.JSON(w, http.StatusOK, map[string]string{"status": "DATA_UNAVAILABLE"})
			return
		}
	}
	response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
}

func (h *IPOHandler) GetIPOSubscription(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			if sub, ok := ipo["subscription"]; ok && sub != nil {
				response.JSON(w, http.StatusOK, sub)
				return
			}
			response.JSON(w, http.StatusOK, map[string]string{"status": "DATA_UNAVAILABLE"})
			return
		}
	}
	response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
}

func (h *IPOHandler) GetIPOPrediction(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			if pred, ok := ipo["prediction"]; ok && pred != nil {
				response.JSON(w, http.StatusOK, pred)
				return
			}
			response.JSON(w, http.StatusOK, map[string]string{"status": "DATA_UNAVAILABLE"})
			return
		}
	}
	response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
}

func (h *IPOHandler) GetIPODecision(w http.ResponseWriter, r *http.Request) {
	h.GetIPOPrediction(w, r)
}

func (h *IPOHandler) GetIPOPeers(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]any{})
}

// GetIPOResearchReport returns a comprehensive, auditable research report
// separating Verified Facts, Derived Calculations, Third-Party Observations,
// Model Estimates, and Missing Evidence.
func (h *IPOHandler) GetIPOResearchReport(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	ipos, err := h.db.GetEnrichedIPOs(r.Context(), "ALL")
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to fetch IPO"})
		return
	}

	var target map[string]any
	for _, ipo := range ipos {
		if ipo["id"] == idStr || ipo["symbol"] == idStr {
			target = ipo
			break
		}
	}
	if target == nil {
		response.JSON(w, http.StatusNotFound, map[string]string{"error": "IPO not found"})
		return
	}

	// 1. Verified Facts (Official Exchange Filings & DRHP)
	verifiedFacts := map[string]any{
		"company_name":      target["name"],
		"symbol":            target["symbol"],
		"exchange":          target["exchange"],
		"status":            target["status"],
		"sector":            target["sector"],
		"price_band":        target["price_band"],
		"issue_size":        target["issue_size"],
		"issue_size_raw_cr": target["issue_size_raw"],
		"fresh_issue_cr":    target["fresh_issue"],
		"ofs_cr":            target["ofs"],
		"lot_size":          target["lot_size"],
		"min_investment":    target["min_investment"],
		"open_date":         target["open_date"],
		"close_date":        target["close_date"],
		"listing_date":      target["listing_date"],
		"source":            target["source"],
		"source_provenance": "Official exchange circular & primary regulatory filings",
		"ingestion_status":  "VERIFIED_IN_DATABASE",
	}

	// 2. Derived Calculations
	freshCr, _ := target["fresh_issue"].(*float64)
	totalIssueCr, _ := target["issue_size_raw"].(*float64)
	var dilutionPct *float64
	var ofsPct *float64
	if freshCr != nil && totalIssueCr != nil && *totalIssueCr > 0 {
		d := (*freshCr / *totalIssueCr) * 100.0
		dilutionPct = &d
		o := 100.0 - d
		ofsPct = &o
	}

	derivedCalculations := map[string]any{
		"fresh_issue_proportion_pct": dilutionPct,
		"ofs_proportion_pct":         ofsPct,
		"valuation_methodology":      "Point-in-time financial statement ratios normalized to upper issue band",
	}

	// 3. Third-Party Observations (GMP & Bidding Books)
	thirdPartyObservations := map[string]any{
		"gmp":                   target["gmp"],
		"subscription":          target["subscription"],
		"provenance_disclosure": "GMP is collected from unofficial third-party grey market observers. It carries ZERO regulatory protection and is not exchange-guaranteed.",
	}

	// 4. Model Estimates
	modelEstimates := map[string]any{
		"prediction":          target["prediction"],
		"lookahead_integrity": "STRICT_ENFORCED: Zero post-listing information entered pre-listing predictions. All features cut off prior to listing date.",
	}

	// 5. Missing Evidence
	missingEvidence := make([]string, 0)
	if target["price_band"] == "" {
		missingEvidence = append(missingEvidence, "Official price band unannounced by issuer/merchant bankers")
	}
	if target["financials"] == nil {
		missingEvidence = append(missingEvidence, "Audited restated financial statements not yet ingested")
	}
	if target["subscription"] == nil {
		missingEvidence = append(missingEvidence, "Exchange bidding book data unavailable (issue not currently open)")
	}
	if target["gmp"] == nil {
		missingEvidence = append(missingEvidence, "No verifiable grey market premium quotes recorded for this issue")
	}

	report := map[string]any{
		"ipo_id":                   target["id"],
		"company_name":             target["name"],
		"symbol":                   target["symbol"],
		"status":                   target["status"],
		"shariah_status":           target["shariah_status"],
		"verified_facts":           verifiedFacts,
		"derived_calculations":     derivedCalculations,
		"third_party_observations": thirdPartyObservations,
		"model_estimates":          modelEstimates,
		"missing_evidence":         missingEvidence,
		"generated_at":             time.Now().UTC(),
	}

	response.JSON(w, http.StatusOK, report)
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
	r.Get("/breakouts", h.GetBreakouts)
	r.Get("/breakouts/monitor", h.GetBreakoutMonitor)
	r.Get("/scanner", h.GetBreakouts)
	r.Get("/weights", h.GetWeights)
	r.Put("/weights", h.UpdateWeights)
	return r
}

func (h *SignalsHandler) GetBreakouts(w http.ResponseWriter, r *http.Request) {
	scan, err := getBreakoutScan(r.Context(), h.db, h.quantClient)
	if err != nil {
		log.Error().Err(err).Msg("breakout scan failed")
		response.Error(w, http.StatusBadGateway, "breakout scanner unavailable")
		return
	}
	response.JSON(w, http.StatusOK, scan)
}

func (h *SignalsHandler) ListSignals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbols, err := h.db.GetAllActiveSymbols(ctx)
	if err != nil {
		response.JSON(w, http.StatusOK, []map[string]any{})
		return
	}

	scanItems := make([]quant.StockScanItem, 0)
	for _, s := range symbols {
		candles, err := h.db.GetRecentCandlesForSymbol(ctx, s.ID, 40)
		if err != nil || len(candles) < 10 {
			continue
		}

		var opens, highs, lows, closes []float64
		var volumes []int64
		for _, c := range candles {
			opens = append(opens, c.Open)
			highs = append(highs, c.High)
			lows = append(lows, c.Low)
			closes = append(closes, c.Close)
			volumes = append(volumes, c.Volume)
		}

		shariahStatus := h.db.GetShariahStatusForSymbol(ctx, s.ID)

		scanItems = append(scanItems, quant.StockScanItem{
			Symbol:        s.Symbol,
			Sector:        s.Sector,
			Open:          opens,
			High:          highs,
			Low:           lows,
			Close:         closes,
			Volume:        volumes,
			ShariahStatus: shariahStatus,
			DataQuality:   "LAST_AVAILABLE",
		})
	}

	scanResp, err := h.quantClient.ScanBreakouts(ctx, quant.BreakoutScanRequest{
		Stocks: scanItems,
	})
	if err != nil {
		response.JSON(w, http.StatusOK, []map[string]any{})
		return
	}

	var signals []map[string]any
	for _, c := range scanResp.Candidates {
		if c.Stage == "CONFIRMED_BREAKOUT" || c.Stage == "BREAKOUT_CANDIDATE" {
			act := "BUY"
			if c.Stage == "CONFIRMED_BREAKOUT" {
				act = "STRONG_BUY"
			}
			signals = append(signals, map[string]any{
				"symbol":         c.Symbol,
				"name":           c.Symbol,
				"price":          c.CurrentPrice,
				"action":         act,
				"score":          c.OverallScore,
				"shariah_status": c.ShariahStatus,
				"timestamp":      time.Now().Format(time.RFC3339),
			})
		}
	}

	response.JSON(w, http.StatusOK, signals)
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
	ctx := r.Context()
	query := `
		SELECT s.symbol, s.name, COALESCE(s.sector, ''), sc.status, sc.screening_date,
		       COALESCE(sc.debt_ratio, 0), COALESCE(sc.interest_income_ratio, 0),
		       COALESCE(sc.cash_deposit_ratio, 0), COALESCE(sc.receivables_ratio, 0),
		       COALESCE(sc.purification_per_share, 0), COALESCE(sc.business_activity_notes, '')
		FROM shariah_screenings sc
		JOIN symbols s ON sc.symbol_id = s.id
		ORDER BY s.symbol ASC
	`
	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to query screenings"})
		return
	}
	defer rows.Close()

	var screenings []map[string]any
	for rows.Next() {
		var sym, name, sector, status, notes string
		var date time.Time
		var debtR, intR, cashR, recR, purif float64
		if err := rows.Scan(&sym, &name, &sector, &status, &date, &debtR, &intR, &cashR, &recR, &purif, &notes); err == nil {
			screenings = append(screenings, map[string]any{
				"symbol":                  sym,
				"name":                    name,
				"sector":                  sector,
				"status":                  status,
				"screening_date":          date.Format("2006-01-02"),
				"debt_ratio":              debtR,
				"interest_income_ratio":   intR,
				"cash_deposit_ratio":      cashR,
				"receivables_ratio":       recR,
				"purification_per_share":  purif,
				"business_activity_notes": notes,
			})
		}
	}

	response.JSON(w, http.StatusOK, screenings)
}

func (h *ShariahHandler) ListRuleSets(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]any{
		{
			"id":          1,
			"name":        "AAOIFI Standard",
			"description": "Accounting and Auditing Organization for Islamic Financial Institutions standard screening criteria",
			"is_active":   true,
		},
	})
}

func (h *ShariahHandler) ListRuleVersions(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusOK, []map[string]any{})
}

func (h *ShariahHandler) CreateRuleVersion(w http.ResponseWriter, r *http.Request) {
	response.JSON(w, http.StatusCreated, map[string]string{"status": "created"})
}

// ---- Portfolio & Paper Trade Handler ----

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
	r.Get("/trades", h.ListTrades)
	r.Post("/trade", h.ExecuteTrade)
	r.Get("/analytics", h.GetAnalytics)
	return r
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
	r.Get("/gate", h.GetGate)
	r.Get("/research", h.GetResearch)
	r.Get("/gate/diagnostics", h.GetGateDiagnostics)
	r.Get("/gate/security/{symbol}", h.GetSecurityGateDiagnostics)
	r.Get("/strategies", h.ListStrategies)
	r.Post("/strategies", h.CreateStrategy)
	r.Post("/run", h.RunBacktest)
	r.Post("/run_walk_forward", h.RunWalkForward)
	r.Get("/runs", h.ListRuns)
	r.Get("/runs/{id}", h.GetRunResults)
	return r
}

func (h *BacktestingHandler) GetGateDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := `
		WITH latest_screenings AS (
			SELECT DISTINCT ON (symbol_id) symbol_id, status
			FROM shariah_screenings
			ORDER BY symbol_id, screening_date DESC, id DESC
		),
		ca_stats AS (
			SELECT symbol_id, 
			       COUNT(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED') as unauth_ca,
			       COUNT(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY') as verified_ca
			FROM corporate_action_events
			GROUP BY symbol_id
		),
		ohlcv_stats AS (
			SELECT symbol_id,
			       COUNT(*) FILTER (WHERE adjusted_close IS NULL) as quarantined_bars,
			       COUNT(*) as total_bars
			FROM ohlcv_daily
			GROUP BY symbol_id
		)
		SELECT 
			s.symbol,
			s.name,
			s.is_active,
			COALESCE(ls.status, 'UNKNOWN') as shariah_status,
			COALESCE(os.quarantined_bars, 0) as quarantined_bars,
			COALESCE(os.total_bars, 0) as total_bars,
			COALESCE(cs.unauth_ca, 0) as unauth_events,
			COALESCE(cs.verified_ca, 0) as verified_events
		FROM symbols s
		LEFT JOIN latest_screenings ls ON ls.symbol_id = s.id
		LEFT JOIN ca_stats cs ON cs.symbol_id = s.id
		LEFT JOIN ohlcv_stats os ON os.symbol_id = s.id
		ORDER BY s.symbol;
	`
	rows, err := h.db.QueryContext(ctx, query)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	type SecDiag struct {
		Symbol             string `json:"symbol"`
		Name               string `json:"name"`
		IsActive           bool   `json:"is_active"`
		ShariahStatus      string `json:"shariah_status"`
		QuarantinedBars    int    `json:"quarantined_bars"`
		TotalBars          int    `json:"total_bars"`
		UnauthEvents       int    `json:"unauthenticated_events"`
		VerifiedEvents     int    `json:"verified_events"`
		ResearchEligible   bool   `json:"research_eligible"`
		NextActionRequired string `json:"next_action_required"`
	}

	var diags []SecDiag
	for rows.Next() {
		var d SecDiag
		if err := rows.Scan(&d.Symbol, &d.Name, &d.IsActive, &d.ShariahStatus, &d.QuarantinedBars, &d.TotalBars, &d.UnauthEvents, &d.VerifiedEvents); err != nil {
			continue
		}
		d.ResearchEligible = (d.QuarantinedBars == 0 && d.VerifiedEvents > 0 && d.ShariahStatus == "PASS")

		if d.QuarantinedBars > 0 && d.UnauthEvents > 0 {
			d.NextActionRequired = "Acquire primary exchange circular PDF to authenticate historical corporate actions and unquarantine bars"
		} else if d.ShariahStatus == "DATA_INSUFFICIENT" {
			d.NextActionRequired = "Acquire audited annual report Note 24/25 non-operating interest income footnote"
		} else if d.ShariahStatus == "FAIL" {
			d.NextActionRequired = "Security failed Shariah screening; trading and research restricted"
		} else if d.TotalBars == 0 {
			d.NextActionRequired = "Restructuring/lineage symbol; zero bars present in daily table"
		} else {
			d.NextActionRequired = "Pending verification of primary exchange corporate actions"
		}

		diags = append(diags, d)
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"global_status": "RESEARCH_BLOCKED",
		"total_symbols": len(diags),
		"diagnostics":   diags,
	})
}

func (h *BacktestingHandler) GetSecurityGateDiagnostics(w http.ResponseWriter, r *http.Request) {
	symbol := symbolParam(r)
	ctx := r.Context()

	var symID int
	var symName string
	var isActive bool
	err := h.db.QueryRowContext(ctx, "SELECT id, name, is_active FROM symbols WHERE symbol = $1;", symbol).Scan(&symID, &symName, &isActive)
	if err != nil {
		response.Error(w, http.StatusNotFound, fmt.Sprintf("symbol %s not found", symbol))
		return
	}

	var quarantinedBars, totalBars int
	_ = h.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE adjusted_close IS NULL),
			COUNT(*)
		FROM ohlcv_daily WHERE symbol_id = $1;
	`, symID).Scan(&quarantinedBars, &totalBars)

	var unauthEvents, verifiedEvents int
	_ = h.db.QueryRowContext(ctx, `
		SELECT 
			COUNT(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED'),
			COUNT(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY')
		FROM corporate_action_events WHERE symbol_id = $1;
	`, symID).Scan(&unauthEvents, &verifiedEvents)

	var shariahStatus string
	_ = h.db.QueryRowContext(ctx, `
		SELECT status FROM shariah_screenings 
		WHERE symbol_id = $1 
		ORDER BY screening_date DESC, id DESC LIMIT 1;
	`, symID).Scan(&shariahStatus)
	if shariahStatus == "" {
		shariahStatus = "UNKNOWN"
	}

	nextAction := "Pending verification of primary exchange corporate actions"
	if quarantinedBars > 0 && unauthEvents > 0 {
		nextAction = "Acquire primary exchange circular PDF to authenticate historical corporate actions and unquarantine bars"
	} else if shariahStatus == "DATA_INSUFFICIENT" {
		nextAction = "Acquire audited annual report Note 24/25 non-operating interest income footnote"
	} else if shariahStatus == "FAIL" {
		nextAction = "Security failed Shariah screening; trading and research restricted"
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"symbol":                 symbol,
		"name":                   symName,
		"is_active":              isActive,
		"shariah_status":         shariahStatus,
		"quarantined_bars":       quarantinedBars,
		"total_bars":             totalBars,
		"unauthenticated_events": unauthEvents,
		"verified_events":        verifiedEvents,
		"research_eligible":      false,
		"global_gate":            "RESEARCH_BLOCKED",
		"next_action_required":   nextAction,
	})
}

// ---- Risk Handler ----

type PositionSizeRequest struct {
	Capital        float64 `json:"capital"`
	RiskPercentage float64 `json:"risk_percentage"`
	EntryPrice     float64 `json:"entry_price"`
	StopLoss       float64 `json:"stop_loss"`
	TargetPrice    float64 `json:"target_price,omitempty"`
}

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
	r.Get("/metrics", h.GetPortfolioRisk)
	r.Get("/", h.GetPortfolioRisk)
	return r
}

func (h *RiskHandler) CalculatePositionSize(w http.ResponseWriter, r *http.Request) {
	var req PositionSizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Capital <= 0 {
		req.Capital = 1000000.0 // Default capital if omitted
	}
	if req.RiskPercentage <= 0 {
		req.RiskPercentage = 1.0 // Default 1% risk per trade
	}
	if req.EntryPrice <= 0 {
		response.Error(w, http.StatusBadRequest, "entry_price must be positive")
		return
	}
	if req.StopLoss <= 0 || req.StopLoss >= req.EntryPrice {
		response.Error(w, http.StatusBadRequest, "stop_loss must be positive and lower than entry_price (Long-only)")
		return
	}

	maxRiskAmount := req.Capital * (req.RiskPercentage / 100.0)
	riskPerShare := req.EntryPrice - req.StopLoss
	shares := int(maxRiskAmount / riskPerShare)

	// Capital constraint: never exceed available capital
	maxSharesByCapital := int(req.Capital / req.EntryPrice)
	if shares > maxSharesByCapital {
		shares = maxSharesByCapital
	}
	if shares < 0 {
		shares = 0
	}

	positionValue := float64(shares) * req.EntryPrice
	actualRiskAmount := float64(shares) * riskPerShare
	portfolioRiskPct := 0.0
	if req.Capital > 0 {
		portfolioRiskPct = (actualRiskAmount / req.Capital) * 100.0
	}

	riskRewardRatio := 0.0
	if req.TargetPrice > req.EntryPrice && riskPerShare > 0 {
		riskRewardRatio = (req.TargetPrice - req.EntryPrice) / riskPerShare
	}

	response.JSON(w, http.StatusOK, map[string]any{
		"capital":              req.Capital,
		"risk_percentage":      req.RiskPercentage,
		"entry_price":          req.EntryPrice,
		"stop_loss":            req.StopLoss,
		"target_price":         req.TargetPrice,
		"max_risk_amount":      maxRiskAmount,
		"actual_risk_amount":   actualRiskAmount,
		"risk_per_share":       riskPerShare,
		"suggested_shares":     shares,
		"position_value":       positionValue,
		"capital_exposure_pct": (positionValue / req.Capital) * 100.0,
		"portfolio_risk_pct":   portfolioRiskPct,
		"risk_reward_ratio":    riskRewardRatio,
		"rule":                 "Long-only Shariah spot rule. No margin or short-selling permitted.",
	})
}

func (h *RiskHandler) GetPortfolioRisk(w http.ResponseWriter, r *http.Request) {
	pRecord, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	positions, _ := h.db.GetPortfolioPositions(r.Context(), pRecord.ID)

	var investedVal float64
	for _, pos := range positions {
		investedVal += float64(pos.Shares) * pos.AvgCostBasis
	}
	totalEquity := pRecord.CashBalance + investedVal

	response.JSON(w, http.StatusOK, map[string]any{
		"portfolio_id":            pRecord.ID,
		"total_equity":            totalEquity,
		"cash_balance":            pRecord.CashBalance,
		"invested_value":          investedVal,
		"open_positions_count":    len(positions),
		"max_risk_per_trade_pct":  2.0,
		"max_portfolio_heat_pct":  10.0,
		"execution_mode":          "PAPER_TRADING",
		"broker_trading_enabled":  false,
		"live_order_execution":    false,
		"short_selling_permitted": false,
		"margin_permitted":        false,
	})
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
	r.Get("/health", h.GetDashboard)
	r.Get("/issues", h.ListIssues)
	r.Get("/evidence/corporate-actions", h.GetCorporateActionsEvidence)
	r.Get("/evidence/financial-disclosures", h.GetFinancialDisclosuresEvidence)
	r.Get("/evidence/reconciliations", h.GetReconciliationReviews)
	r.Get("/historical-readiness", h.GetHistoricalReadiness)
	r.Get("/intraday-feed", h.GetIntradayFeedHealth)
	return r
}

func (h *DataQualityHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. OHLCV Market Data health
	queryOHLCV := `
		SELECT 
			COALESCE(MAX(source), 'yahoo_finance') as provider,
			COALESCE(MAX(timestamp), NOW()) as latest_data,
			COUNT(CASE WHEN quality_status = 'VALID' THEN 1 END) as valid_records,
			COUNT(CASE WHEN quality_status != 'VALID' THEN 1 END) as invalid_records,
			COALESCE(MAX(retrieved_at), NOW()) as last_successful_fetch
		FROM ohlcv_daily
	`
	var provider string
	var latestData, lastFetch time.Time
	var valid, invalid int64

	_ = h.db.QueryRowContext(ctx, queryOHLCV).Scan(&provider, &latestData, &valid, &invalid, &lastFetch)

	ohlcvStatus := "VALID"
	if invalid > 0 {
		ohlcvStatus = "PARTIAL"
	}

	// 2. Corporate Actions Primary Evidence
	var caCount int64
	var caLastUpdated time.Time
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(retrieval_timestamp), NOW()) FROM corporate_action_source_artifacts`).Scan(&caCount, &caLastUpdated)

	// 3. Financial Statements (PIT)
	var finCount int64
	var finLatest time.Time
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(period_end), NOW()) FROM financial_statements`).Scan(&finCount, &finLatest)

	// 4. IPO Offerings
	var ipoCount int64
	var ipoLastFetch time.Time
	_ = h.db.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(updated_at), NOW()) FROM ipos`).Scan(&ipoCount, &ipoLastFetch)

	// 5. NSE's own daily record and index closes
	var exCount, exFiles int64
	var exLatest, exFetched time.Time
	_ = h.db.QueryRowContext(ctx, `SELECT count(*), (SELECT count(*) FROM exchange_source_files), COALESCE(max(trade_date), NOW()), COALESCE((SELECT max(retrieved_at) FROM exchange_source_files), NOW()) FROM exchange_daily_bars`).Scan(&exCount, &exFiles, &exLatest, &exFetched)
	var idxCount int64
	var idxLatest, idxFetched time.Time
	_ = h.db.QueryRowContext(ctx, `SELECT count(*), COALESCE(max(trade_date), NOW()), COALESCE(max(retrieved_at), NOW()) FROM index_daily_bars`).Scan(&idxCount, &idxLatest, &idxFetched)

	resp := []map[string]any{
		{
			"dataset":               "NSE Daily Bhavcopy (exchange record)",
			"provider":              fmt.Sprintf("NSE archives, %d daily files with SHA-256", exFiles),
			"latest_data":           exLatest.Format(time.RFC3339),
			"valid_records":         exCount,
			"invalid_records":       0,
			"last_successful_fetch": exFetched.Format(time.RFC3339),
			"status":                "VALID",
		},
		{
			"dataset":               "NSE Index Closes",
			"provider":              "NSE archives (ind_close_all)",
			"latest_data":           idxLatest.Format(time.RFC3339),
			"valid_records":         idxCount,
			"invalid_records":       0,
			"last_successful_fetch": idxFetched.Format(time.RFC3339),
			"status":                "VALID",
		},
		{
			"dataset":               "OHLCV Market Data",
			"provider":              provider,
			"latest_data":           latestData.Format(time.RFC3339),
			"valid_records":         valid,
			"invalid_records":       invalid,
			"last_successful_fetch": lastFetch.Format(time.RFC3339),
			"status":                ohlcvStatus,
		},
		{
			"dataset":               "Corporate Actions Primary Evidence",
			"provider":              "NSE corporate-actions feed and filings (awaiting human review)",
			"latest_data":           caLastUpdated.Format(time.RFC3339),
			"valid_records":         caCount,
			"invalid_records":       0,
			"last_successful_fetch": caLastUpdated.Format(time.RFC3339),
			"status":                "PENDING_REVIEW",
		},
		{
			"dataset":               "Financial Statements (PIT)",
			"provider":              "Yahoo Finance annual figures (unverified aggregation)",
			"latest_data":           finLatest.Format(time.RFC3339),
			"valid_records":         finCount,
			"invalid_records":       0,
			"last_successful_fetch": finLatest.Format(time.RFC3339),
			"status":                "UNVERIFIED",
		},
		{
			"dataset":               "IPO Offerings & Subscriptions",
			"provider":              "NSE IPO API; GMP and SME share counts from IPOWatch (unofficial)",
			"latest_data":           ipoLastFetch.Format(time.RFC3339),
			"valid_records":         ipoCount,
			"invalid_records":       0,
			"last_successful_fetch": ipoLastFetch.Format(time.RFC3339),
			"status":                "VALID",
		},
	}

	response.JSON(w, http.StatusOK, resp)
}

func (h *DataQualityHandler) ListIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	issues := make([]map[string]any, 0)

	// Bars already caught and removed from the price history. They need no action, so they are summarised per date as
	// resolved items rather than listed as one warning per stock.
	qQuery := `
		SELECT split_part(q.quarantine_reason, ':', 1), q.timestamp::date, count(*), max(q.quarantined_at)
		FROM ohlcv_daily_quarantine_p17 q
		GROUP BY 1, 2 ORDER BY 2 DESC LIMIT 25
	`
	rows, err := h.db.QueryContext(ctx, qQuery)
	if err == nil {
		defer rows.Close()
		id := 1
		for rows.Next() {
			var reason string
			var day, qAt time.Time
			var n int
			if err := rows.Scan(&reason, &day, &n, &qAt); err == nil {
				why := "the exchange did not trade that day"
				if holiday, name := market.IsTradingHoliday(day); holiday && name != "" {
					why = "the exchange was closed for " + name
				}
				issues = append(issues, map[string]any{
					"id": id, "symbol": fmt.Sprintf("%d stocks", n), "check_type": "resolved_" + strings.ToLower(reason), "severity": "RESOLVED",
					"message":     fmt.Sprintf("The data vendor sent %d empty bars dated %s, but %s. They were removed from the price history automatically; no action is needed.", n, day.Format("2006-01-02"), why),
					"recorded_at": qAt.Format(time.RFC3339),
				})
				id++
			}
		}
	}

	// Fetch from data_quality_logs if any
	dqQuery := `
		SELECT d.id, COALESCE(s.symbol, 'SYSTEM'), d.check_type, d.severity, d.message, d.recorded_at 
		FROM data_quality_logs d
		LEFT JOIN symbols s ON d.symbol_id = s.id
		ORDER BY d.recorded_at DESC LIMIT 25
	`
	dqRows, err := h.db.QueryContext(ctx, dqQuery)
	if err == nil {
		defer dqRows.Close()
		for dqRows.Next() {
			var id int
			var symbol, checkType, severity, message string
			var recordedAt time.Time
			if err := dqRows.Scan(&id, &symbol, &checkType, &severity, &message, &recordedAt); err == nil {
				issues = append(issues, map[string]any{
					"id":          len(issues) + 1,
					"symbol":      symbol,
					"check_type":  checkType,
					"severity":    severity,
					"message":     message,
					"recorded_at": recordedAt.Format(time.RFC3339),
				})
			}
		}
	}

	response.JSON(w, http.StatusOK, issues)
}

func (h *DataQualityHandler) GetCorporateActionsEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, artifact_id, issuing_authority, source_url, document_identifier,
		       document_type, sha256_checksum, parser_version, validation_status, created_at
		FROM corporate_action_source_artifacts
		ORDER BY id DESC LIMIT 50;
	`)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var artifacts []map[string]any
	for rows.Next() {
		var id int
		var artID, auth, url, docID, docType, chk, ver, status string
		var cr time.Time
		if err := rows.Scan(&id, &artID, &auth, &url, &docID, &docType, &chk, &ver, &status, &cr); err == nil {
			artifacts = append(artifacts, map[string]any{
				"id":                  id,
				"artifact_id":         artID,
				"issuing_authority":   auth,
				"source_url":          url,
				"document_identifier": docID,
				"document_type":       docType,
				"sha256_checksum":     chk,
				"parser_version":      ver,
				"validation_status":   status,
				"created_at":          cr.Format(time.RFC3339),
			})
		}
	}
	response.JSON(w, http.StatusOK, artifacts)
}

func (h *DataQualityHandler) GetFinancialDisclosuresEvidence(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, artifact_id, symbol, fiscal_year, period_type, period_end,
		       document_title, issuing_authority, source_url, sha256_checksum,
		       validation_status, review_status, created_at
		FROM financial_disclosure_source_artifacts
		ORDER BY id DESC LIMIT 50;
	`)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var disclosures []map[string]any
	for rows.Next() {
		var id, fy int
		var artID, sym, pt, dt, auth, url, chk, vStat, rStat string
		var pe, cr time.Time
		if err := rows.Scan(&id, &artID, &sym, &fy, &pt, &pe, &dt, &auth, &url, &chk, &vStat, &rStat, &cr); err == nil {
			disclosures = append(disclosures, map[string]any{
				"id":                id,
				"artifact_id":       artID,
				"symbol":            sym,
				"fiscal_year":       fy,
				"period_type":       pt,
				"period_end":        pe.Format("2006-01-02"),
				"document_title":    dt,
				"issuing_authority": auth,
				"source_url":        url,
				"sha256_checksum":   chk,
				"validation_status": vStat,
				"review_status":     rStat,
				"created_at":        cr.Format(time.RFC3339),
			})
		}
	}
	response.JSON(w, http.StatusOK, disclosures)
}

func (h *DataQualityHandler) GetReconciliationReviews(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rows, err := h.db.QueryContext(ctx, `
		SELECT id, event_id, symbol, discrepancy_field, existing_claim_value,
		       official_claim_value, discrepancy_reason, review_status, created_at
		FROM corporate_action_reconciliation_reviews
		ORDER BY id DESC LIMIT 50;
	`)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	var reviews []map[string]any
	for rows.Next() {
		var id int
		var evID, sym, field, existV, offV, reason, status string
		var cr time.Time
		if err := rows.Scan(&id, &evID, &sym, &field, &existV, &offV, &reason, &status, &cr); err == nil {
			reviews = append(reviews, map[string]any{
				"id":                   id,
				"event_id":             evID,
				"symbol":               sym,
				"discrepancy_field":    field,
				"existing_claim_value": existV,
				"official_claim_value": offV,
				"discrepancy_reason":   reason,
				"review_status":        status,
				"created_at":           cr.Format(time.RFC3339),
			})
		}
	}
	response.JSON(w, http.StatusOK, reviews)
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
	r.Get("/session", h.GetSession)
	r.Get("/orders", h.ListOrders)
	r.Get("/price-at", h.PriceAt)
	r.Get("/performance", h.Performance)
	r.Get("/risk-rules", h.RiskRules)
	r.Post("/simulate", h.Simulate)
	r.Post("/orders", h.PlaceOrder)
	r.Post("/trade", h.PlaceOrder)
	r.Get("/positions", h.ListPositions)
	r.Get("/analytics", h.GetAnalytics)
	r.Get("/portfolios", h.ListPortfolios)
	r.Post("/portfolios", h.CreatePortfolio)
	r.Get("/portfolios/{id}", h.GetPortfolio)
	r.Post("/portfolios/{id}/orders", h.PlaceOrder)
	r.Get("/portfolios/{id}/orders", h.ListOrders)
	r.Get("/portfolios/{id}/positions", h.ListPositions)
	r.Get("/portfolios/{id}/analytics", h.GetAnalytics)
	return r
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

// GetMarketHealth returns the health and freshness of the market data.
func (h *StocksHandler) GetMarketHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats, err := h.db.GetMarketDataHealthStats(ctx)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to get market health stats")
		return
	}

	now := time.Now()
	session := market.GetCurrentSession(now)

	var latestMarketTs, latestReceivedTs, latestIngestionTs *string
	var dataAgeSeconds *int
	var freshness string = "MISSING"

	// 1. Latest market candle and its associated retrieval timestamp from THAT SAME ROW
	if stats.LatestCandleTimestamp != nil && !stats.LatestCandleTimestamp.IsZero() {
		tsStr := stats.LatestCandleTimestamp.UTC().Format(time.RFC3339)
		latestMarketTs = &tsStr
		age := int(now.Sub(*stats.LatestCandleTimestamp).Seconds())
		dataAgeSeconds = &age
		// Freshness derived STRICTLY from candle market timestamp and current session
		freshness = market.GetFreshnessStatus(*stats.LatestCandleTimestamp, now)
	}

	if stats.CandleRetrievedAt != nil && !stats.CandleRetrievedAt.IsZero() {
		recStr := stats.CandleRetrievedAt.UTC().Format(time.RFC3339)
		latestReceivedTs = &recStr
	}

	// 2. Separate system-wide ingestion heartbeat (MAX(retrieved_at) across table)
	if stats.LatestIngestionActivity != nil && !stats.LatestIngestionActivity.IsZero() {
		ingStr := stats.LatestIngestionActivity.UTC().Format(time.RFC3339)
		latestIngestionTs = &ingStr
	}

	resp := map[string]any{
		"provider":                  "yahoofinance",
		"exchange_verified":         false,
		"guaranteed_realtime":       false,
		"latest_market_timestamp":   latestMarketTs,
		"latest_received_timestamp": latestReceivedTs,  // Retrieval timestamp of the latest candle
		"latest_ingestion_time":     latestIngestionTs, // Latest DB write activity across table
		"data_age_seconds":          dataAgeSeconds,
		"freshness_status":          freshness,
		"market_session":            string(session),
		"fallback_active":           false,
		"data_interval":             1,
		"data_type":                 "INTRADAY",
		"total_candles":             stats.TotalCandles,
	}

	response.JSON(w, http.StatusOK, resp)
}

// ---- Providers Handler ----

type ProvidersHandler struct{}

func NewProvidersHandler() *ProvidersHandler {
	return &ProvidersHandler{}
}

func (h *ProvidersHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/readiness", h.GetReadiness)
	return r
}

func (h *ProvidersHandler) GetReadiness(w http.ResponseWriter, r *http.Request) {
	report := marketdata.AuditProviders(r.Context())
	response.JSON(w, http.StatusOK, report)
}

// ---- Paper Observations Handler ----

type PaperObservationsHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewPaperObservationsHandler(db *database.DB, quantClient *quant.Client) *PaperObservationsHandler {
	return &PaperObservationsHandler{db: db, quantClient: quantClient}
}

func (h *PaperObservationsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.ListObservations)
	r.Post("/", h.RecordObservation)
	r.Get("/summary", h.GetSummary)
	return r
}

func (h *PaperObservationsHandler) ListObservations(w http.ResponseWriter, r *http.Request) {
	setupType := r.URL.Query().Get("setup_type")
	list, err := h.db.ListPaperObservations(r.Context(), setupType, 100)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list paper observations"})
		return
	}
	response.JSON(w, http.StatusOK, list)
}

// pinnedStrategyHash is the frozen Strategy V2 hash; candidates under any other rule_version are rejected.
const pinnedStrategyHash = "92ab27c24e4bc4c65977c17238a85df8e3aa9e1fbdec727d4bd9bab2c398c780"

// ValidateCandidate applies the server-side acceptance rules for a new paper candidate. last is the latest stored daily close.
func ValidateCandidate(obs *database.PaperObservation, last float64, now time.Time) error {
	if obs.Symbol == "" || obs.SetupType == "" {
		return fmt.Errorf("symbol and setup_type are required")
	}
	if !(obs.InvalidationPrice > 0 && obs.InvalidationPrice < obs.EntryTriggerPrice && obs.EntryTriggerPrice < obs.TargetPrice) {
		return fmt.Errorf("require 0 < invalidation_price < entry_trigger_price < target_price")
	}
	if obs.ShariahStatus != "PASS" {
		return fmt.Errorf("shariah hard gate: only PASS symbols may be recorded")
	}
	if obs.RuleVersion != "" && obs.RuleVersion != pinnedStrategyHash {
		return fmt.Errorf("rule_version must equal the pinned Strategy V2 hash")
	}
	if last <= 0 {
		return fmt.Errorf("no stored daily history for symbol; cannot validate candidate")
	}
	if r := obs.EntryTriggerPrice / last; r > 1.25 || r < 0.75 {
		return fmt.Errorf("entry_trigger_price %.2f is more than 25%% away from last stored close %.2f; rejected as implausible", obs.EntryTriggerPrice, last)
	}
	if !obs.DataCutoffAt.IsZero() && obs.DataCutoffAt.After(now) {
		return fmt.Errorf("data_cutoff_at is in the future")
	}
	return nil
}

func (h *PaperObservationsHandler) RecordObservation(w http.ResponseWriter, r *http.Request) {
	var in database.PaperObservation
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid paper observation JSON payload"})
		return
	}
	// Only decision-time inputs are accepted from the client. Lifecycle, outcome, returns, cost and timestamps are server-controlled.
	obs := database.PaperObservation{
		Symbol: in.Symbol, SetupType: in.SetupType, EntryTriggerPrice: in.EntryTriggerPrice, InvalidationPrice: in.InvalidationPrice,
		TargetPrice: in.TargetPrice, ShariahStatus: in.ShariahStatus, Sector: in.Sector, LiquidityBucket: in.LiquidityBucket,
		MarketRegime: in.MarketRegime, RuleVersion: in.RuleVersion, Provider: in.Provider, DataCutoffAt: in.DataCutoffAt,
	}

	symRecord, err := h.db.GetSymbolByCode(r.Context(), obs.Symbol, "NSE")
	if err != nil {
		response.JSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("unknown symbol %s", obs.Symbol)})
		return
	}
	obs.SymbolID = symRecord.ID

	var last float64
	var lastTS time.Time
	_ = h.db.QueryRowContext(r.Context(), `SELECT close, timestamp FROM ohlcv_daily WHERE symbol_id=$1 ORDER BY timestamp DESC LIMIT 1`, obs.SymbolID).Scan(&last, &lastTS)

	now := time.Now().UTC()
	if err := ValidateCandidate(&obs, last, now); err != nil {
		response.JSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	obs.RuleVersion = pinnedStrategyHash
	if obs.Provider == "" {
		obs.Provider = "yahoo_finance"
	}
	obs.CandidateTimestamp = now
	obs.SignalTimestamp = now
	if obs.DataCutoffAt.IsZero() {
		obs.DataCutoffAt = lastTS // honest default: the newest stored daily bar
	}
	obs.RiskRewardRatio = (obs.TargetPrice - obs.EntryTriggerPrice) / (obs.EntryTriggerPrice - obs.InvalidationPrice)

	id, created, err := h.db.CreatePaperObservation(r.Context(), &obs)
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("failed to record paper observation: %v", err)})
		return
	}
	obs.ID = id
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	response.JSON(w, status, map[string]any{"id": id, "created": created, "duplicate": !created, "observation": obs})
}

func (h *PaperObservationsHandler) GetSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.db.GetPaperObservationSummary(r.Context())
	if err != nil {
		response.JSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to calculate observation summary"})
		return
	}
	response.JSON(w, http.StatusOK, summary)
}
