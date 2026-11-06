package handlers

import (
	"net/http"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/pkg/response"
)

type perfTrade struct {
	PnL                float64  `json:"pnl"`
	Invested           float64  `json:"invested"`
	Fees               float64  `json:"fees"`
	HoldingDays        *float64 `json:"holding_days,omitempty"`
	BenchmarkReturnPct *float64 `json:"benchmark_return_pct,omitempty"`
}

// closedTrades turns the ledger into one record per SELL: its net P&L, the cost of the shares sold (the ledger's
// average cost already includes buy charges), the sell-side charges, the holding period (from the share-weighted
// average entry time) and the NIFTY 50 return over the same dates when index history is stored.
func (h *PaperTradingHandler) closedTrades(r *http.Request, portfolioID int) ([]perfTrade, error) {
	trades, err := h.db.GetPortfolioTrades(r.Context(), portfolioID)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(trades, func(i, j int) bool { return trades[i].TradeDate.Before(trades[j].TradeDate) })
	type lot struct {
		shares float64
		sumT   float64 // share-weighted sum of entry unix times
	}
	open := map[int]*lot{}
	var out []perfTrade
	for _, t := range trades {
		l := open[t.SymbolID]
		if l == nil {
			l = &lot{}
			open[t.SymbolID] = l
		}
		qty := float64(t.Shares)
		switch t.TradeType {
		case "BUY":
			l.shares += qty
			l.sumT += qty * float64(t.TradeDate.Unix())
		case "SELL":
			if t.RealizedPnL == nil || l.shares <= 0 {
				continue
			}
			netProceeds := qty*t.Price - t.FeesPaid
			invested := netProceeds - *t.RealizedPnL
			pt := perfTrade{PnL: *t.RealizedPnL, Invested: invested, Fees: t.FeesPaid}
			entry := time.Unix(int64(l.sumT/l.shares), 0)
			days := t.TradeDate.Sub(entry).Hours() / 24
			pt.HoldingDays = &days
			if a, okA, _ := h.db.IndexCloseOnOrBefore(r.Context(), database.IndexNifty50, entry.In(market.Location)); okA {
				if b, okB, _ := h.db.IndexCloseOnOrBefore(r.Context(), database.IndexNifty50, t.TradeDate.In(market.Location)); okB && b.Date.After(a.Date) {
					v := (b.Close/a.Close - 1) * 100
					pt.BenchmarkReturnPct = &v
				}
			}
			out = append(out, pt)
			frac := qty / l.shares
			l.sumT -= l.sumT * frac
			l.shares -= qty
		}
	}
	return out, nil
}

// Performance reports bootstrap-backed statistics over the caller's closed paper trades.
func (h *PaperTradingHandler) Performance(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	if h.quantClient == nil {
		response.Error(w, http.StatusServiceUnavailable, "the analytics engine is not available")
		return
	}
	trades, err := h.closedTrades(r, p.ID)
	if err != nil {
		log.Error().Err(err).Msg("performance: ledger read failed")
		response.Error(w, http.StatusInternalServerError, "failed to read the ledger")
		return
	}
	if trades == nil {
		trades = []perfTrade{}
	}
	rep, err := h.quantClient.PerformanceReport(r.Context(), map[string]any{"trades": trades, "initial_capital": p.InitialCapital})
	if err != nil {
		log.Error().Err(err).Msg("performance: engine call failed")
		response.Error(w, http.StatusBadGateway, "the analytics engine could not compute the report")
		return
	}
	response.JSON(w, http.StatusOK, rep)
}
