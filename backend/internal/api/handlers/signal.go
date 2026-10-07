package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/notifications"
)

type SignalHandler struct {
	db          *database.DB
	quantClient *quant.Client
	notifier    *notifications.TelegramNotifier
}

func NewSignalHandler(db *database.DB, quantClient *quant.Client, notifier *notifications.TelegramNotifier) *SignalHandler {
	return &SignalHandler{
		db:          db,
		quantClient: quantClient,
		notifier:    notifier,
	}
}

// RegisterRoutes registers the signal API routes.
func (h *SignalHandler) RegisterRoutes(r chi.Router) {
	r.Get("/", h.GetActiveSignals)
	r.Get("/{symbol_id}", h.GetSignalBySymbolID)
	r.Post("/{symbol_id}/calculate", h.TriggerSignalCalculation)
}

// GetActiveSignals returns all active signals.
func (h *SignalHandler) GetActiveSignals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	
	signals, err := h.db.GetActiveSignals(ctx)
	if err != nil {
		http.Error(w, "Failed to retrieve active signals", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signals)
}

// GetSignalBySymbolID returns the latest signal for a specific symbol.
func (h *SignalHandler) GetSignalBySymbolID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbolIDStr := chi.URLParam(r, "symbol_id")
	symbolID, err := strconv.Atoi(symbolIDStr)
	if err != nil {
		http.Error(w, "Invalid symbol ID", http.StatusBadRequest)
		return
	}

	signal, err := h.db.GetLatestSignal(ctx, symbolID)
	if err != nil {
		http.Error(w, "Failed to retrieve signal", http.StatusInternalServerError)
		return
	}

	if signal == nil {
		http.Error(w, "Signal not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(signal)
}

// TriggerSignalCalculation triggers a re-calculation of the signal by calling the quant engine.
func (h *SignalHandler) TriggerSignalCalculation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	symbolIDStr := chi.URLParam(r, "symbol_id")
	symbolID, err := strconv.Atoi(symbolIDStr)
	if err != nil {
		http.Error(w, "Invalid symbol ID", http.StatusBadRequest)
		return
	}

	// This is a stub for the full orchestration.
	// Normally we would:
	// 1. Fetch OHLCV data for this symbol from DB
	// 2. Fetch Fundamental/Financial data for this symbol from DB
	// 3. Call QuantEngine to compute Technicals, Candlesticks, Fundamentals, Shariah
	// 4. Pass those scores to QuantEngine Signal/Calculate
	// 5. Store the resulting SignalRecord in the DB

	// Determine freshness
	var latestTs time.Time
	ts, err := h.db.GetLatestIntradayOHLCVTimestamp(ctx, symbolID)
	var freshness string = "MISSING"
	if err == nil && ts != nil {
		latestTs = *ts
		freshness = market.GetFreshnessStatus(latestTs, time.Now())
	}

	// Example mock request to test integration
	req := quant.SignalRequest{
		FundamentalScore:   ptr(75.0),
		TechnicalScore:     ptr(85.0),
		ShariahStatus:      "PASS",
		FundamentalFactors: []string{"Strong ROE"},
		MarketDataStatus:   freshness,
	}

	resp, err := h.quantClient.CalculateSignal(ctx, req)
	if err != nil {
		http.Error(w, "Quant engine calculation failed", http.StatusInternalServerError)
		return
	}

	// Mock DB storage
	record := database.SignalRecord{
		SymbolID:         symbolID,
		Signal:           resp.Signal,
		OverallScore:     resp.OverallScore,
		FundamentalScore: req.FundamentalScore,
		TechnicalScore:   req.TechnicalScore,
		ShariahStatus:    resp.ShariahStatus,
		Explanation:      resp.Explanation,
	}
	
	id, err := h.db.UpsertSignal(ctx, record)
	if err != nil {
		http.Error(w, "Failed to save signal", http.StatusInternalServerError)
		return
	}
	
	record.ID = id

	// Send notifications
	if h.notifier != nil {
		if resp.ShariahOverride {
			h.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertShariahChanged,
				Title:     "Shariah Status Alert: FAIL",
				Message:   "A stock previously tracked has failed Shariah screening and must be avoided.",
				Symbol:    strconv.Itoa(symbolID),
				Timestamp: time.Now(),
			})
		} else if resp.Signal == "STRONG_BUY" {
			h.notifier.SendAlert(notifications.Alert{
				Type:      notifications.AlertNewBuySignal,
				Title:     "New STRONG BUY Signal",
				Message:   fmt.Sprintf("Composite Score: %.1f\n%s", resp.OverallScore, resp.Explanation),
				Symbol:    strconv.Itoa(symbolID),
				Timestamp: time.Now(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(record)
}

func ptr(v float64) *float64 {
	return &v
}
