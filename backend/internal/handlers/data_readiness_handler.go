package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/pkg/response"
)

// Next-day (daily-horizon) prediction research needs this much research-eligible history.
const (
	nextDayMinEligibleSessions = 250
	nextDayMinSymbols          = 30
)

// quarantineReleaseFloor mirrors the hardcoded date in fn_ohlcv_daily_quarantine_guard: bars before it
// cannot be released by any evidence until the guard is deliberately changed.
const quarantineReleaseFloor = "2026-10-05"

// GetHistoricalReadiness reports, per instrument, what exchange evidence exists and exactly what still blocks
// research eligibility. It changes nothing; every number is measured at request time.
func (h *DataQualityHandler) GetHistoricalReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var runID sql.NullInt64
	var runAt sql.NullTime
	_ = h.db.QueryRowContext(ctx, `SELECT id, run_at FROM ca_reconciliation_runs ORDER BY id DESC LIMIT 1`).Scan(&runID, &runAt)

	rows, err := h.db.QueryContext(ctx, `
		WITH v AS (
			SELECT d.symbol_id, count(*) AS vendor_bars,
			       count(*) FILTER (WHERE d.adjusted_close IS NULL) AS quarantined,
			       count(*) FILTER (WHERE `+database.StrictResearchEligible+`) AS eligible,
			       count(*) FILTER (WHERE d.adjusted_close IS NOT NULL AND NOT `+database.StrictResearchEligible+`) AS revoked
			FROM ohlcv_daily d GROUP BY d.symbol_id),
		e AS (SELECT symbol_id, count(*) AS exchange_bars, min(trade_date) AS ex_first, max(trade_date) AS ex_last
		      FROM exchange_daily_bars GROUP BY symbol_id),
		rc AS (SELECT symbol_id,
		              count(*) FILTER (WHERE classification = 'NOMINAL_MATCH') AS nominal_match,
		              count(*) FILTER (WHERE classification = 'VENDOR_ADJUSTED') AS vendor_adjusted,
		              count(*) FILTER (WHERE classification = 'MISMATCH') AS mismatch,
		              count(*) FILTER (WHERE classification = 'MISSING_VENDOR_BAR') AS missing_vendor,
		              count(*) FILTER (WHERE classification = 'MISSING_EXCHANGE_BAR') AS missing_exchange
		       FROM evidence.v_daily_bar_reconciliation GROUP BY symbol_id),
		f AS (SELECT symbol_id,
		             count(*) FILTER (WHERE finding_type = 'FEED_ACTION_MISSING_FROM_CANONICAL' AND action_class IN ('BONUS','SPLIT','DEMERGER')) AS missing_price_actions,
		             count(*) FILTER (WHERE finding_type = 'FEED_ACTION_MISSING_FROM_CANONICAL' AND action_class NOT IN ('BONUS','SPLIT','DEMERGER')) AS missing_other_actions,
		             count(*) FILTER (WHERE finding_type = 'CANONICAL_CONFIRMED_BY_FEED') AS confirmed_pending_review,
		             count(*) FILTER (WHERE finding_type = 'PARSER_FRAGMENT') AS parser_fragments,
		             bool_or(finding_type = 'FEED_UNAVAILABLE') AS feed_unavailable
		      FROM ca_reconciliation_findings WHERE run_id = $1 GROUP BY symbol_id),
		ev AS (SELECT symbol_id, count(*) AS events,
		              count(*) FILTER (WHERE verification_status = 'VERIFIED_EXCHANGE_PRIMARY') AS verified_events
		       FROM corporate_action_events GROUP BY symbol_id)
		SELECT s.symbol, s.is_active, COALESCE(v.vendor_bars,0), COALESCE(v.quarantined,0), COALESCE(v.eligible,0), COALESCE(v.revoked,0),
		       COALESCE(e.exchange_bars,0), e.ex_first, e.ex_last,
		       COALESCE(rc.nominal_match,0), COALESCE(rc.vendor_adjusted,0), COALESCE(rc.mismatch,0),
		       COALESCE(rc.missing_vendor,0), COALESCE(rc.missing_exchange,0),
		       COALESCE(f.missing_price_actions,0), COALESCE(f.missing_other_actions,0),
		       COALESCE(f.confirmed_pending_review,0), COALESCE(f.parser_fragments,0), COALESCE(f.feed_unavailable,false),
		       COALESCE(ev.events,0), COALESCE(ev.verified_events,0)
		FROM symbols s
		LEFT JOIN v ON v.symbol_id = s.id LEFT JOIN e ON e.symbol_id = s.id LEFT JOIN rc ON rc.symbol_id = s.id
		LEFT JOIN f ON f.symbol_id = s.id LEFT JOIN ev ON ev.symbol_id = s.id
		WHERE (v.symbol_id IS NOT NULL OR e.symbol_id IS NOT NULL) AND COALESCE(s.metadata->>'user_added', 'false') <> 'true'
		ORDER BY s.symbol`, runID)
	if err != nil {
		log.Error().Err(err).Msg("historical readiness query failed")
		response.Error(w, http.StatusInternalServerError, "failed to evaluate historical readiness")
		return
	}
	defer rows.Close()

	type item struct {
		Symbol                 string   `json:"symbol"`
		IsActive               bool     `json:"is_active"`
		VendorBars             int      `json:"vendor_bars"`
		Quarantined            int      `json:"quarantined"`
		Eligible               int      `json:"research_eligible"`
		Revoked                int      `json:"eligibility_revoked"`
		ExchangeBars           int      `json:"exchange_bars"`
		ExchangeFirst          *string  `json:"exchange_first"`
		ExchangeLast           *string  `json:"exchange_last"`
		NominalMatch           int      `json:"nominal_match"`
		VendorAdjusted         int      `json:"vendor_adjusted"`
		Mismatch               int      `json:"mismatch"`
		MissingVendor          int      `json:"missing_vendor_bars"`
		MissingExchange        int      `json:"missing_exchange_bars"`
		MissingPriceActions    int      `json:"missing_price_changing_actions"`
		MissingOtherActions    int      `json:"missing_other_actions"`
		ConfirmedPendingReview int      `json:"events_confirmed_pending_review"`
		ParserFragments        int      `json:"parser_fragments"`
		FeedUnavailable        bool     `json:"feed_unavailable"`
		CanonicalEvents        int      `json:"canonical_events"`
		VerifiedEvents         int      `json:"verified_events"`
		Status                 string   `json:"status"`
		Blockers               []string `json:"blockers"`
	}
	list := []item{}
	symbolsReadyForNextDay := 0
	for rows.Next() {
		var it item
		var exFirst, exLast sql.NullTime
		if err := rows.Scan(&it.Symbol, &it.IsActive, &it.VendorBars, &it.Quarantined, &it.Eligible, &it.Revoked, &it.ExchangeBars, &exFirst, &exLast,
			&it.NominalMatch, &it.VendorAdjusted, &it.Mismatch, &it.MissingVendor, &it.MissingExchange,
			&it.MissingPriceActions, &it.MissingOtherActions, &it.ConfirmedPendingReview, &it.ParserFragments, &it.FeedUnavailable,
			&it.CanonicalEvents, &it.VerifiedEvents); err != nil {
			response.Error(w, http.StatusInternalServerError, "failed to read readiness")
			return
		}
		if exFirst.Valid {
			a, b := exFirst.Time.Format("2006-01-02"), exLast.Time.Format("2006-01-02")
			it.ExchangeFirst, it.ExchangeLast = &a, &b
		}
		it.Blockers = []string{}
		if it.Quarantined > 0 {
			it.Blockers = append(it.Blockers, "quarantined vendor bars remain (release requires verified corporate-action factors)")
		}
		if it.Revoked > 0 {
			it.Blockers = append(it.Blockers, "bars released earlier are no longer backed by a verified corporate-action event (eligibility revoked at read time)")
		}
		if it.ExchangeBars == 0 {
			it.Blockers = append(it.Blockers, "no exchange (bhavcopy) evidence loaded")
		}
		if it.VendorAdjusted > 0 {
			it.Blockers = append(it.Blockers, "vendor history is back-adjusted, not the traded price")
		}
		if it.Mismatch > 0 {
			it.Blockers = append(it.Blockers, "vendor bars disagree with the exchange record")
		}
		if it.MissingPriceActions > 0 {
			it.Blockers = append(it.Blockers, "exchange lists bonus/split/demerger actions the platform has not recorded")
		}
		if runID.Valid && it.FeedUnavailable {
			it.Blockers = append(it.Blockers, "no exchange corporate-actions feed evidence")
		}
		if it.CanonicalEvents > it.VerifiedEvents {
			it.Blockers = append(it.Blockers, "corporate-action events await operator verification")
		}
		if len(it.Blockers) == 0 {
			it.Status = "READY"
		} else {
			it.Status = "BLOCKED"
		}
		if it.Eligible >= nextDayMinEligibleSessions {
			symbolsReadyForNextDay++
		}
		list = append(list, it)
	}

	nextDay := map[string]any{
		"status":                      "DATA_INSUFFICIENT",
		"required_eligible_sessions":  nextDayMinEligibleSessions,
		"required_symbols":            nextDayMinSymbols,
		"symbols_meeting_requirement": symbolsReadyForNextDay,
		"note":                        "No next-day model is trained or shown until enough research-eligible daily history exists.",
	}
	if symbolsReadyForNextDay >= nextDayMinSymbols {
		nextDay["status"] = "READY_FOR_RESEARCH"
	}
	var runInfo any
	if runID.Valid {
		runInfo = map[string]any{"id": runID.Int64, "run_at": runAt.Time}
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"symbols":                      list,
		"latest_reconciliation_run":    runInfo,
		"next_day_prediction_research": nextDay,
		"research_series":              h.researchSeries(r),
		"quarantine_release_floor":     quarantineReleaseFloor,
		"policy_note": "Bars dated before " + quarantineReleaseFloor + " cannot leave quarantine under the current guard, whatever evidence exists. " +
			"Changing that is an owner decision. Exchange evidence here is stored as AUTHENTICITY_PENDING and is never promoted automatically.",
	})
}

// GetIntradayFeedHealth measures the intraday feed: coverage per session, structural gaps and (from P1.9.24 on)
// receipt latency. Latency before first_received_at existed is reported as unknown, never estimated.
func (h *DataQualityHandler) GetIntradayFeedHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	type day struct {
		Date        string  `json:"date"`
		Symbols     int     `json:"symbols"`
		ValidBars   int     `json:"valid_bars"`
		SuspectBars int     `json:"suspect_bars"`
		CoveragePct float64 `json:"coverage_pct_of_375_minutes"`
	}
	days := []day{}
	rows, err := h.db.QueryContext(ctx, `
		SELECT (timestamp AT TIME ZONE 'Asia/Kolkata')::date AS d, count(DISTINCT symbol_id),
		       count(*) FILTER (WHERE quality_status = 'VALID'), count(*) FILTER (WHERE quality_status <> 'VALID')
		FROM ohlcv_intraday WHERE interval_minutes = 1 AND timestamp > now() - interval '45 days'
		GROUP BY 1 ORDER BY 1 DESC`)
	if err != nil {
		log.Error().Err(err).Msg("feed health query failed")
		response.Error(w, http.StatusInternalServerError, "failed to evaluate intraday feed")
		return
	}
	for rows.Next() {
		var d time.Time
		var x day
		if err := rows.Scan(&d, &x.Symbols, &x.ValidBars, &x.SuspectBars); err != nil {
			rows.Close()
			response.Error(w, http.StatusInternalServerError, "failed to read intraday feed")
			return
		}
		x.Date = d.Format("2006-01-02")
		if x.Symbols > 0 {
			x.CoveragePct = float64(x.ValidBars) / float64(x.Symbols*375) * 100
		}
		days = append(days, x)
	}
	rows.Close()

	type gap struct {
		Minute     string  `json:"minute_ist"`
		MissingPct float64 `json:"missing_pct_of_symbol_days"`
	}
	gaps := []gap{}
	grows, err := h.db.QueryContext(ctx, `
		WITH m AS (SELECT symbol_id, (timestamp AT TIME ZONE 'Asia/Kolkata') AS t FROM ohlcv_intraday
		           WHERE interval_minutes = 1 AND quality_status = 'VALID' AND timestamp > now() - interval '45 days'),
		     g AS (SELECT DISTINCT symbol_id, t::date AS d FROM m)
		SELECT to_char(g.d + make_interval(mins => i), 'HH24:MI') AS hhmm,
		       100.0 * count(*) FILTER (WHERE m.t IS NULL) / count(*) AS missing_pct
		FROM g CROSS JOIN generate_series(555, 929) AS i
		LEFT JOIN m ON m.symbol_id = g.symbol_id AND m.t = g.d + make_interval(mins => i)
		GROUP BY 1 HAVING 100.0 * count(*) FILTER (WHERE m.t IS NULL) / count(*) > 20
		ORDER BY 1`)
	if err == nil {
		for grows.Next() {
			var x gap
			if grows.Scan(&x.Minute, &x.MissingPct) == nil {
				gaps = append(gaps, x)
			}
		}
		grows.Close()
	}

	var partial int
	_ = h.db.QueryRowContext(r.Context(), `SELECT count(*) FROM ohlcv_intraday WHERE first_received_at IS NOT NULL
		AND timestamp + interval '1 minute' > first_received_at`).Scan(&partial)
	latency := map[string]any{
		"bars_first_seen_incomplete": partial,
		"definition": "Live observations only: bars first received during the trading session on the day they formed " +
			"The provider returns the still-forming current minute; such partial bars are excluded (bars_first_seen_incomplete). " +
			"bar_receipt = first receipt minus the minute bar's close, counting only bars complete when received and formed after the symbol's previous poll " +
			"that day (the day's first poll and catch-up/recovery batches are excluded); poll_freshness = age of the newest bar " +
			"at each poll. Receipt times were not recorded before P1.9.24.",
		"bar_receipt":    measureLatency(r, h, false),
		"poll_freshness": measureLatency(r, h, true),
	}

	truedataConfigured := os.Getenv("TRUEDATA_USERNAME") != "" || os.Getenv("TRUEDATA_API_KEY") != ""
	response.JSON(w, http.StatusOK, map[string]any{
		"provider":              "yahoo_finance (free, delayed, not exchange-certified)",
		"poll_interval_minutes": os.Getenv("INTRADAY_POLL_MINUTES"),
		"sessions":              days,
		"structural_gaps":       gaps,
		"latency":               latency,
		"recovery":              "Each poll re-requests the earliest past session (within the provider's ~7-day window) holding fewer than 300 valid bars. Holes older than the window are unrecoverable from this provider.",
		"truedata": map[string]any{
			"configured": truedataConfigured,
			"status":     map[bool]string{true: "CONFIGURED_NOT_EVALUATED_HERE", false: "NOT_CONFIGURED: no credentials, so latency, completeness and recovery cannot be measured"}[truedataConfigured],
		},
	})
}

// measureLatency returns percentiles of live receipt latency, or status NOT_MEASURED when no live data exists.
// Nothing is estimated: with zero observations every percentile stays null.
func measureLatency(r *http.Request, h *DataQualityHandler, perPoll bool) map[string]any {
	// A "poll" is one symbol's batch: every row inserted together shares first_received_at (DEFAULT now()).
	// Only polls made during the session on the day the bars formed are live. For bar receipt latency a bar
	// must also have formed after the symbol's previous poll that day, which excludes the day's first poll and
	// any catch-up/recovery batch (their delay measures downtime, not the feed).
	const base = `
		WITH live AS (
			SELECT symbol_id, timestamp, first_received_at
			FROM ohlcv_intraday
			WHERE first_received_at IS NOT NULL
			  AND (first_received_at AT TIME ZONE 'Asia/Kolkata')::date = (timestamp AT TIME ZONE 'Asia/Kolkata')::date
			  AND (first_received_at AT TIME ZONE 'Asia/Kolkata')::time BETWEEN '09:15' AND '15:35'),
		polls AS (
			SELECT symbol_id, first_received_at AS poll_at,
			       max(timestamp) FILTER (WHERE timestamp + interval '1 minute' <= first_received_at) AS newest,
			       lag(first_received_at) OVER (PARTITION BY symbol_id, (first_received_at AT TIME ZONE 'Asia/Kolkata')::date
			                                    ORDER BY first_received_at) AS prev_poll
			FROM live GROUP BY symbol_id, first_received_at)`
	src := base + `
		SELECT extract(epoch FROM p.poll_at - (l.timestamp + interval '1 minute')) AS lat
		FROM live l JOIN polls p ON p.symbol_id = l.symbol_id AND p.poll_at = l.first_received_at
		WHERE p.prev_poll IS NOT NULL AND l.timestamp + interval '1 minute' > p.prev_poll
		  AND l.timestamp + interval '1 minute' <= p.poll_at`
	if perPoll {
		src = base + `
		SELECT extract(epoch FROM poll_at - (newest + interval '1 minute')) AS lat FROM polls WHERE newest IS NOT NULL`
	}
	var n int
	var p50, p95, p99, maxv sql.NullFloat64
	err := h.db.QueryRowContext(r.Context(), `SELECT count(*),
		percentile_cont(0.50) WITHIN GROUP (ORDER BY lat), percentile_cont(0.95) WITHIN GROUP (ORDER BY lat),
		percentile_cont(0.99) WITHIN GROUP (ORDER BY lat), max(lat) FROM (`+src+`) x`).Scan(&n, &p50, &p95, &p99, &maxv)
	out := map[string]any{"observations": n, "median_seconds": nil, "p95_seconds": nil, "p99_seconds": nil, "max_seconds": nil}
	switch {
	case err != nil:
		out["status"] = "ERROR"
	case n == 0:
		out["status"] = "NOT_MEASURED"
	default:
		out["status"] = "MEASURED"
		if n < 100 {
			out["status"] = "MEASURED_LOW_SAMPLE"
		}
		out["median_seconds"], out["p95_seconds"], out["p99_seconds"], out["max_seconds"] = p50.Float64, p95.Float64, p99.Float64, maxv.Float64
	}
	return out
}

// researchSeries summarises the price series built from NSE's own files, which is usable for research today even though
// the vendor history above stays quarantined. Nil until a research run exists.
func (h *DataQualityHandler) researchSeries(r *http.Request) map[string]any {
	run, at, err := h.db.LatestResearchRun(r.Context(), "BREAKOUT_FOLLOW_THROUGH")
	if err != nil {
		return nil
	}
	s, _ := run["series"].(map[string]any)
	if s == nil {
		return nil
	}
	return map[string]any{"status": "READY_FOR_RESEARCH", "built_at": at, "series": s}
}
