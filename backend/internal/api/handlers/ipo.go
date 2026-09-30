package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"halal-equity/internal/database"
	"halal-equity/internal/services/quant"
)

type IPOHandler struct {
	db          *database.DB
	quantClient *quant.Client
}

func NewIPOHandler(db *database.DB, quantClient *quant.Client) *IPOHandler {
	return &IPOHandler{
		db:          db,
		quantClient: quantClient,
	}
}

// RegisterRoutes registers the IPO API routes.
func (h *IPOHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.GetIPOs)
	r.Post("/analyze", h.AnalyzeIPO)
}

// GetIPOs returns all tracked IPOs.
func (h *IPOHandler) GetIPOs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	status := r.URL.Query().Get("status")
	ipos, err := h.db.GetIPOs(ctx, status)
	if err != nil {
		http.Error(w, "Failed to retrieve IPOs", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ipos)
}

// AnalyzeIPO triggers a prediction calculation via the Python Quant engine.
func (h *IPOHandler) AnalyzeIPO(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req quant.IPOAnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Determine Shariah Status (Stubbed here, ideally looked up from DB)
	// For demo, we default to PASS if not provided
	if req.ShariahStatus == "" {
		req.ShariahStatus = "PASS"
	}

	resp, err := h.quantClient.AnalyzeIPO(ctx, req)
	if err != nil {
		http.Error(w, "Quant engine IPO analysis failed", http.StatusInternalServerError)
		return
	}

	// Add timestamp to response
	fullResp := map[string]interface{}{
		"analyzed_at": time.Now().Format(time.RFC3339),
		"analysis":    resp,
		"inputs":      req,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(fullResp)
}
