package autopaper

import (
	"context"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/plans"
)

// DBStore is the PostgreSQL-backed Store.
type DBStore struct{ DB *database.DB }

func (s DBStore) Signals(ctx context.Context) ([]database.AutoSignal, error) {
	return s.DB.AutoSignals(ctx)
}
func (s DBStore) Sessions(ctx context.Context, after string) ([]string, error) {
	return s.DB.ExchangeSessions(ctx, after)
}
func (s DBStore) Bars(ctx context.Context, symbolID int, after, upTo string) ([]plans.Bar, error) {
	rows, err := s.DB.ExchangeBarsBetween(ctx, symbolID, after, upTo)
	if err != nil {
		return nil, err
	}
	out := make([]plans.Bar, len(rows))
	for i, b := range rows {
		out[i] = plans.Bar{Date: b.Date, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close}
	}
	return out, nil
}
func (s DBStore) UptrendAt(ctx context.Context, day string) (bool, bool, error) {
	return s.DB.NiftyUptrendAt(ctx, day)
}
func (s DBStore) Actions(ctx context.Context, pid int) ([]database.AutoAction, error) {
	return s.DB.AutoActions(ctx, pid)
}
func (s DBStore) SaveAction(ctx context.Context, a database.AutoAction) (int64, bool, error) {
	return s.DB.SaveAutoAction(ctx, a)
}
func (s DBStore) SetActionTrade(ctx context.Context, id int64, tradeID int) error {
	return s.DB.SetAutoActionTrade(ctx, id, tradeID)
}
func (s DBStore) FindTrade(ctx context.Context, pid, symbolID int, side string, at time.Time) (int, error) {
	return s.DB.FindTradeAt(ctx, pid, symbolID, side, at)
}
func (s DBStore) Cash(ctx context.Context, pid int) (float64, error) {
	return s.DB.PortfolioCash(ctx, pid)
}
func (s DBStore) Cursor(ctx context.Context, pid int) (string, error) {
	return s.DB.AutoCursor(ctx, pid)
}
func (s DBStore) SetCursor(ctx context.Context, pid int, day string) error {
	return s.DB.SetAutoCursor(ctx, pid, day)
}

// PriceSourcePlanFill labels fills made at an exchange daily-bar level by a plan, not at a live quote.
const PriceSourcePlanFill = "PLAN_FILL_EXCHANGE_DAILY"

// Fill records a ledger trade at its simulated time. The ledger enforces cash and no-short-selling; fees are the ledger's real delivery charges.
func (s DBStore) Fill(ctx context.Context, pid, symbolID int, side string, shares int, price float64, at time.Time, barDate string) (int, error) {
	shariah := "UNEVALUATED"
	if sc, err := s.DB.GetLatestShariahScreening(ctx, symbolID); err == nil && sc != nil && sc.Status != "" {
		shariah = string(sc.Status)
	}
	bar, _ := time.ParseInLocation("2006-01-02", barDate, ist)
	tr, err := s.DB.ExecutePaperTradeAt(ctx, pid, symbolID, side, shares, price, database.TradeMeta{
		ShariahStatus: shariah, ExecutedAt: at, PriceSource: PriceSourcePlanFill, PriceBarTime: &bar, Backdated: true})
	if err != nil {
		return 0, err
	}
	return tr.ID, nil
}
