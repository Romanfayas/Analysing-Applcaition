package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/middleware"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/pkg/response"
)

// ---- Alerts ----

// AlertsHandler manages a user's price alerts.
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

// supportedAlertTypes are the conditions the worker can actually evaluate. Anything else is
// rejected up front instead of being stored and silently never firing.
var supportedAlertTypes = map[string]bool{"PRICE_ABOVE": true, "PRICE_BELOW": true}

func (h *AlertsHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	alerts, err := h.db.ListAlerts(r.Context(), claims.UserID)
	if err != nil {
		log.Error().Err(err).Msg("list alerts failed")
		response.Error(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}
	response.JSON(w, http.StatusOK, alerts)
}

func (h *AlertsHandler) CreateAlert(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	var req struct {
		Symbol    string  `json:"symbol"`
		AlertType string  `json:"alert_type"`
		Threshold float64 `json:"threshold"`
		Channel   string  `json:"channel"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.AlertType = strings.ToUpper(strings.TrimSpace(req.AlertType))
	req.Channel = strings.ToUpper(strings.TrimSpace(req.Channel))
	if req.Channel == "" {
		req.Channel = "IN_APP"
	}
	if !supportedAlertTypes[req.AlertType] {
		response.ValidationError(w, "alert_type must be PRICE_ABOVE or PRICE_BELOW")
		return
	}
	if req.Channel != "IN_APP" && req.Channel != "TELEGRAM" {
		response.ValidationError(w, "channel must be IN_APP or TELEGRAM")
		return
	}
	if !(req.Threshold > 0) || req.Threshold > 1e9 {
		response.ValidationError(w, "threshold must be a positive price")
		return
	}
	sym, err := h.db.GetSymbolByCode(r.Context(), strings.ToUpper(strings.TrimSpace(req.Symbol)), "NSE")
	if err != nil || !sym.IsActive {
		response.ValidationError(w, "symbol is not an active NSE instrument")
		return
	}
	a, err := h.db.CreateAlert(r.Context(), claims.UserID, sym.ID, req.AlertType, req.Threshold, req.Channel)
	if errors.Is(err, database.ErrAlertLimit) {
		response.Error(w, http.StatusConflict, "active alert limit of "+strconv.Itoa(database.MaxActiveAlertsPerUser)+" reached")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("create alert failed")
		response.Error(w, http.StatusInternalServerError, "failed to create alert")
		return
	}
	response.JSON(w, http.StatusCreated, a)
}

func (h *AlertsHandler) DeleteAlert(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		response.ValidationError(w, "alert id must be a positive integer")
		return
	}
	// Ownership is enforced in the DELETE itself; another user's id is indistinguishable from a missing one.
	deleted, err := h.db.DeleteAlert(r.Context(), claims.UserID, id)
	if err != nil {
		log.Error().Err(err).Msg("delete alert failed")
		response.Error(w, http.StatusInternalServerError, "failed to delete alert")
		return
	}
	if !deleted {
		response.NotFound(w, "alert")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"deleted": id})
}

func (h *AlertsHandler) GetHistory(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	events, err := h.db.ListAlertEvents(r.Context(), claims.UserID, 100)
	if err != nil {
		log.Error().Err(err).Msg("alert history failed")
		response.Error(w, http.StatusInternalServerError, "failed to load alert history")
		return
	}
	response.JSON(w, http.StatusOK, events)
}

// ---- Settings ----

// SettingsHandler manages per-user preferences. Secrets are never accepted or stored here;
// notification credentials are server-side configuration and only their presence is reported.
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

var (
	allowedBenchmarks = map[string]bool{"NIFTY 50": true, "NIFTY 500": true, "SENSEX": true}
	allowedTimezones  = map[string]bool{"Asia/Kolkata": true, "UTC": true}
)

func settingsResponse(s database.UserSettings) map[string]any {
	return map[string]any{
		"preferences": s,
		"notifications": map[string]any{
			"telegram_configured": os.Getenv("TELEGRAM_BOT_TOKEN") != "" && os.Getenv("TELEGRAM_CHAT_ID") != "",
			"note":                "Telegram credentials are server-side configuration (environment variables) and are never stored or shown here.",
		},
		"execution_safety": map[string]any{
			"paper_trading_only":     true,
			"live_order_execution":   false,
			"broker_trading_enabled": false,
		},
	}
}

func (h *SettingsHandler) GetSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	s, err := h.db.GetUserSettings(r.Context(), claims.UserID)
	if err != nil {
		log.Error().Err(err).Msg("get settings failed")
		response.Error(w, http.StatusInternalServerError, "failed to load settings")
		return
	}
	response.JSON(w, http.StatusOK, settingsResponse(s))
}

func (h *SettingsHandler) UpdateSettings(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return
	}
	var req database.UserSettings
	if !decodeJSON(w, r, &req) {
		return
	}
	if !allowedBenchmarks[req.DefaultBenchmark] {
		response.ValidationError(w, "default_benchmark must be one of NIFTY 50, NIFTY 500, SENSEX")
		return
	}
	if !(req.DefaultRiskPct > 0 && req.DefaultRiskPct <= 5) {
		response.ValidationError(w, "default_risk_pct must be greater than 0 and at most 5")
		return
	}
	if !allowedTimezones[req.DisplayTimezone] {
		response.ValidationError(w, "display_timezone must be Asia/Kolkata or UTC")
		return
	}
	if err := h.db.SaveUserSettings(r.Context(), claims.UserID, req); err != nil {
		log.Error().Err(err).Msg("save settings failed")
		response.Error(w, http.StatusInternalServerError, "failed to save settings")
		return
	}
	response.JSON(w, http.StatusOK, settingsResponse(req))
}
