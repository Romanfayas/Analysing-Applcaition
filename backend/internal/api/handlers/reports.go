package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/llm"
	"github.com/halal-equity/backend/internal/services/quant"
)

type ReportsHandler struct {
	db          *database.DB
	quantClient *quant.Client
	llmService  *llm.GeminiService
}

func NewReportsHandler(db *database.DB, quantClient *quant.Client, llmService *llm.GeminiService) *ReportsHandler {
	return &ReportsHandler{
		db:          db,
		quantClient: quantClient,
		llmService:  llmService,
	}
}

// RegisterRoutes registers the LLM reporting routes.
func (h *ReportsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/generate", h.GenerateReport)
}

// GenerateReport orchestrates data collection and triggers the LLM.
func (h *ReportsHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbol := r.URL.Query().Get("symbol")

	if symbol == "" {
		http.Error(w, "symbol parameter is required", http.StatusBadRequest)
		return
	}

	// In a real implementation, we'd fetch the latest OHLCV and financials for this symbol.
	// For MVP integration, we simulate the data to pass to the Quant Engine.
	
	// Helper
	ptr := func(f float64) *float64 { return &f }

	// 1. Mock fetch fundamental data
	fundReq := quant.FundamentalRequest{
		NetProfit:    ptr(500000),
		Revenue:      ptr(1000000),
		TotalAssets:  ptr(2000000),
		TotalEquity:  ptr(1000000),
		TotalDebt:    ptr(100000),
		MarketCap:    ptr(10000000),
	}
	
	fundData, err := h.quantClient.CalculateFundamentals(ctx, fundReq)
	if err != nil {
		http.Error(w, "Failed to analyze fundamentals", http.StatusInternalServerError)
		return
	}

	// 2. Mock fetch Shariah data
	shariahReq := quant.ShariahRequest{
		TotalDebt:               ptr(200000),
		CashAndEquivalents:      ptr(500000),
		InterestBearingDeposits: ptr(100000),
		TotalReceivables:        ptr(300000),
		MarketCap:               ptr(10000000),
		TotalAssets:             ptr(2000000),
		TotalRevenue:            ptr(1000000),
		InterestIncome:          ptr(10000),
		NonCompliantRevenue:     ptr(30000),
		IsFinancialInstitution:  false,
		IsProhibitedIndustry:    false,
	}
	
	shariahData, err := h.quantClient.ScreenShariah(ctx, shariahReq)
	if err != nil {
		http.Error(w, "Failed to screen Shariah compliance", http.StatusInternalServerError)
		return
	}
	
	// 3. Mock technical data (using existing endpoints if needed, or simply passing a struct)
	techData := map[string]interface{}{
		"symbol": symbol,
		"rsi_14": 65.5,
		"macd_signal": "BULLISH",
		"trend": "UPTREND",
		"volatility": "MEDIUM",
	}

	// 4. Send everything to Gemini
	reportText, err := h.llmService.GenerateReport(ctx, symbol, techData, fundData, shariahData)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"symbol": symbol,
		"report": reportText,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
