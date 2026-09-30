package response

import (
	"encoding/json"
	"net/http"
)

// APIResponse is the standard API response wrapper.
type APIResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
}

// Meta contains pagination and metadata for list responses.
type Meta struct {
	Page       int   `json:"page,omitempty"`
	PerPage    int   `json:"per_page,omitempty"`
	Total      int64 `json:"total,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
}

// Disclaimer contains important disclaimers about predictions/analysis.
type Disclaimer struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Common disclaimers — predictions are NEVER represented as certainty.
var (
	PredictionDisclaimer = Disclaimer{
		Type:    "prediction",
		Message: "This is a statistical prediction based on historical data and models. It is NOT a guarantee of future performance. Past performance does not guarantee future results.",
	}
	BacktestDisclaimer = Disclaimer{
		Type:    "backtest",
		Message: "Backtested results are based on historical data and do not account for all real-world factors. Actual trading results may differ significantly.",
	}
	ShariahDisclaimer = Disclaimer{
		Type:    "shariah",
		Message: "Shariah screening is based on available financial data and configured rules. Please consult a qualified Shariah advisor for definitive rulings.",
	}
	GMPDisclaimer = Disclaimer{
		Type:    "gmp",
		Message: "Grey Market Premium (GMP) is unofficial market sentiment data. It is NOT a guaranteed indicator of listing price.",
	}
	AlphaBetaDisclaimer = Disclaimer{
		Type:    "statistical",
		Message: "Alpha and beta are statistical measures based on historical data. They are historical observations, not guaranteed predictors of future returns.",
	}
)

// JSON sends a JSON response.
func JSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: status >= 200 && status < 300,
		Data:    data,
	})
}

// WithMeta sends a JSON response with pagination metadata.
func WithMeta(w http.ResponseWriter, data any, meta *Meta) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
		Meta:    meta,
	})
}

// Error sends an error response.
func Error(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   message,
	})
}

// ValidationError sends a 422 response for input validation failures.
func ValidationError(w http.ResponseWriter, message string) {
	Error(w, http.StatusUnprocessableEntity, message)
}

// NotFound sends a 404 response.
func NotFound(w http.ResponseWriter, resource string) {
	Error(w, http.StatusNotFound, resource+" not found")
}

// Unauthorized sends a 401 response.
func Unauthorized(w http.ResponseWriter) {
	Error(w, http.StatusUnauthorized, "unauthorized")
}
