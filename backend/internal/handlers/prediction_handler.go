package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/pkg/response"
)

// PredictionHandler serves the intraday prediction engine's evidence and forecasts (read-only).
type PredictionHandler struct{ db *database.DB }

func NewPredictionHandler(db *database.DB) *PredictionHandler { return &PredictionHandler{db: db} }

func (h *PredictionHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/models", h.Models)
	r.Get("/latest", h.Latest)
	r.Get("/tracking", h.Tracking)
	r.Get("/symbol/{symbol}", h.BySymbol)
	return r
}

const predictionDisclaimer = "Statistical forecasts, not certainties. Only act on a BUY_CANDIDATE when its horizon is marked ACTIONABLE by out-of-sample testing; you place and manage every order yourself."

// Models returns each horizon's active model with its walk-forward evidence.
func (h *PredictionHandler) Models(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT id, model_version, horizon_minutes, trained_at, data_from, data_to, COALESCE(n_symbols, 0),
		       verdict, metrics, cost_model
		FROM prediction_model_runs
		WHERE is_active OR id IN (SELECT max(id) FROM prediction_model_runs GROUP BY horizon_minutes)
		ORDER BY horizon_minutes, is_active DESC, id DESC`)
	if err != nil {
		log.Error().Err(err).Msg("prediction models query failed")
		response.Error(w, http.StatusInternalServerError, "failed to load prediction models")
		return
	}
	defer rows.Close()
	seen := map[int]bool{}
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var version, verdict string
		var horizon, nSym int
		var trained time.Time
		var from, to sql.NullTime
		var metrics, cost []byte
		if err := rows.Scan(&id, &version, &horizon, &trained, &from, &to, &nSym, &verdict, &metrics, &cost); err != nil {
			response.Error(w, http.StatusInternalServerError, "failed to read prediction models")
			return
		}
		if seen[horizon] {
			continue
		}
		seen[horizon] = true
		item := map[string]any{
			"run_id": id, "model_version": version, "horizon_minutes": horizon, "trained_at": trained,
			"n_symbols": nSym, "verdict": verdict, "metrics": json.RawMessage(metrics), "cost_model": json.RawMessage(cost),
			"data_from": nil, "data_to": nil,
		}
		if from.Valid {
			item["data_from"] = from.Time.Format("2006-01-02")
		}
		if to.Valid {
			item["data_to"] = to.Time.Format("2006-01-02")
		}
		out = append(out, item)
	}
	response.JSON(w, http.StatusOK, map[string]any{"models": out, "disclaimer": predictionDisclaimer})
}

const predictionColumns = `
	p.id, s.symbol, p.horizon_minutes, p.as_of, p.target_time, p.issued_at,
	p.reference_price::float8, p.p_down::float8, p.p_neutral::float8, p.p_up::float8, p.expected_return::float8,
	p.return_q10::float8, p.return_q50::float8, p.return_q90::float8, p.action, p.action_reasons,
	p.entry_price::float8, p.target_price::float8, p.stop_price::float8, p.shariah_status, r.verdict,
	p.realized_price::float8, p.realized_return::float8, p.realized_class, p.resolved_at`

func scanPredictions(rows *sql.Rows) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		var id int64
		var sym, action, shariah, verdict string
		var horizon int
		var asOf, target, issued time.Time
		var ref, pd, pn, pu, exp, q10, q50, q90 float64
		var reasons []byte
		var entry, tgt, stop, realPx, realRet sql.NullFloat64
		var realCls sql.NullString
		var resolved sql.NullTime
		if err := rows.Scan(&id, &sym, &horizon, &asOf, &target, &issued, &ref, &pd, &pn, &pu, &exp, &q10, &q50, &q90,
			&action, &reasons, &entry, &tgt, &stop, &shariah, &verdict, &realPx, &realRet, &realCls, &resolved); err != nil {
			return nil, err
		}
		nf := func(v sql.NullFloat64) any {
			if v.Valid {
				return v.Float64
			}
			return nil
		}
		item := map[string]any{
			"id": id, "symbol": sym, "horizon_minutes": horizon, "as_of": asOf, "target_time": target, "issued_at": issued,
			"data_latency_seconds": int(issued.Sub(asOf).Seconds()),
			"reference_price":      ref, "p_down": pd, "p_neutral": pn, "p_up": pu, "expected_return": exp,
			"return_q10": q10, "return_q50": q50, "return_q90": q90,
			"action": action, "action_reasons": json.RawMessage(reasons),
			"entry_price": nf(entry), "target_price": nf(tgt), "stop_price": nf(stop),
			"shariah_status": shariah, "model_verdict": verdict,
			"realized_price": nf(realPx), "realized_return": nf(realRet), "realized_class": nil, "resolved_at": nil,
		}
		if realCls.Valid {
			item["realized_class"] = realCls.String
		}
		if resolved.Valid {
			item["resolved_at"] = resolved.Time
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// Latest returns the most recent forecast for every symbol, per horizon.
func (h *PredictionHandler) Latest(w http.ResponseWriter, r *http.Request) {
	horizon := 0
	if raw := r.URL.Query().Get("horizon"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			response.ValidationError(w, "horizon must be a positive number of minutes")
			return
		}
		horizon = v
	}
	// Latest forecast per symbol and horizon. (Filtering on the single newest bar time would silently drop
	// any symbol whose last bar lags by one interval; each row carries its own as_of instead.)
	rows, err := h.db.QueryContext(r.Context(), `
		WITH latest AS (
			SELECT DISTINCT ON (horizon_minutes, symbol_id) id FROM predictions
			WHERE ($1 = 0 OR horizon_minutes = $1)
			ORDER BY horizon_minutes, symbol_id, as_of DESC, id DESC
		)
		SELECT `+predictionColumns+`
		FROM predictions p
		JOIN latest l ON l.id = p.id
		JOIN symbols s ON s.id = p.symbol_id
		JOIN prediction_model_runs r ON r.id = p.model_run_id
		ORDER BY p.horizon_minutes, (p.action = 'BUY_CANDIDATE') DESC, p.expected_return DESC`, horizon)
	if err != nil {
		log.Error().Err(err).Msg("latest predictions failed")
		response.Error(w, http.StatusInternalServerError, "failed to load predictions")
		return
	}
	defer rows.Close()
	list, err := scanPredictions(rows)
	if err != nil {
		log.Error().Err(err).Msg("scan predictions failed")
		response.Error(w, http.StatusInternalServerError, "failed to load predictions")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"predictions": list, "disclaimer": predictionDisclaimer})
}

// BySymbol returns a symbol's recent forecasts and how they turned out.
func (h *PredictionHandler) BySymbol(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(strings.TrimSpace(chi.URLParam(r, "symbol")))
	if _, err := h.db.GetSymbolByCode(r.Context(), code, "NSE"); errors.Is(err, sql.ErrNoRows) {
		response.NotFound(w, "symbol")
		return
	}
	rows, err := h.db.QueryContext(r.Context(), `
		SELECT `+predictionColumns+`
		FROM predictions p JOIN symbols s ON s.id = p.symbol_id JOIN prediction_model_runs r ON r.id = p.model_run_id
		WHERE s.symbol = $1 AND s.exchange = 'NSE' ORDER BY p.as_of DESC, p.horizon_minutes LIMIT 200`, code)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load predictions")
		return
	}
	defer rows.Close()
	list, err := scanPredictions(rows)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to load predictions")
		return
	}
	response.JSON(w, http.StatusOK, list)
}

// Tracking reports how live (forward) forecasts actually turned out, net of the same round-trip cost
// the model was trained against. This is the evidence that cannot be overfitted.
func (h *PredictionHandler) Tracking(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.QueryContext(r.Context(), `
		WITH x AS (
			SELECT p.horizon_minutes, p.action, p.p_up::float8 AS p_up, p.realized_class, p.realized_return::float8 AS ret,
			       (rr.cost_model->>'round_trip_cost_fraction')::float8 AS cost,
			       p.action_reasons @> '[{"code":"MODEL_VIEW","detail":"BUY"}]' AS view_buy,
			       p.resolved_at IS NOT NULL AS resolved
			FROM predictions p JOIN prediction_model_runs rr ON rr.id = p.model_run_id
		)
		SELECT horizon_minutes,
		       count(*)                                                        AS issued,
		       count(*) FILTER (WHERE resolved AND ret IS NOT NULL)            AS resolved,
		       count(*) FILTER (WHERE NOT resolved)                            AS pending,
		       count(*) FILTER (WHERE resolved AND ret IS NULL)                AS unresolvable,
		       count(*) FILTER (WHERE action = 'BUY_CANDIDATE' AND ret IS NOT NULL) AS buy_n,
		       avg(ret - cost) FILTER (WHERE action = 'BUY_CANDIDATE' AND ret IS NOT NULL) AS buy_net,
		       avg(((ret - cost) > 0)::int) FILTER (WHERE action = 'BUY_CANDIDATE' AND ret IS NOT NULL) AS buy_hit,
		       count(*) FILTER (WHERE view_buy AND ret IS NOT NULL)            AS view_buy_n,
		       avg(ret - cost) FILTER (WHERE view_buy AND ret IS NOT NULL)     AS view_buy_net,
		       avg(ret - cost) FILTER (WHERE ret IS NOT NULL)                  AS always_long_net,
		       avg(power(p_up - (realized_class = 'UP')::int, 2)) FILTER (WHERE ret IS NOT NULL) AS brier_up,
		       avg(p_up) FILTER (WHERE ret IS NOT NULL)                        AS mean_p_up,
		       avg((realized_class = 'UP')::int) FILTER (WHERE ret IS NOT NULL) AS up_rate
		FROM x GROUP BY horizon_minutes ORDER BY horizon_minutes`)
	if err != nil {
		log.Error().Err(err).Msg("prediction tracking failed")
		response.Error(w, http.StatusInternalServerError, "failed to load tracking")
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var hz, issued, resolved, pending, unres, buyN, viewN int
		var buyNet, buyHit, viewNet, alwaysNet, brier, meanPUp, upRate sql.NullFloat64
		if err := rows.Scan(&hz, &issued, &resolved, &pending, &unres, &buyN, &buyNet, &buyHit, &viewN, &viewNet,
			&alwaysNet, &brier, &meanPUp, &upRate); err != nil {
			response.Error(w, http.StatusInternalServerError, "failed to read tracking")
			return
		}
		nf := func(v sql.NullFloat64) any {
			if v.Valid {
				return v.Float64
			}
			return nil
		}
		out = append(out, map[string]any{
			"horizon_minutes": hz, "issued": issued, "resolved": resolved, "pending": pending, "unresolvable": unres,
			"recommended_buys":                     map[string]any{"n": buyN, "mean_net_return": nf(buyNet), "hit_rate_net": nf(buyHit)},
			"model_view_buys":                      map[string]any{"n": viewN, "mean_net_return": nf(viewNet)},
			"baseline_always_long_mean_net_return": nf(alwaysNet),
			"brier_p_up":                           nf(brier), "mean_p_up": nf(meanPUp), "realized_up_rate": nf(upRate),
		})
	}
	response.JSON(w, http.StatusOK, map[string]any{"horizons": out, "disclaimer": predictionDisclaimer})
}
