package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/quant"
	"github.com/halal-equity/backend/internal/workers/news"
	"github.com/halal-equity/backend/pkg/response"
)

// BreakoutsHandler serves the Breakouts pages: today's upcoming and confirmed breakouts with their reasons, everything tracked so far
// with outcomes, per-stock tests, and the automatic paper portfolios.
type BreakoutsHandler struct {
	db        *database.DB
	quant     *quant.Client
	refresher *news.Refresher
}

func NewBreakoutsHandler(db *database.DB, q *quant.Client, refresher *news.Refresher) *BreakoutsHandler {
	return &BreakoutsHandler{db: db, quant: q, refresher: refresher}
}

func (h *BreakoutsHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/overview", h.Overview)
	r.Get("/upcoming", h.Upcoming)
	r.Get("/stock/{symbol}", h.Stock)
	return r
}

// Overview is everything the Breakouts page shows, in one call.
func (h *BreakoutsHandler) Overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{}
	signals, err := h.db.ListTrackedBreakouts(ctx, 200, "")
	if err != nil {
		log.Error().Err(err).Msg("breakouts overview: signals")
		response.Error(w, http.StatusInternalServerError, "failed to load tracked breakouts")
		return
	}
	score, err := h.db.BreakoutScores(ctx)
	if err != nil {
		log.Error().Err(err).Msg("breakouts overview: scorecard")
		response.Error(w, http.StatusInternalServerError, "failed to load the breakout scorecard")
		return
	}
	flags, err := h.db.ListWatch(ctx, "", "", 200)
	if err != nil {
		log.Error().Err(err).Msg("breakouts overview: watch")
		response.Error(w, http.StatusInternalServerError, "failed to load upcoming breakouts")
		return
	}
	out["confirmed"] = map[string]any{"signals": signals, "scorecard": score}
	upcoming := map[string]any{"flags": flags}
	for _, o := range []struct{ key, origin string }{{"all", ""}, {"live", "LIVE"}, {"history_replay", "HISTORY_REPLAY"}} {
		if sc, err := h.db.WatchScores(ctx, o.origin); err == nil {
			upcoming["scorecard_"+o.key] = sc
		}
	}
	out["upcoming"] = upcoming
	if run, at, err := h.db.LatestResearchRun(ctx, "CANDIDATE_LIFT"); err == nil {
		out["candidate_lift"] = map[string]any{"result": run, "run_at": at}
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Warn().Err(err).Msg("candidate lift unavailable")
	}
	if scan, err := getBreakoutScan(ctx, h.db, h.quant); err == nil {
		out["today"] = todayView(scan)
	} else {
		out["today"] = map[string]any{"available": false, "error": "the breakout scan is unavailable"}
		log.Warn().Err(err).Msg("breakouts overview: scan")
	}
	if auto, err := h.db.AutoSummaries(ctx); err == nil {
		brief := []map[string]any{}
		for _, a := range auto {
			brief = append(brief, map[string]any{"portfolio_id": a.PortfolioID, "label": a.Label, "closed_count": a.ClosedCount, "wins": a.Wins, "net_pnl_closed": a.NetPnl,
				"return_on_capital_pct": a.ReturnOnCapital, "open_positions": len(a.Open), "reconciled": a.Reconciled, "processed_through": a.ProcessedTo})
		}
		out["auto_paper"] = brief
	}
	response.JSON(w, http.StatusOK, out)
}

// todayView lists the stocks that are upcoming or confirmed breakouts on the latest completed session, with reasons and where the price is now.
func todayView(scan *BreakoutScan) map[string]any {
	conf, up := []quant.BreakoutResult{}, []quant.BreakoutResult{}
	counts := map[string]int{}
	for _, c := range scan.Candidates {
		counts[c.Stage]++
		switch c.Stage {
		case "CONFIRMED_BREAKOUT":
			conf = append(conf, c)
		case "BREAKOUT_CANDIDATE":
			up = append(up, c)
		}
	}
	return map[string]any{"available": true, "data_as_of": scan.DataAsOf, "data_quality": scan.DataQuality, "computed_at": scan.ComputedAt, "basis": scan.Basis,
		"total_screened": scan.TotalScreened, "stage_counts": counts, "confirmed": conf, "upcoming": up, "excluded": scan.Excluded}
}

// Stock is the per-stock Breakouts tab: today's reading, every signal and flag for this stock, and its own test results.
func (h *BreakoutsHandler) Stock(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sym := strings.ToUpper(strings.TrimSpace(symbolParam(r)))
	rec, err := h.db.GetSymbolByTicker(ctx, sym)
	if err != nil || rec == nil {
		response.Error(w, http.StatusNotFound, "unknown symbol")
		return
	}
	out := map[string]any{"symbol": rec.Symbol, "name": rec.Name}
	if scan, err := getBreakoutScan(ctx, h.db, h.quant); err == nil {
		if c, ok := scan.For(rec.Symbol); ok {
			out["today"] = map[string]any{"available": true, "data_as_of": scan.DataAsOf, "reading": c}
		} else {
			out["today"] = map[string]any{"available": false, "reason": excludedReason(scan, rec.Symbol)}
		}
	}
	signals, _ := h.db.ListTrackedBreakouts(ctx, 100, rec.Symbol)
	flags, _ := h.db.ListWatch(ctx, rec.Symbol, "", 100)
	out["signals"], out["flags"] = signals, flags
	if st, asOf, err := h.db.StockStats(ctx, rec.Symbol); err == nil && st != nil {
		out["test"] = map[string]any{"result": st, "as_of": asOf}
	} else {
		out["test"] = nil
	}
	response.JSON(w, http.StatusOK, out)
}

func excludedReason(scan *BreakoutScan, symbol string) string {
	for _, e := range scan.Excluded {
		if e.Symbol == symbol {
			return e.Reason
		}
	}
	return "not part of the scanned universe"
}

// AutoPaperHandler serves the automatic paper portfolios.
type AutoPaperHandler struct{ db *database.DB }

func NewAutoPaperHandler(db *database.DB) *AutoPaperHandler { return &AutoPaperHandler{db: db} }

func (h *AutoPaperHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/summary", h.Summary)
	return r
}

// Summary returns every automatic portfolio with open positions, closed trades, skips with reasons, and the ledger reconciliation.
func (h *AutoPaperHandler) Summary(w http.ResponseWriter, r *http.Request) {
	s, err := h.db.AutoSummaries(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("auto paper summary failed")
		response.Error(w, http.StatusInternalServerError, "failed to load the automatic paper portfolios")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"portfolios": s})
}

// StockFeedHandler serves news, official announcements and events for a stock, refreshing stale data on demand.
type StockFeedHandler struct {
	db        *database.DB
	refresher *news.Refresher
}

func NewStockFeedHandler(db *database.DB, r *news.Refresher) *StockFeedHandler {
	return &StockFeedHandler{db: db, refresher: r}
}

func (h *StockFeedHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/{symbol}/news", h.News)
	r.Get("/{symbol}/events", h.Events)
	return r
}

func (h *StockFeedHandler) symbol(w http.ResponseWriter, r *http.Request) (*database.SymbolRecord, bool) {
	rec, err := h.db.GetSymbolByTicker(r.Context(), strings.ToUpper(strings.TrimSpace(symbolParam(r))))
	if err != nil || rec == nil {
		response.Error(w, http.StatusNotFound, "unknown symbol")
		return nil, false
	}
	return rec, true
}

// feedStatus tells the page when each feed last succeeded and whether the latest attempt failed.
func (h *StockFeedHandler) feedStatus(ctx context.Context, id int, feeds ...string) map[string]any {
	out := map[string]any{}
	for _, f := range feeds {
		st := map[string]any{"last_success": nil, "last_error": nil}
		if t, err := h.db.LastNewsFetch(ctx, id, f); err == nil && !t.IsZero() {
			st["last_success"] = t
		}
		if e := h.db.LastNewsError(ctx, id, f); e != "" {
			st["last_error"] = e
		}
		out[strings.ToLower(f)] = st
	}
	return out
}

func (h *StockFeedHandler) refresh(r *http.Request, rec *database.SymbolRecord) {
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	h.refresher.Refresh(ctx, *rec, news.Freshness)
}

// News returns headlines (unofficial) and official exchange announcements.
func (h *StockFeedHandler) News(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.symbol(w, r)
	if !ok {
		return
	}
	h.refresh(r, rec)
	ctx := r.Context()
	headlines, err1 := h.db.ListNews(ctx, rec.ID, 40)
	filings, err2 := h.db.ListFilings(ctx, rec.ID, 40)
	if err1 != nil || err2 != nil {
		log.Error().Err(errors.Join(err1, err2)).Msg("stock news read failed")
		response.Error(w, http.StatusInternalServerError, "failed to load news")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"symbol": rec.Symbol, "name": rec.Name, "headlines": headlines, "filings": filings, "status": h.feedStatus(ctx, rec.ID, "NEWS", "FILINGS"),
		"sources": map[string]string{
			"headlines": "Google News RSS: headlines from third-party publishers. Unofficial; no sentiment or accuracy is claimed. Open the link for the full story.",
			"filings":   "NSE corporate announcements: official exchange records with the filed document.",
		},
	})
}

// Events returns upcoming and recent board meetings, dividends, splits and other corporate actions (NSE).
func (h *StockFeedHandler) Events(w http.ResponseWriter, r *http.Request) {
	rec, ok := h.symbol(w, r)
	if !ok {
		return
	}
	h.refresh(r, rec)
	ctx := r.Context()
	up, err1 := h.db.ListEvents(ctx, rec.ID, true, 30)
	past, err2 := h.db.ListEvents(ctx, rec.ID, false, 30)
	if err1 != nil || err2 != nil {
		log.Error().Err(errors.Join(err1, err2)).Msg("stock events read failed")
		response.Error(w, http.StatusInternalServerError, "failed to load events")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"symbol": rec.Symbol, "name": rec.Name, "upcoming": up, "recent": past, "status": h.feedStatus(ctx, rec.ID, "EVENTS"),
		"source": "NSE event calendar and corporate actions: official exchange records. Dates are as announced and can change."})
}

// closestToBreakout ranks the stocks the engine only WATCHES: still below resistance, nearest first, then by structure score. They are not
// forecasts; the page labels them as "closest to breaking out" and keeps them apart from the engine's own BREAKOUT_CANDIDATE stage.
func closestToBreakout(scan *BreakoutScan, limit int) []quant.BreakoutResult {
	var out []quant.BreakoutResult
	for _, c := range scan.Candidates {
		if c.Stage == "WATCHLIST" && c.ResistanceLevel > 0 && c.DistanceToResistancePct > 0 && c.DistanceToResistancePct <= 6 {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].DistanceToResistancePct != out[j].DistanceToResistancePct {
			return out[i].DistanceToResistancePct < out[j].DistanceToResistancePct
		}
		si, sj := 0.0, 0.0
		if out[i].StructureScore != nil {
			si = *out[i].StructureScore
		}
		if out[j].StructureScore != nil {
			sj = *out[j].StructureScore
		}
		return si > sj
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Upcoming is the page for stocks about to break out: the engine's own upcoming candidates, then the ones closest to resistance, each
// with the engine's reasons and the stock's latest headlines, official filings and next event.
func (h *BreakoutsHandler) Upcoming(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	scan, err := getBreakoutScan(ctx, h.db, h.quant)
	if err != nil {
		log.Warn().Err(err).Msg("upcoming breakouts: scan unavailable")
		response.Error(w, http.StatusServiceUnavailable, "the breakout scan is unavailable")
		return
	}
	var candidates []quant.BreakoutResult
	for _, c := range scan.Candidates {
		if c.Stage == "BREAKOUT_CANDIDATE" {
			candidates = append(candidates, c)
		}
	}
	closest := closestToBreakout(scan, 12)

	type item struct {
		Result  quant.BreakoutResult `json:"result"`
		Name    string               `json:"name"`
		News    []database.NewsRow   `json:"news"`
		Filings []database.FilingRow `json:"filings"`
		Event   *database.EventRow   `json:"next_event"`
	}
	build := func(list []quant.BreakoutResult) []item {
		out := make([]item, len(list))
		var wg sync.WaitGroup
		for i, c := range list {
			out[i] = item{Result: c, News: []database.NewsRow{}, Filings: []database.FilingRow{}}
			wg.Add(1)
			go func(i int, sym string) {
				defer wg.Done()
				rec, err := h.db.GetSymbolByTicker(ctx, sym)
				if err != nil || rec == nil {
					return
				}
				out[i].Name = rec.Name
				if h.refresher != nil {
					rctx, cancel := context.WithTimeout(ctx, 12*time.Second)
					h.refresher.Refresh(rctx, *rec, news.Freshness)
					cancel()
				}
				if n, err := h.db.ListNews(ctx, rec.ID, 4); err == nil {
					out[i].News = n
				}
				if f, err := h.db.ListFilings(ctx, rec.ID, 2); err == nil {
					out[i].Filings = f
				}
				if ev, err := h.db.ListEvents(ctx, rec.ID, true, 1); err == nil && len(ev) > 0 {
					out[i].Event = &ev[0]
				}
			}(i, c.Symbol)
		}
		wg.Wait()
		return out
	}
	flags, _ := h.db.ListWatch(ctx, "", "WATCHING", 50)
	response.JSON(w, http.StatusOK, map[string]any{
		"data_as_of": scan.DataAsOf, "computed_at": scan.ComputedAt, "total_screened": scan.TotalScreened,
		"candidates": build(candidates), "closest": build(closest), "watching_flags": flags,
		"note": "Upcoming = the engine's own BREAKOUT_CANDIDATE stage. Closest to breaking out = stocks it only watches, ranked by distance to resistance. Neither is a forecast: the nightly test has not shown that either predicts profit.",
	})
}
