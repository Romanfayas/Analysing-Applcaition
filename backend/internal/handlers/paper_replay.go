package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
	"github.com/halal-equity/backend/pkg/response"
)

// liveOrderWindow: an execute_at this close to "now" is treated as an ordinary live order.
const liveOrderWindow = 2 * time.Minute

var whenLayouts = []string{
	time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02",
}

// parseWhen reads an optional timestamp. Values without a zone are Indian market time. A bare date means the close
// of that session (15:30 IST), because "what if I had bought on that day" most naturally means the day's close.
func parseWhen(raw *string) (*time.Time, error) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, nil
	}
	s := strings.TrimSpace(*raw)
	for _, l := range whenLayouts {
		t, err := time.ParseInLocation(l, s, market.Location)
		if err != nil {
			continue
		}
		if l == "2006-01-02" {
			t = time.Date(t.Year(), t.Month(), t.Day(), 15, 30, 0, 0, market.Location)
		}
		return &t, nil
	}
	return nil, fmt.Errorf("could not read time %q; use YYYY-MM-DD or YYYY-MM-DDTHH:MM (IST)", s)
}

// PriceAt previews the price an order would fill at for a requested time, without placing anything.
func (h *PaperTradingHandler) PriceAt(w http.ResponseWriter, r *http.Request) {
	if _, ok := userIDFrom(w, r); !ok {
		return
	}
	sym, err := h.db.GetSymbolByTicker(r.Context(), strings.TrimSpace(r.URL.Query().Get("symbol")))
	if err != nil || sym == nil || !sym.IsActive {
		response.Error(w, http.StatusBadRequest, "symbol not found")
		return
	}
	at := r.URL.Query().Get("at")
	when, err := parseWhen(&at)
	if err != nil || when == nil {
		response.Error(w, http.StatusBadRequest, "at is required: YYYY-MM-DD or YYYY-MM-DDTHH:MM (IST)")
		return
	}
	if when.After(time.Now()) {
		response.Error(w, http.StatusBadRequest, "at is in the future")
		return
	}
	pp, err := h.db.PriceAt(r.Context(), sym.ID, *when)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{"symbol": sym.Symbol, "requested_at": when, "price": pp})
}

type simulateRequest struct {
	Symbol string  `json:"symbol"`
	Shares int     `json:"shares"`
	From   string  `json:"from"`
	To     *string `json:"to,omitempty"` // omitted → the latest stored price
	// Product is DELIVERY (default: held overnight, delivery charges) or INTRADAY (same session, lower STT, brokerage).
	Product string `json:"product,omitempty"`
	// Optional protective orders, as a percent of the entry price. Tested against the bars that actually followed.
	StopLossPct     float64 `json:"stop_loss_pct,omitempty"`
	TargetPct       float64 `json:"target_pct,omitempty"`
	TrailingStopPct float64 `json:"trailing_stop_pct,omitempty"`
}

const (
	productDelivery = "DELIVERY"
	productIntraday = "INTRADAY"
)

func sameSession(a, b time.Time) bool {
	a, b = a.In(market.Location), b.In(market.Location)
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

// Simulate answers "what would a BUY at `from` held until `to` have returned?". It is read-only: nothing is written
// to the ledger. The answer is net of statutory charges and brokerage, of slippage that grows with the order's size
// relative to what the stock normally trades, and (optionally) of protective orders tested on the bars that followed.
func (h *PaperTradingHandler) Simulate(w http.ResponseWriter, r *http.Request) {
	if _, ok := userIDFrom(w, r); !ok {
		return
	}
	var req simulateRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Shares <= 0 || req.Shares > maxTradeShares {
		response.Error(w, http.StatusBadRequest, fmt.Sprintf("shares must be between 1 and %d", maxTradeShares))
		return
	}
	req.Product = strings.ToUpper(strings.TrimSpace(req.Product))
	if req.Product == "" {
		req.Product = productDelivery
	}
	if req.Product != productDelivery && req.Product != productIntraday {
		response.Error(w, http.StatusBadRequest, "product must be DELIVERY or INTRADAY")
		return
	}
	stops := paper.StopConfig{StopLossPct: req.StopLossPct, TargetPct: req.TargetPct, TrailingStopPct: req.TrailingStopPct}
	if err := stops.Validate(); err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	sym, err := h.db.GetSymbolByTicker(r.Context(), strings.TrimSpace(req.Symbol))
	if err != nil || sym == nil || !sym.IsActive {
		response.Error(w, http.StatusBadRequest, "symbol not found")
		return
	}
	from, err := parseWhen(&req.From)
	if err != nil || from == nil {
		response.Error(w, http.StatusBadRequest, "from is required: YYYY-MM-DD or YYYY-MM-DDTHH:MM (IST)")
		return
	}
	now := time.Now()
	to, err := parseWhen(req.To)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if to == nil {
		to = &now
	}
	if from.After(now) || to.After(now.Add(liveOrderWindow)) {
		response.Error(w, http.StatusBadRequest, "times cannot be in the future")
		return
	}
	if !to.After(*from) {
		response.Error(w, http.StatusBadRequest, "to must be after from")
		return
	}
	if req.Product == productIntraday && !sameSession(*from, *to) {
		response.Error(w, http.StatusBadRequest, "an INTRADAY trade must be bought and sold in the same session; use DELIVERY to hold overnight")
		return
	}

	ctx := r.Context()
	entry, err := h.db.PriceAt(ctx, sym.ID, *from)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, "entry: "+err.Error())
		return
	}
	var exit *database.PricePoint
	if req.To == nil {
		px, perr := h.db.GetLatestPrice(ctx, sym.ID)
		if perr != nil || px == nil || px.Close <= 0 {
			response.Error(w, http.StatusUnprocessableEntity, "exit: latest price unavailable")
			return
		}
		bt := px.Timestamp
		exit = &database.PricePoint{Price: px.Close, Source: database.PriceSourceLiveLatest, BarTime: &bt, ResolvedAt: now}
	} else if exit, err = h.db.PriceAt(ctx, sym.ID, *to); err != nil {
		response.Error(w, http.StatusUnprocessableEntity, "exit: "+err.Error())
		return
	}

	warnings := []string{}
	plannedExit := *exit
	exitReason := "END_OF_WINDOW"
	var outcome *paper.Outcome
	if stops.Any() {
		bars, missing, perr := h.db.PathBars(ctx, sym.ID, *from, *to)
		switch {
		case perr != nil:
			warnings = append(warnings, "Protective orders could not be tested: "+perr.Error())
		case len(bars) == 0:
			warnings = append(warnings, "No stored price bars cover this window, so the protective orders could not be tested; the result is a plain hold.")
		default:
			o := paper.Evaluate(entry.Price, bars, stops)
			outcome = &o
			if o.Triggered {
				bs := o.BarStart
				exit = &database.PricePoint{Price: o.Price, Source: o.Reason, BarTime: &bs, ResolvedAt: o.BarStart,
					Note: "protective order filled on the " + strings.ToLower(strings.ReplaceAll(o.Basis, "_", " ")) + " bar"}
				exitReason = o.Reason
			}
			if len(missing) > 0 {
				warnings = append(warnings, fmt.Sprintf("No price bars are stored for %d session(s) in the window (%s); a stop could have triggered there unseen.", len(missing), strings.Join(missing, ", ")))
			}
			if o.Ambiguous {
				warnings = append(warnings, "One bar touched both the stop and the target; the order inside a bar is unknowable, so the stop is assumed to have filled first.")
			}
			if o.Gap {
				warnings = append(warnings, "The price gapped through the level, so the fill is the opening price, not the level you set.")
			}
		}
	}

	// Slippage: the platform's 5 bps per side plus a square-root market-impact term sized from the stock's own volume.
	qty := float64(req.Shares)
	var liq *paper.Liquidity
	if adv, vol, n, ok, lerr := h.db.LiquidityBefore(ctx, sym.ID, *from); lerr == nil && ok {
		liq = paper.NewLiquidity(adv, vol, n, qty)
	}
	slipBps := paper.BaseSlippageBps
	if liq != nil {
		slipBps += liq.ImpactBps
		if liq.Participation >= 1 {
			warnings = append(warnings, fmt.Sprintf("This order is %.1f%% of the stock's average daily volume; a real order that size would move the price and may not fill in full.", liq.Participation))
		}
	} else {
		warnings = append(warnings, "No exchange volume history is stored for this stock, so only the base slippage is applied and market impact is not sized.")
	}
	buyFill := entry.Price * (1 + slipBps/1e4)
	sellFill := exit.Price * (1 - slipBps/1e4)

	costs := database.CalculateDeliveryTransactionCosts
	if req.Product == productIntraday {
		costs = database.CalculateIntradayTransactionCosts
	}
	buyCost := costs("BUY", buyFill*qty)
	sellCost := costs("SELL", sellFill*qty)
	invested := buyFill*qty + buyCost.TotalFees
	proceeds := sellFill*qty - sellCost.TotalFees
	pnl := proceeds - invested
	retPct := pnl / invested * 100

	// What the same trade would have returned with no protective orders, so their effect is visible.
	var without map[string]any
	if outcome != nil && outcome.Triggered {
		wSell := plannedExit.Price * (1 - slipBps/1e4)
		wProceeds := wSell*qty - costs("SELL", wSell*qty).TotalFees
		without = map[string]any{"exit": plannedExit, "net_pnl": wProceeds - invested, "net_return_pct": (wProceeds - invested) / invested * 100}
	}

	// Benchmark: NIFTY 50 over the same dates, from NSE's official daily closes (daily granularity only).
	var bench map[string]any
	exitDay := *to
	if exit.BarTime != nil && outcome != nil && outcome.Triggered {
		exitDay = *exit.BarTime
	}
	if a, okA, _ := h.db.IndexCloseOnOrBefore(ctx, database.IndexNifty50, from.In(market.Location)); okA {
		if b, okB, _ := h.db.IndexCloseOnOrBefore(ctx, database.IndexNifty50, exitDay.In(market.Location)); okB && b.Date.After(a.Date) {
			r := (b.Close/a.Close - 1) * 100
			bench = map[string]any{"index": "NIFTY 50", "from_date": a.Date.Format("2006-01-02"), "to_date": b.Date.Format("2006-01-02"),
				"return_pct": r, "excess_return_pct": retPct - r,
				"note": "Daily index closes: the index return excludes dividends and is not comparable inside a single session."}
		}
	}
	if bench == nil {
		note := "Index history for these dates is not loaded yet."
		if sameSession(*from, exitDay) {
			note = "No comparison for a same-day trade: index levels are stored as daily closes."
		}
		bench = map[string]any{"index": "NIFTY 50", "note": note}
	}

	if entry.Source != database.PriceSourceIntraday1M && entry.Source != database.PriceSourceIntraday5M {
		warnings = append(warnings, "Entry uses a daily close, not an intraday price: "+entry.Note)
	}
	if req.Product == productIntraday {
		warnings = append(warnings, "Intraday trades are normally squared off by the broker around 15:15-15:20 IST, and many scholars require delivery for a sale to be valid; confirm the Shariah position of same-day trading before relying on it.")
	}
	evs, _ := h.db.GetCorporateActionEvents(ctx, sym.Symbol)
	actions := []map[string]any{}
	for _, ev := range evs {
		if ev.ActionType == "DIVIDEND" || ev.ActionType == "CASH_DIVIDEND" || ev.ExDate.Before(*from) || ev.ExDate.After(*to) {
			continue
		}
		actions = append(actions, map[string]any{"type": ev.ActionType, "ex_date": ev.ExDate.Format("2006-01-02"), "subject": ev.SourceSubject})
	}
	if len(actions) > 0 {
		warnings = append(warnings, "A corporate action falls inside this window; prices before and after the ex-date may not be comparable, so the result can be misleading.")
	}
	// The stored corporate-action table is known to be incomplete for splits and bonuses, so absence is not proof.
	warnings = append(warnings, "Splits and bonus issues may be missing from the stored corporate-action list; check the exchange before trusting a long window.")

	log.Debug().Str("symbol", sym.Symbol).Msg("paper simulate")
	response.JSON(w, http.StatusOK, map[string]any{
		"symbol": sym.Symbol, "shares": req.Shares, "product": req.Product, "from": from, "to": to,
		"entry": entry, "exit": exit, "exit_reason": exitReason,
		"protective_orders": map[string]any{"config": stops, "outcome": outcome, "without_protection": without},
		"slippage": map[string]any{
			"base_bps": paper.BaseSlippageBps, "total_bps_each_side": slipBps, "liquidity": liq,
			"assumption": "Slippage is a modelled parameter (5 bps per side plus a square-root market-impact term from the stock's own volume and volatility), not an observed fill.",
		},
		"buy_costs": buyCost, "sell_costs": sellCost,
		"invested": invested, "proceeds": proceeds, "net_pnl": pnl, "net_return_pct": retPct,
		"benchmark": bench, "corporate_actions": actions, "warnings": warnings,
		"disclaimer": "Hypothetical result on stored prices. Nothing was written to your ledger.",
	})
}
