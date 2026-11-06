package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/paper"
	"github.com/halal-equity/backend/pkg/response"
)

// riskState gathers what the paper risk rules need for an order on `day` (IST) in the given stock.
func riskState(ctx context.Context, db *database.DB, p *database.PortfolioRecord, symbolID int, day time.Time, isBuy bool, orderCost float64) (paper.State, error) {
	positions, err := db.GetPortfolioPositions(ctx, p.ID)
	if err != nil {
		return paper.State{}, err
	}
	trades, err := db.GetPortfolioTrades(ctx, p.ID)
	if err != nil {
		return paper.State{}, err
	}
	s := paper.State{IsBuy: isBuy, OrderCost: orderCost, InitialCapital: p.InitialCapital, BookEquity: p.CashBalance}
	for _, pos := range positions {
		cost := float64(pos.Shares) * pos.AvgCostBasis
		s.BookEquity += cost
		if pos.SymbolID == symbolID {
			s.SymbolCost = cost
		}
	}
	d := day.In(market.Location).Format("2006-01-02")
	for _, t := range trades {
		if t.TradeDate.In(market.Location).Format("2006-01-02") != d {
			continue
		}
		s.OrdersToday++
		if t.RealizedPnL != nil {
			s.RealizedToday += *t.RealizedPnL
		}
	}
	return s, nil
}

// RiskRules reports the account's risk limits and how much of each is used today.
func (h *PaperTradingHandler) RiskRules(w http.ResponseWriter, r *http.Request) {
	p, ok := ownedPortfolio(w, r, h.db)
	if !ok {
		return
	}
	s, err := riskState(r.Context(), h.db, p, 0, time.Now(), true, 0)
	if err != nil {
		response.Error(w, http.StatusInternalServerError, "failed to read the ledger")
		return
	}
	response.JSON(w, http.StatusOK, map[string]any{
		"limits": paper.LimitsFromEnv(), "orders_today": s.OrdersToday, "realized_today": s.RealizedToday,
		"book_equity": s.BookEquity, "note": "Sells are never blocked by these rules.",
	})
}
