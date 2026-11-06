package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/middleware"
	"github.com/halal-equity/backend/pkg/response"
)

// researchGate is the single, DB-derived answer to "may historical research run?".
// Nothing in it is a constant: every count is measured at request time.
type researchGate struct {
	Status                  string   `json:"status"`
	Reason                  string   `json:"reason"`
	QuarantinedBars         int64    `json:"quarantined_bars_count"`
	EquitiesWithQuarantine  int      `json:"equities_with_quarantined_bars"`
	TotalInstruments        int      `json:"total_instruments"`
	InstrumentsWithBars     int      `json:"instruments_with_bars"`
	UnauthenticatedEvents   int      `json:"unauthenticated_demoted_events"`
	PrimaryVerifiedEvents   int      `json:"primary_verified_events"`
	ShariahPass             int      `json:"shariah_active_pass"`
	ShariahFail             int      `json:"shariah_active_fail"`
	ShariahDataInsufficient int      `json:"shariah_active_data_insufficient"`
	StrategyHash            *string  `json:"canonical_strategy_hash"`
	PinnedStrategyHash      *string  `json:"pinned_strategy_hash"`
	StrategyHashValid       *bool    `json:"is_strategy_hash_valid"`
	StrategyProblems        []string `json:"strategy_problems"`
	ConfigID                string   `json:"config_id"`
	Timestamp               string   `json:"timestamp"`
}

func (h *BacktestingHandler) computeGate(ctx context.Context) (*researchGate, error) {
	g := &researchGate{ConfigID: "STRATEGY_V2_CORE_PARAMETERS_V1", StrategyProblems: []string{}}

	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE d.adjusted_close IS NULL),
		       count(DISTINCT d.symbol_id) FILTER (WHERE d.adjusted_close IS NULL),
		       count(DISTINCT d.symbol_id)
		FROM ohlcv_daily d JOIN symbols s ON s.id = d.symbol_id
		WHERE COALESCE(s.metadata->>'user_added', 'false') <> 'true'`).Scan(&g.QuarantinedBars, &g.EquitiesWithQuarantine, &g.InstrumentsWithBars); err != nil {
		return nil, fmt.Errorf("bar gate query failed: %w", err)
	}
	if err := h.db.QueryRowContext(ctx, `SELECT count(*) FROM symbols s WHERE COALESCE(s.metadata->>'user_added', 'false') <> 'true'`).Scan(&g.TotalInstruments); err != nil {
		return nil, fmt.Errorf("symbol count failed: %w", err)
	}
	if err := h.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE verification_status = 'ARCHIVE_UNAUTHENTICATED'),
		       count(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY')
		FROM corporate_action_events`).Scan(&g.UnauthenticatedEvents, &g.PrimaryVerifiedEvents); err != nil {
		return nil, fmt.Errorf("corporate action gate query failed: %w", err)
	}
	if err := h.db.QueryRowContext(ctx, `
		WITH latest AS (
			SELECT DISTINCT ON (symbol_id) symbol_id, status
			FROM shariah_screenings ORDER BY symbol_id, screening_date DESC, id DESC
		)
		SELECT count(*) FILTER (WHERE l.status = 'PASS'),
		       count(*) FILTER (WHERE l.status = 'FAIL'),
		       count(*) FILTER (WHERE l.status = 'DATA_INSUFFICIENT')
		FROM symbols s JOIN latest l ON l.symbol_id = s.id WHERE s.is_active AND COALESCE(s.metadata->>'user_added', 'false') <> 'true'`).
		Scan(&g.ShariahPass, &g.ShariahFail, &g.ShariahDataInsufficient); err != nil {
		return nil, fmt.Errorf("shariah gate query failed: %w", err)
	}

	// Strategy attestation comes from the engine that will actually run the strategy.
	// If it cannot be reached the hash is reported unknown and the gate stays closed.
	if h.quantClient != nil {
		gctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if gov, err := h.quantClient.GetStrategyGovernance(gctx); err == nil {
			g.StrategyHash, g.PinnedStrategyHash = &gov.RuntimeHash, &gov.PinnedHash
			g.StrategyHashValid = &gov.MatchesPinned
			if gov.Problems != nil {
				g.StrategyProblems = gov.Problems
			}
		} else {
			log.Warn().Err(err).Msg("strategy governance unavailable")
		}
	}

	var blockers []string
	if g.QuarantinedBars > 0 {
		blockers = append(blockers, fmt.Sprintf("%d historical daily bars across %d instruments remain quarantined (NULL adjusted close)",
			g.QuarantinedBars, g.EquitiesWithQuarantine))
	}
	if g.UnauthenticatedEvents > 0 {
		blockers = append(blockers, fmt.Sprintf("%d corporate-action events lack primary-source authentication", g.UnauthenticatedEvents))
	}
	if g.StrategyHashValid == nil {
		blockers = append(blockers, "frozen-strategy attestation could not be obtained from the quant engine")
	} else if !*g.StrategyHashValid {
		blockers = append(blockers, "running strategy does not match the pinned hash")
	}
	g.Timestamp = time.Now().UTC().Format(time.RFC3339)
	if len(blockers) == 0 {
		g.Status = "RESEARCH_ELIGIBLE"
		g.Reason = "All research gates satisfied."
	} else {
		g.Status = "RESEARCH_BLOCKED"
		g.Reason = "Historical research is blocked: " + strings.Join(blockers, "; ") + "."
	}
	return g, nil
}

// GetGate reports the research-eligibility gate.
func (h *BacktestingHandler) GetGate(w http.ResponseWriter, r *http.Request) {
	g, err := h.computeGate(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("research gate failed")
		response.Error(w, http.StatusInternalServerError, "failed to evaluate research gate")
		return
	}
	response.JSON(w, http.StatusOK, g)
}

// RunBacktest refuses to run while the research gate is closed.
func (h *BacktestingHandler) RunBacktest(w http.ResponseWriter, r *http.Request) {
	g, err := h.computeGate(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("research gate failed")
		response.Error(w, http.StatusInternalServerError, "failed to evaluate research gate")
		return
	}
	if g.Status != "RESEARCH_ELIGIBLE" {
		response.JSON(w, http.StatusOK, map[string]any{
			"status":           "RESEARCH_BLOCKED",
			"reason":           g.Reason,
			"quarantined_bars": g.QuarantinedBars,
			"gate":             g,
			"execution_safety": map[string]any{
				"paper_trading_enabled": true, "live_order_execution": false, "broker_trading_enabled": false,
			},
		})
		return
	}
	response.Error(w, http.StatusNotImplemented,
		"historical backtest execution is not enabled; use /backtesting/run_walk_forward once the gate is open")
}

// RunWalkForward is gated by exactly the same measured conditions as RunBacktest.
func (h *BacktestingHandler) RunWalkForward(w http.ResponseWriter, r *http.Request) {
	g, err := h.computeGate(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("research gate failed")
		response.Error(w, http.StatusInternalServerError, "failed to evaluate research gate")
		return
	}
	if g.Status != "RESEARCH_ELIGIBLE" {
		response.JSON(w, http.StatusOK, map[string]any{
			"status":           "RESEARCH_BLOCKED",
			"reason":           "Walk-forward simulation blocked. " + g.Reason,
			"quarantined_bars": g.QuarantinedBars,
			"gate":             g,
		})
		return
	}
	response.Error(w, http.StatusNotImplemented, "walk-forward execution is not enabled until the research gate has been reviewed")
}

// ListRuns returns persisted backtest run summaries.
func (h *BacktestingHandler) ListRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.db.ListBacktestRuns(r.Context(), 100)
	if err != nil {
		log.Error().Err(err).Msg("list runs failed")
		response.Error(w, http.StatusInternalServerError, "failed to list backtest runs")
		return
	}
	response.JSON(w, http.StatusOK, runs)
}

// GetRunResults returns one persisted run.
func (h *BacktestingHandler) GetRunResults(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id < 1 {
		response.ValidationError(w, "run id must be a positive integer")
		return
	}
	run, err := h.db.GetBacktestRun(r.Context(), id)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "backtest run")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("get run failed")
		response.Error(w, http.StatusInternalServerError, "failed to load backtest run")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"run": run, "disclaimer": response.BacktestDisclaimer.Message})
}

// ListStrategies returns stored strategy definitions.
func (h *BacktestingHandler) ListStrategies(w http.ResponseWriter, r *http.Request) {
	list, err := h.db.ListBacktestStrategies(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("list strategies failed")
		response.Error(w, http.StatusInternalServerError, "failed to list strategies")
		return
	}
	response.JSON(w, http.StatusOK, list)
}

// CreateStrategy stores a new strategy definition as the next version of its name.
func (h *BacktestingHandler) CreateStrategy(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Config      json.RawMessage `json:"config"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 255 {
		response.ValidationError(w, "name is required (max 255 characters)")
		return
	}
	var cfg map[string]any
	if len(req.Config) == 0 || json.Unmarshal(req.Config, &cfg) != nil || cfg == nil {
		response.ValidationError(w, "config must be a JSON object")
		return
	}
	var createdBy *string
	if claims := middleware.GetUserFromContext(r.Context()); claims != nil {
		id := claims.UserID.String()
		createdBy = &id
	}
	s, err := h.db.CreateBacktestStrategy(r.Context(), req.Name, req.Description, req.Config, createdBy)
	if err != nil {
		log.Error().Err(err).Msg("create strategy failed")
		response.Error(w, http.StatusInternalServerError, "failed to create strategy")
		return
	}
	response.JSON(w, http.StatusCreated, s)
}

// GetWeights returns the active signal weight configuration.
func (h *SignalsHandler) GetWeights(w http.ResponseWriter, r *http.Request) {
	var raw []byte
	err := h.db.QueryRowContext(r.Context(),
		`SELECT weights FROM signal_weight_configs WHERE is_active ORDER BY updated_at DESC, id DESC LIMIT 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "active signal weight configuration")
		return
	}
	if err != nil {
		log.Error().Err(err).Msg("weights query failed")
		response.Error(w, http.StatusInternalServerError, "failed to load weights")
		return
	}
	var weights map[string]float64
	if err := json.Unmarshal(raw, &weights); err != nil {
		response.Error(w, http.StatusInternalServerError, "stored weights are malformed")
		return
	}
	response.JSON(w, http.StatusOK, weights)
}

// UpdateWeights is refused: signal weights are part of the frozen Strategy V2 definition.
// Changing them would silently invalidate the pinned strategy hash and every paper result
// attributed to it; a new weight set requires a new pre-registered strategy version.
func (h *SignalsHandler) UpdateWeights(w http.ResponseWriter, r *http.Request) {
	response.Error(w, http.StatusConflict,
		"signal weights are frozen under Strategy V2; propose a new strategy version instead of editing them in place")
}
