package handlers

import (
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/halal-equity/backend/internal/database"
)

type PaperSyncHandler struct {
	db *database.DB
}

func NewPaperSyncHandler(db *database.DB) *PaperSyncHandler {
	return &PaperSyncHandler{db: db}
}

func (h *PaperSyncHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/order", h.SyncOrder)
	r.Post("/fill", h.SyncFill)
	r.Post("/event", h.SyncEvent)
	return r
}

type SyncOrderRequest struct {
	PortfolioID   int     `json:"portfolio_id"`
	SymbolID      int     `json:"symbol_id"`
	Side          string  `json:"side"`
	Quantity      int     `json:"quantity"`
	OrderPrice    float64 `json:"order_price"`
	SignalID      int64   `json:"signal_id,omitempty"`
}

func (h *PaperSyncHandler) SyncOrder(w http.ResponseWriter, r *http.Request) {
	var req SyncOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	query := `
		INSERT INTO paper_orders (portfolio_id, symbol_id, side, quantity, order_price, status, signal_id)
		VALUES ($1, $2, $3, $4, $5, 'PENDING', $6) RETURNING id
	`
	var orderID int
	err := h.db.QueryRowContext(r.Context(), query, req.PortfolioID, req.SymbolID, req.Side, req.Quantity, req.OrderPrice, req.SignalID).Scan(&orderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"id": orderID})
}

type SyncFillRequest struct {
	OrderID           int     `json:"order_id"`
	FillPrice         float64 `json:"fill_price"`
	SimulatedSlippage float64 `json:"simulated_slippage"`
}

func (h *PaperSyncHandler) SyncFill(w http.ResponseWriter, r *http.Request) {
	var req SyncFillRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	
	query := `
		UPDATE paper_orders
		SET status = 'FILLED', fill_price = $1, simulated_slippage = $2, filled_at = NOW()
		WHERE id = $3
	`
	_, err := h.db.ExecContext(r.Context(), query, req.FillPrice, req.SimulatedSlippage, req.OrderID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	
	w.WriteHeader(http.StatusOK)
}

func (h *PaperSyncHandler) SyncEvent(w http.ResponseWriter, r *http.Request) {
	// Simulated minimal event intake
	w.WriteHeader(http.StatusCreated)
}
