package handlers

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/pkg/response"
)

const breakoutRunKind = "BREAKOUT_FOLLOW_THROUGH"

// GetBreakoutMonitor reports every tracked breakout, what happened to it, and the multi-year evidence next to it.
func (h *SignalsHandler) GetBreakoutMonitor(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.ListTrackedBreakouts(r.Context(), 100, "")
	if err != nil {
		log.Error().Err(err).Msg("breakout monitor failed")
		response.Error(w, http.StatusInternalServerError, "failed to load the breakout monitor")
		return
	}
	score, err := h.db.BreakoutScores(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("breakout scorecard failed")
		response.Error(w, http.StatusInternalServerError, "failed to load the breakout monitor")
		return
	}
	out := map[string]any{"signals": rows, "scorecard": score, "history": nil, "history_run_at": nil}
	if run, at, err := h.db.LatestResearchRun(r.Context(), breakoutRunKind); err == nil {
		out["history"] = map[string]any{"overall": run["overall"], "by_period": run["by_period"], "shariah_pass_only": run["shariah_pass_only"], "rules_of_the_test": run["rules_of_the_test"]}
		out["history_run_at"] = at
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Warn().Err(err).Msg("breakout history unavailable")
	}
	response.JSON(w, http.StatusOK, out)
}

// GetResearch returns the newest exchange-series research run: real results, clearly separate from the certified gate.
func (h *BacktestingHandler) GetResearch(w http.ResponseWriter, r *http.Request) {
	run, at, err := h.db.LatestResearchRun(r.Context(), breakoutRunKind)
	if errors.Is(err, sql.ErrNoRows) {
		response.JSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("research run failed to load")
		response.Error(w, http.StatusInternalServerError, "failed to load research results")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"available": true, "run_at": at.Format(time.RFC3339), "result": run})
}
