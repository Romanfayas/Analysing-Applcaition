package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"halal-equity/internal/database"
	"halal-equity/internal/services/llm"
	"halal-equity/internal/services/quant"
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
	
	// 1. Mock fetch fundamental data
	fundReq := quant.FundamentalInput{
		PriceToEarnings: 22.5,
		DebtToEquity:    0.1,
		ReturnOnEquity:  18.5,
		FreeCashFlow:    1500.5,
		RevenueGrowth:   12.0,
		ProfitMargin:    15.0,
	}
	
	fundData, err := h.quantClient.AnalyzeFundamentals(ctx, fundReq)
	if err != nil {
		http.Error(w, "Failed to analyze fundamentals", http.StatusInternalServerError)
		return
	}

	// 2. Mock fetch Shariah data
	shariahReq := quant.ShariahRequest{
		Symbol:                 symbol,
		DebtToTotalAssets:      0.20, // 20% (< 33%)
		IlliquidToTotalAssets:  0.40, // 40% (> 25%)
		NonCompliantRevenuePct: 0.03, // 3% (< 5%)
		IsFinancialSector:      false,
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
