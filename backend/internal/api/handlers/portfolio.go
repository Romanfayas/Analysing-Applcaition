package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"halal-equity/internal/database"
	"halal-equity/internal/services/quant"
)

type PortfolioHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewPortfolioHandler(db *database.DB, quantClient *quant.Client) *PortfolioHandler {
	return &PortfolioHandler{
		db:          db,
		quantClient: quantClient,
	}
}

// RegisterRoutes registers the Portfolio and Paper Trading API routes.
func (h *PortfolioHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.GetPortfolioSummary)
	r.Post("/trade", h.ExecuteTrade)
	r.Post("/risk", h.CalculateRisk)
}

// GetPortfolioSummary retrieves the MVP user's portfolio and active positions.
func (h *PortfolioHandler) GetPortfolioSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := 1 // Hardcoded for MVP single-user

	portfolio, err := h.db.GetOrCreatePortfolio(ctx, userID)
	if err != nil {
		http.Error(w, "Failed to load portfolio", http.StatusInternalServerError)
		return
	}

	positions, err := h.db.GetPortfolioPositions(ctx, portfolio.ID)
	if err != nil {
		http.Error(w, "Failed to load positions", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"portfolio": portfolio,
		"positions": positions,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// TradeRequest represents a paper trading payload.
type TradeRequest struct {
	SymbolID  int     `json:"symbol_id"`
	TradeType string  `json:"trade_type"` // BUY or SELL
	Shares    int     `json:"shares"`
	Price     float64 `json:"price"`
}

// ExecuteTrade executes a paper trade.
func (h *PortfolioHandler) ExecuteTrade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := 1

	var req TradeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	portfolio, err := h.db.GetOrCreatePortfolio(ctx, userID)
	if err != nil {
		http.Error(w, "Failed to load portfolio", http.StatusInternalServerError)
		return
	}

	trade, err := h.db.ExecutePaperTrade(ctx, portfolio.ID, req.SymbolID, req.TradeType, req.Shares, req.Price)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trade)
}

// CalculateRisk forwards position sizing requests to the Quant Engine.
func (h *PortfolioHandler) CalculateRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req quant.PositionSizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	resp, err := h.quantClient.CalculatePositionSize(ctx, req)
	if err != nil {
		http.Error(w, "Failed to calculate risk", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
