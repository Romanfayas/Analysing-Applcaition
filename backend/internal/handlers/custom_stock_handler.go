package handlers

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/workers/ingestion"
	"github.com/halal-equity/backend/pkg/response"
)

var tickerPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9&-]{0,19}$`)

// onboardTimeout bounds one onboarding run (daily + 1m + 5m history and a Shariah screening).
const onboardTimeout = 6 * time.Minute

type addStockRequest struct {
	Symbol string `json:"symbol"`
}

// normalizeTicker accepts "pcjeweller", "PCJEWELLER.NS" or "NSE:PCJEWELLER".
func normalizeTicker(raw string) string {
	t := strings.ToUpper(strings.TrimSpace(raw))
	t = strings.TrimPrefix(t, "NSE:")
	return strings.TrimSuffix(t, ".NS")
}

// AddStock registers an NSE stock the owner wants to follow and starts loading its data in the background.
// The stock then appears in the screener and the paper-trading terminal like any other, once its price history
// has loaded. Nothing is assumed: a ticker Yahoo does not know is rejected, and each data step reports its own result.
func (h *StocksHandler) AddStock(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(w, r)
	if !ok {
		return
	}
	var req addStockRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	ticker := normalizeTicker(req.Symbol)
	if !tickerPattern.MatchString(ticker) {
		response.ValidationError(w, "enter an NSE ticker such as PCJEWELLER (letters, digits, & or -)")
		return
	}

	p := ingestion.NewPipeline(h.db, nil)
	if st, userAdded, err := h.db.GetOnboardStatus(r.Context(), ticker); err == nil {
		if st != nil && st.State == database.OnboardRunning && time.Since(st.UpdatedAt) < onboardTimeout {
			response.JSON(w, http.StatusAccepted, map[string]any{"symbol": ticker, "status": st})
			return
		}
		if !userAdded {
			response.JSON(w, http.StatusOK, map[string]any{"symbol": ticker, "already_tracked": true,
				"message": ticker + " is already part of the platform; find it in the Screener."})
			return
		}
	}

	name, err := p.ValidateNewSymbol(ticker)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	id, created, err := p.RegisterNewSymbol(r.Context(), ticker, name, uid)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	if !created {
		response.JSON(w, http.StatusOK, map[string]any{"symbol": ticker, "already_tracked": true,
			"message": ticker + " is already part of the platform; find it in the Screener."})
		return
	}

	screen := func(ctx context.Context, t string) (string, error) {
		if h.quantClient == nil {
			return "", context.Canceled
		}
		return h.quantClient.ScreenSymbol(ctx, t)
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), onboardTimeout)
		defer cancel()
		st := p.OnboardSymbol(ctx, id, ticker, name, screen)
		log.Info().Str("symbol", ticker).Str("state", st.State).Msg("stock onboarding finished")
	}()

	response.JSON(w, http.StatusAccepted, map[string]any{"symbol": ticker, "name": name,
		"status": database.OnboardStatus{State: database.OnboardRunning}})
}

// AddStockStatus reports how far loading an added stock has got.
func (h *StocksHandler) AddStockStatus(w http.ResponseWriter, r *http.Request) {
	ticker := normalizeTicker(symbolParam(r))
	st, userAdded, err := h.db.GetOnboardStatus(r.Context(), ticker)
	if err != nil {
		response.NotFound(w, "symbol")
		return
	}
	if st != nil && st.State == database.OnboardRunning && time.Since(st.UpdatedAt) > onboardTimeout {
		st.State = database.OnboardFailed // the run died with its process
		st.Steps = append(st.Steps, database.OnboardStep{Name: "timeout", Detail: "loading did not finish; add the stock again to retry"})
	}
	response.JSON(w, http.StatusOK, map[string]any{"symbol": ticker, "user_added": userAdded, "status": st})
}
