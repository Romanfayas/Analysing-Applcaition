package handlers

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/middleware"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
	"github.com/halal-equity/backend/pkg/response"
)

const maxTradeShares = 1_000_000

// ExecuteTradeRequest is a paper order. The fill price is deliberately not part of it: a ledger whose
// prices are chosen by the caller proves nothing, so fills always use stored market data.
type ExecuteTradeRequest struct {
	Symbol    string   `json:"symbol"`
	TradeType string   `json:"trade_type"` // "BUY" or "SELL"
	Shares    int      `json:"shares"`
	Price     *float64 `json:"price,omitempty"` // rejected if present; see validate
	// ExecuteAt runs the order "as of" a past time (IST unless a zone is given). Omit it, or give a time within two
	// minutes of now, for a normal order at the latest price.
	ExecuteAt *string `json:"execute_at,omitempty"`
}

func (r *ExecuteTradeRequest) validate() error {
	if r.Price != nil {
		return fmt.Errorf("price is set by the server from market data; remove the price field")
	}
	if r.Shares <= 0 || r.Shares > maxTradeShares {
		return fmt.Errorf("shares must be between 1 and %d", maxTradeShares)
	}
	r.TradeType = strings.ToUpper(strings.TrimSpace(r.TradeType))
	if r.TradeType != "BUY" && r.TradeType != "SELL" {
		return fmt.Errorf("trade_type must be BUY or SELL")
	}
	return nil
}

// paperShariahGateEnabled reports whether paper BUY orders require a certified Shariah PASS.
// Off unless PAPER_SHARIAH_GATE_ENABLED is true: the owner removed the restriction for paper trading (P1.9.24).
func paperShariahGateEnabled() bool {
	v, err := strconv.ParseBool(os.Getenv("PAPER_SHARIAH_GATE_ENABLED"))
	return err == nil && v
}

// userIDFrom returns the authenticated account, answering 401 when absent.
func userIDFrom(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	claims := middleware.GetUserFromContext(r.Context())
	if claims == nil {
		response.Unauthorized(w)
		return uuid.Nil, false
	}
	return claims.UserID, true
}

// ownedPortfolio resolves the caller's portfolio. If the route carries an {id}, it must be the caller's
// own; another account's id is indistinguishable from one that does not exist.
func ownedPortfolio(w http.ResponseWriter, r *http.Request, db *database.DB) (*database.PortfolioRecord, bool) {
	uid, ok := userIDFrom(w, r)
	if !ok {
		return nil, false
	}
	p, err := db.GetOrCreatePortfolioForUser(r.Context(), uid)
	if err != nil {
		log.Error().Err(err).Msg("portfolio lookup failed")
		response.Error(w, http.StatusInternalServerError, "failed to load portfolio")
		return nil, false
	}
	if raw := chi.URLParam(r, "id"); raw != "" {
		if id, err := strconv.Atoi(raw); err != nil || id != p.ID {
			response.NotFound(w, "portfolio")
			return nil, false
		}
	}
	return p, true
}

// executePaperTradeCore fills a paper order at the latest stored market price.
func executePaperTradeCore(ctx context.Context, db *database.DB, userID uuid.UUID, req ExecuteTradeRequest) (*database.PortfolioTrade, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	sym, err := db.GetSymbolByTicker(ctx, strings.TrimSpace(req.Symbol))
	if err != nil || sym == nil || !sym.IsActive {
		return nil, fmt.Errorf("symbol not found: %s", req.Symbol)
	}

	// Shariah status is always looked up and recorded on the trade. Whether it BLOCKS a paper BUY is the owner's
	// choice (PAPER_SHARIAH_GATE_ENABLED, default off for paper trading). Live/broker trading stays disabled either
	// way, and the prediction engine's own Shariah rule (FAIL is never a BUY candidate) is unaffected.
	shariahStatus := "UNEVALUATED"
	if sc, err := db.GetLatestShariahScreening(ctx, sym.ID); err == nil && sc != nil && sc.Status != "" {
		shariahStatus = string(sc.Status)
	}
	if req.TradeType == "BUY" && paperShariahGateEnabled() && shariahStatus != "PASS" {
		return nil, fmt.Errorf("Shariah compliance gate blocked: cannot BUY %s because screening status is %s (must be certified PASS)", sym.Symbol, shariahStatus)
	}

	p, err := db.GetOrCreatePortfolioForUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve paper portfolio: %w", err)
	}

	meta := database.TradeMeta{ShariahStatus: shariahStatus, PriceSource: database.PriceSourceLiveLatest}
	var fill float64
	if when, err := parseWhen(req.ExecuteAt); err != nil {
		return nil, err
	} else if when != nil && when.After(time.Now().Add(liveOrderWindow)) {
		return nil, fmt.Errorf("execute_at is in the future")
	} else if when != nil && time.Since(*when) > liveOrderWindow {
		// Replay: price as of the requested time, from stored history, with no lookahead.
		pp, err := db.PriceAt(ctx, sym.ID, *when)
		if err != nil {
			return nil, err
		}
		fill, meta.PriceSource, meta.PriceBarTime = pp.Price, pp.Source, pp.BarTime
		meta.ExecutedAt, meta.Backdated = *when, true
	} else {
		px, err := db.GetLatestPrice(ctx, sym.ID)
		if err != nil || px == nil || px.Close <= 0 {
			return nil, fmt.Errorf("market price currently unavailable for %s", sym.Symbol)
		}
		if f := market.GetFreshnessStatus(px.Timestamp, time.Now()); f != "FRESH" && f != "LAST_AVAILABLE" {
			return nil, fmt.Errorf("market price for %s is %s (last bar %s); paper fills require a current price",
				sym.Symbol, f, px.Timestamp.UTC().Format(time.RFC3339))
		}
		fill = px.Close
		bt := px.Timestamp
		meta.PriceBarTime = &bt
	}

	if req.TradeType == "BUY" {
		day := time.Now()
		if meta.Backdated {
			day = meta.ExecutedAt
		}
		cost := fill*float64(req.Shares) + database.CalculateDeliveryTransactionCosts("BUY", fill*float64(req.Shares)).TotalFees
		st, rerr := riskState(ctx, db, p, sym.ID, day, true, cost)
		if rerr != nil {
			return nil, fmt.Errorf("could not check the account's risk rules: %w", rerr)
		}
		if b := paper.Check(paper.LimitsFromEnv(), st); b != nil {
			return nil, fmt.Errorf("%s", b.Message)
		}
	}

	trade, err := db.ExecutePaperTradeAt(ctx, p.ID, sym.ID, req.TradeType, req.Shares, fill, meta)
	if err != nil {
		return nil, err
	}
	trade.Symbol = sym.Symbol
	return trade, nil
}

// portfolioView values a portfolio's positions at the latest stored prices.
// P&L here is unrealized versus cost basis; no "daily" figure is claimed.
func portfolioView(ctx context.Context, db *database.DB, p *database.PortfolioRecord) map[string]any {
	positions, err := db.GetPortfolioPositions(ctx, p.ID)
	if err != nil {
		positions = []database.PortfolioPosition{}
	}
	var invested, unrealized float64
	for i := range positions {
		positions[i].LastPrice = positions[i].AvgCostBasis
		if px, err := db.GetLatestPrice(ctx, positions[i].SymbolID); err == nil && px != nil && px.Close > 0 {
			positions[i].LastPrice = px.Close
			positions[i].UnrealizedPnL = (px.Close - positions[i].AvgCostBasis) * float64(positions[i].Shares)
		}
		invested += float64(positions[i].Shares) * positions[i].LastPrice
		unrealized += positions[i].UnrealizedPnL
	}
	pct := 0.0
	if p.InitialCapital > 0 {
		pct = unrealized / p.InitialCapital * 100
	}
	return map[string]any{
		"portfolio": map[string]any{
			"id": p.ID, "cash_balance": p.CashBalance, "initial_capital": p.InitialCapital,
			"invested_value": invested, "total_equity": p.CashBalance + invested,
			"unrealized_pnl": unrealized, "unrealized_pnl_pct": pct,
		},
		"positions": positions,
	}
}

// ---- Portfolio ----

func (h *PortfolioHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	response.JSON(w, http.StatusOK, portfolioView(r.Context(), h.db, p))
}

func (h *PortfolioHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) { h.GetPortfolio(w, r) }

func (h *PortfolioHandler) ListPositions(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	positions, err := h.db.GetPortfolioPositions(r.Context(), p.ID)
	if err != nil {
		log.Error().Err(err).Msg("list positions failed")
		response.Error(w, http.StatusInternalServerError, "failed to load positions")
		return
	}
	response.JSON(w, http.StatusOK, positions)
}

func (h *PortfolioHandler) ListTrades(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	trades, err := h.db.GetPortfolioTrades(r.Context(), p.ID)
	if err != nil {
		log.Error().Err(err).Msg("list trades failed")
		response.Error(w, http.StatusInternalServerError, "failed to load trades")
		return
	}
	response.JSON(w, http.StatusOK, trades)
}

func (h *PortfolioHandler) ExecuteTrade(w http.ResponseWriter, r *http.Request) {
	uid, ok := userIDFrom(w, r)
	if !ok {
		return
	}
	var req ExecuteTradeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	trade, err := executePaperTradeCore(r.Context(), h.db, uid, req)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	p, _ := h.db.GetOrCreatePortfolioForUser(r.Context(), uid)
	view := portfolioView(r.Context(), h.db, p)
	view["trade"] = trade
	response.JSON(w, http.StatusCreated, view)
}

// ---- Paper trading ----

func (h *PaperTradingHandler) session(r *http.Request, p *database.PortfolioRecord) map[string]any {
	frozen := map[string]any{"attested": false}
	if h.quantClient != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		if gov, err := h.quantClient.GetStrategyGovernance(ctx); err == nil {
			frozen = map[string]any{
				"attested": true, "version": "Strategy V2", "config_id": "STRATEGY_V2_CORE_PARAMETERS_V1",
				"canonical_sha256": gov.RuntimeHash, "matches_pinned": gov.MatchesPinned,
			}
		}
	}
	return map[string]any{
		"session_status": "ACTIVE", "portfolio_id": p.ID, "cash_balance": p.CashBalance,
		"initial_capital": p.InitialCapital, "paper_trading_enabled": true,
		"live_order_execution": false, "broker_trading_enabled": false, "frozen_strategy": frozen,
	}
}

func (h *PaperTradingHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	response.JSON(w, http.StatusOK, h.session(r, p))
}

func (h *PaperTradingHandler) GetAnalytics(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	out := h.session(r, p)
	for k, v := range portfolioView(r.Context(), h.db, p) {
		out[k] = v
	}
	response.JSON(w, http.StatusOK, out)
}

func (h *PaperTradingHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	trades, err := h.db.GetPortfolioTrades(r.Context(), p.ID)
	if err != nil {
		log.Error().Err(err).Msg("list orders failed")
		response.Error(w, http.StatusInternalServerError, "failed to load orders")
		return
	}
	response.JSON(w, http.StatusOK, trades)
}

func (h *PaperTradingHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db) // also enforces {id} ownership on /portfolios/{id}/orders
	if !ok {
		return
	}
	var req ExecuteTradeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	trade, err := executePaperTradeCore(r.Context(), h.db, p.OwnerID, req)
	if err != nil {
		response.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	response.JSON(w, http.StatusCreated, map[string]any{"status": "FILLED", "trade": trade})
}

func (h *PaperTradingHandler) ListPositions(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	positions, err := h.db.GetPortfolioPositions(r.Context(), p.ID)
	if err != nil {
		log.Error().Err(err).Msg("list positions failed")
		response.Error(w, http.StatusInternalServerError, "failed to load positions")
		return
	}
	response.JSON(w, http.StatusOK, positions)
}

func (h *PaperTradingHandler) GetPortfolio(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	response.JSON(w, http.StatusOK, p)
}

func (h *PaperTradingHandler) ListPortfolios(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	response.JSON(w, http.StatusOK, []*database.PortfolioRecord{p})
}

// CreatePortfolio is idempotent: each account has exactly one paper portfolio.
func (h *PaperTradingHandler) CreatePortfolio(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	response.JSON(w, http.StatusOK, p)
}

// ---- Market overview ----

// GetMarketOverview reports the session and the instrument universe. No index provider is wired up,
// so index levels are reported as unavailable instead of being invented.
func (h *StocksHandler) GetMarketOverview(w http.ResponseWriter, r *http.Request) {
	now := time.Now().In(market.Location)
	symbols, err := h.db.GetAllActiveSymbols(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("market overview failed")
		response.Error(w, http.StatusInternalServerError, "failed to retrieve market overview")
		return
	}
	counts := map[string]int{}
	for _, s := range symbols {
		if s.Sector != "" {
			counts[s.Sector]++
		}
	}
	sectors := make([]map[string]any, 0, len(counts))
	for sec, n := range counts {
		sectors = append(sectors, map[string]any{"sector": sec, "count": n})
	}
	sort.Slice(sectors, func(i, j int) bool {
		if sectors[i]["count"].(int) != sectors[j]["count"].(int) {
			return sectors[i]["count"].(int) > sectors[j]["count"].(int)
		}
		return sectors[i]["sector"].(string) < sectors[j]["sector"].(string)
	})

	session := string(market.GetCurrentSession(now))
	// Index levels come from NSE's own end-of-day index files (loaded by the worker), never from a guess.
	names := []struct{ key, label string }{
		{database.IndexNifty50, "NIFTY 50"}, {database.IndexNifty500, "NIFTY 500"}, {"Nifty Bank", "NIFTY Bank"}, {"Nifty IT", "NIFTY IT"},
	}
	keys := make([]string, 0, len(names))
	for _, n := range names {
		keys = append(keys, n.key)
	}
	snaps, _ := h.db.LatestIndexSnapshots(r.Context(), keys)
	byName := map[string]database.IndexSnapshot{}
	for _, sn := range snaps {
		byName[sn.Name] = sn
	}
	indices := make([]map[string]any, 0, len(names))
	loaded := 0
	for _, n := range names {
		item := map[string]any{"symbol": n.key, "name": n.label, "value": nil, "change": nil, "change_percent": nil, "status": "DATA_UNAVAILABLE"}
		if sn, ok := byName[n.key]; ok {
			loaded++
			item["value"], item["change_percent"], item["status"] = sn.Close, sn.ChangePercent, "END_OF_DAY"
			item["as_of"], item["pe"], item["pb"], item["div_yield"] = sn.Date.Format("2006-01-02"), sn.PE, sn.PB, sn.DivYield
			if sn.PrevClose != nil {
				item["change"] = sn.Close - *sn.PrevClose
			}
		}
		indices = append(indices, item)
	}
	note := "Index levels are NSE's official end-of-day closes (not live)."
	if loaded == 0 {
		note = "Index history has not been loaded yet; levels are intentionally not shown until it is."
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"market_status": session, "timestamp": now.Format(time.RFC3339),
		"indices":               indices,
		"indices_note":          note,
		"total_active_equities": len(symbols), "sectors": sectors,
	})
}
