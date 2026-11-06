package database

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/halal-equity/backend/internal/services/market"
)

// PortfolioRecord represents a user's paper trading portfolio.
type PortfolioRecord struct {
	ID             int       `json:"id"`
	OwnerID        uuid.UUID `json:"owner_id"`
	CashBalance    float64   `json:"cash_balance"`
	InitialCapital float64   `json:"initial_capital"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// PortfolioPosition represents a currently held stock position.
type PortfolioPosition struct {
	ID            int       `json:"id"`
	PortfolioID   int       `json:"portfolio_id"`
	SymbolID      int       `json:"symbol_id"`
	Symbol        string    `json:"symbol,omitempty"` // Joined from symbols table
	Shares        int       `json:"shares"`
	AvgCostBasis  float64   `json:"avg_cost_basis"`
	LastPrice     float64   `json:"last_price,omitempty"` // Joined from latest OHLCV or live feed
	UnrealizedPnL float64   `json:"unrealized_pnl,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PortfolioTrade represents a single transaction (BUY or SELL).
type PortfolioTrade struct {
	ID          int      `json:"id"`
	PortfolioID int      `json:"portfolio_id"`
	SymbolID    int      `json:"symbol_id"`
	Symbol      string   `json:"symbol,omitempty"` // Joined from symbols table
	TradeType   string   `json:"trade_type"`       // BUY or SELL
	Shares      int      `json:"shares"`
	Price       float64  `json:"price"`
	FeesPaid    float64  `json:"fees_paid"`
	RealizedPnL *float64 `json:"realized_pnl"` // Only applicable for SELL
	// Shariah screening status of the instrument when the paper trade executed (nil for trades before P1.9.24).
	ShariahStatusAtTrade *string `json:"shariah_status_at_trade"`
	// Where the fill price came from and whether the order was executed "as of" a past time (P1.9.25).
	PriceSource  *string    `json:"price_source"`
	PriceBarTime *time.Time `json:"price_bar_time"`
	IsBackdated  bool       `json:"is_backdated"`
	TradeDate    time.Time  `json:"trade_date"`
}

// DefaultPaperCapital is the starting virtual balance of a new paper portfolio.
const DefaultPaperCapital = 1000000

// GetOrCreatePortfolioForUser returns the account's single paper portfolio, creating it on first use.
func (db *DB) GetOrCreatePortfolioForUser(ctx context.Context, userID uuid.UUID) (*PortfolioRecord, error) {
	var p PortfolioRecord
	err := db.QueryRowContext(ctx, `
		INSERT INTO portfolios (owner_id, cash_balance, initial_capital)
		VALUES ($1, $2, $2)
		ON CONFLICT (owner_id) WHERE owner_id IS NOT NULL DO UPDATE SET owner_id = EXCLUDED.owner_id
		RETURNING id, owner_id, cash_balance, initial_capital, created_at, updated_at
	`, userID, DefaultPaperCapital).Scan(&p.ID, &p.OwnerID, &p.CashBalance, &p.InitialCapital, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get/create portfolio failed: %w", err)
	}
	return &p, nil
}

// GetPortfolioPositions retrieves all active holdings for a portfolio.
func (db *DB) GetPortfolioPositions(ctx context.Context, portfolioID int) ([]PortfolioPosition, error) {
	// Join with symbols table to get the readable ticker
	query := `
		SELECT p.id, p.portfolio_id, p.symbol_id, s.symbol, p.shares, p.avg_cost_basis, p.updated_at
		FROM portfolio_positions p
		JOIN symbols s ON p.symbol_id = s.id
		WHERE p.portfolio_id = $1 AND p.shares > 0
	`
	rows, err := db.QueryContext(ctx, query, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("query positions failed: %w", err)
	}
	defer rows.Close()

	positions := make([]PortfolioPosition, 0)
	for rows.Next() {
		var pos PortfolioPosition
		if err := rows.Scan(
			&pos.ID, &pos.PortfolioID, &pos.SymbolID, &pos.Symbol,
			&pos.Shares, &pos.AvgCostBasis, &pos.UpdatedAt,
		); err != nil {
			return nil, err
		}
		positions = append(positions, pos)
	}
	return positions, rows.Err()
}

// GetPortfolioTrades retrieves the trade execution history for a portfolio.
func (db *DB) GetPortfolioTrades(ctx context.Context, portfolioID int) ([]PortfolioTrade, error) {
	query := `
		SELECT t.id, t.portfolio_id, t.symbol_id, s.symbol, t.trade_type, t.shares, t.price, t.fees_paid, t.realized_pnl, t.trade_date,
		       t.shariah_status_at_trade, t.price_source, t.price_bar_time, t.is_backdated
		FROM portfolio_trades t
		JOIN symbols s ON t.symbol_id = s.id
		WHERE t.portfolio_id = $1
		ORDER BY t.trade_date DESC
		LIMIT 100
	`
	rows, err := db.QueryContext(ctx, query, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("query trades failed: %w", err)
	}
	defer rows.Close()

	trades := make([]PortfolioTrade, 0)
	for rows.Next() {
		var t PortfolioTrade
		if err := rows.Scan(
			&t.ID, &t.PortfolioID, &t.SymbolID, &t.Symbol,
			&t.TradeType, &t.Shares, &t.Price, &t.FeesPaid, &t.RealizedPnL, &t.TradeDate,
			&t.ShariahStatusAtTrade, &t.PriceSource, &t.PriceBarTime, &t.IsBackdated,
		); err != nil {
			return nil, err
		}
		trades = append(trades, t)
	}
	return trades, rows.Err()
}

// ExecutePaperTrade executes a paper trade without recording a Shariah status (kept for existing callers).
func (db *DB) ExecutePaperTrade(ctx context.Context, portfolioID, symbolID int, tradeType string, shares int, price float64) (*PortfolioTrade, error) {
	return db.ExecutePaperTradeWithShariah(ctx, portfolioID, symbolID, tradeType, shares, price, "")
}

// TradeMeta carries what a paper trade records beyond its economics.
type TradeMeta struct {
	ShariahStatus string
	ExecutedAt    time.Time  // simulated execution time; zero means "now"
	PriceSource   string     // see PriceSource* constants
	PriceBarTime  *time.Time // bar the price was taken from, if any
	Backdated     bool
}

// LedgerRow is one trade as the replay sees it.
type LedgerRow struct {
	ID, SymbolID int
	Symbol       string
	Type         string
	Shares       int
	Price, Fees  float64
	Date         time.Time
}

// LedgerPosition is a holding after a replay.
type LedgerPosition struct {
	Shares  int
	AvgCost float64
}

// ReplayLedger rebuilds cash, holdings and realised P&L from the starting capital, oldest trade first. It fails if any
// trade could not have happened at its date: not enough cash at that moment, or selling shares not yet held.
func ReplayLedger(rows []LedgerRow, initialCapital float64) (cash float64, positions map[int]LedgerPosition, realized map[int]float64, err error) {
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Date.Before(rows[j].Date) })
	cash = initialCapital
	positions, realized = map[int]LedgerPosition{}, map[int]float64{}
	for _, r := range rows {
		gross, pos := float64(r.Shares)*r.Price, positions[r.SymbolID]
		when := r.Date.In(market.Location).Format("2006-01-02 15:04")
		if r.Type == "BUY" {
			total := gross + r.Fees
			if total > cash+1e-6 {
				return 0, nil, nil, fmt.Errorf("that would leave the account short of cash on %s: buying %d %s needs %.2f but only %.2f would be available", when, r.Shares, r.Symbol, total, cash)
			}
			cash -= total
			pos.AvgCost = (float64(pos.Shares)*pos.AvgCost + total) / float64(pos.Shares+r.Shares)
			pos.Shares += r.Shares
		} else {
			if r.Shares > pos.Shares {
				return 0, nil, nil, fmt.Errorf("that would mean selling %d %s on %s while holding only %d at that time (short selling is prohibited)", r.Shares, r.Symbol, when, pos.Shares)
			}
			net := gross - r.Fees
			cash += net
			realized[r.ID] = net - float64(r.Shares)*pos.AvgCost
			pos.Shares -= r.Shares
		}
		positions[r.SymbolID] = pos
	}
	return cash, positions, realized, nil
}

// insertAndReplay records a backdated trade and rebuilds the ledger around it, inside the caller's transaction, so a
// trade that would make history impossible leaves everything untouched.
func insertAndReplay(ctx context.Context, tx *sql.Tx, portfolioID, symbolID int, tradeType string, shares int, price float64, at time.Time, meta TradeMeta) (*PortfolioTrade, error) {
	var initial float64
	if err := tx.QueryRowContext(ctx, `SELECT initial_capital FROM portfolios WHERE id = $1`, portfolioID).Scan(&initial); err != nil {
		return nil, fmt.Errorf("failed to read starting capital: %w", err)
	}
	fees := CalculateDeliveryTransactionCosts(tradeType, float64(shares)*price).TotalFees
	var newID int
	err := tx.QueryRowContext(ctx, `
		INSERT INTO portfolio_trades (portfolio_id, symbol_id, trade_type, shares, price, fees_paid, shariah_status_at_trade, trade_date, price_source, price_bar_time, is_backdated)
		VALUES ($1, $2, $3, $4, $5, $6, NULLIF($7, ''), $8, NULLIF($9, ''), $10, $11) RETURNING id`,
		portfolioID, symbolID, tradeType, shares, price, fees, meta.ShariahStatus, at, meta.PriceSource, meta.PriceBarTime, meta.Backdated).Scan(&newID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert trade log: %w", err)
	}
	rows, err := tx.QueryContext(ctx, `SELECT t.id, t.symbol_id, s.symbol, t.trade_type, t.shares, t.price::float8, t.fees_paid::float8, t.trade_date
		FROM portfolio_trades t JOIN symbols s ON s.id = t.symbol_id WHERE t.portfolio_id = $1 ORDER BY t.trade_date, t.id`, portfolioID)
	if err != nil {
		return nil, err
	}
	var ledger []LedgerRow
	for rows.Next() {
		var l LedgerRow
		if err := rows.Scan(&l.ID, &l.SymbolID, &l.Symbol, &l.Type, &l.Shares, &l.Price, &l.Fees, &l.Date); err != nil {
			rows.Close()
			return nil, err
		}
		ledger = append(ledger, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cash, positions, realized, err := ReplayLedger(ledger, initial)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE portfolios SET cash_balance = $1, updated_at = NOW() WHERE id = $2`, cash, portfolioID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id = $1`, portfolioID); err != nil {
		return nil, err
	}
	for sym, pos := range positions {
		if pos.Shares == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO portfolio_positions (portfolio_id, symbol_id, shares, avg_cost_basis) VALUES ($1, $2, $3, $4)`, portfolioID, sym, pos.Shares, pos.AvgCost); err != nil {
			return nil, err
		}
	}
	for id, pnl := range realized {
		if _, err := tx.ExecContext(ctx, `UPDATE portfolio_trades SET realized_pnl = $1 WHERE id = $2`, pnl, id); err != nil {
			return nil, err
		}
	}
	tr := &PortfolioTrade{ID: newID, PortfolioID: portfolioID, SymbolID: symbolID, TradeType: tradeType, Shares: shares, Price: price, FeesPaid: fees,
		TradeDate: at, IsBackdated: meta.Backdated, PriceBarTime: meta.PriceBarTime}
	if v, ok := realized[newID]; ok {
		tr.RealizedPnL = &v
	}
	if meta.PriceSource != "" {
		ps := meta.PriceSource
		tr.PriceSource = &ps
	}
	if meta.ShariahStatus != "" {
		ss := meta.ShariahStatus
		tr.ShariahStatusAtTrade = &ss
	}
	return tr, nil
}

// ExecutePaperTradeWithShariah performs a BUY or SELL transaction safely, updating balances and positions.
// Enforces Shariah spot rules: No short selling, no margin.
func (db *DB) ExecutePaperTradeWithShariah(ctx context.Context, portfolioID, symbolID int, tradeType string, shares int, price float64, shariahStatus string) (*PortfolioTrade, error) {
	return db.ExecutePaperTradeAt(ctx, portfolioID, symbolID, tradeType, shares, price, TradeMeta{ShariahStatus: shariahStatus, PriceSource: PriceSourceLiveLatest})
}

// ExecutePaperTradeAt is the full-form executor. With a past ExecutedAt the order is recorded at that time. If it falls
// before existing trades the ledger is replayed around it, and it is refused only when history would become impossible.
func (db *DB) ExecutePaperTradeAt(ctx context.Context, portfolioID, symbolID int, tradeType string, shares int, price float64, meta TradeMeta) (*PortfolioTrade, error) {
	shariahStatus := meta.ShariahStatus
	executedAt := meta.ExecutedAt
	if executedAt.IsZero() {
		executedAt = time.Now()
	}
	if shares <= 0 {
		return nil, fmt.Errorf("shares must be positive")
	}
	if tradeType != "BUY" && tradeType != "SELL" {
		return nil, fmt.Errorf("invalid trade type: %s", tradeType)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Get Portfolio Balance
	var cashBalance float64
	err = tx.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1 FOR UPDATE`, portfolioID).Scan(&cashBalance)
	if err != nil {
		return nil, fmt.Errorf("failed to lock portfolio: %w", err)
	}

	// 1b. A backdated order goes into its place in history and the ledger is replayed from the start.
	var lastTrade sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT max(trade_date) FROM portfolio_trades WHERE portfolio_id = $1`, portfolioID).Scan(&lastTrade); err != nil {
		return nil, fmt.Errorf("failed to read ledger order: %w", err)
	}
	if lastTrade.Valid && executedAt.Before(lastTrade.Time) {
		tr, err := insertAndReplay(ctx, tx, portfolioID, symbolID, tradeType, shares, price, executedAt, meta)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("transaction commit failed: %w", err)
		}
		return tr, nil
	}

	// 2. Get Current Position
	var posID, currentShares int
	var avgCost float64
	err = tx.QueryRowContext(ctx, `
		SELECT id, shares, avg_cost_basis 
		FROM portfolio_positions 
		WHERE portfolio_id = $1 AND symbol_id = $2 FOR UPDATE`,
		portfolioID, symbolID).Scan(&posID, &currentShares, &avgCost)

	positionExists := err == nil

	// Statutory & exchange transaction fees (INDIA_EQ_DELIVERY_FEES_2026Q4_V2)
	grossValue := float64(shares) * price
	costBreakdown := CalculateDeliveryTransactionCosts(tradeType, grossValue)
	fees := costBreakdown.TotalFees

	var realizedPnL *float64

	if tradeType == "BUY" {
		totalCost := grossValue + fees
		if totalCost > cashBalance {
			return nil, fmt.Errorf("insufficient funds: cost %.2f, balance %.2f", totalCost, cashBalance)
		}

		// Update Cash
		cashBalance -= totalCost

		// Calculate new average cost basis
		var newShares int
		var newAvgCost float64
		if positionExists {
			newShares = currentShares + shares
			totalHistoricalCost := float64(currentShares) * avgCost
			newAvgCost = (totalHistoricalCost + totalCost) / float64(newShares)
		} else {
			newShares = shares
			newAvgCost = totalCost / float64(shares)
		}

		// Upsert Position
		if positionExists {
			_, err = tx.ExecContext(ctx, `UPDATE portfolio_positions SET shares = $1, avg_cost_basis = $2, updated_at = NOW() WHERE id = $3`, newShares, newAvgCost, posID)
		} else {
			_, err = tx.ExecContext(ctx, `INSERT INTO portfolio_positions (portfolio_id, symbol_id, shares, avg_cost_basis) VALUES ($1, $2, $3, $4)`, portfolioID, symbolID, newShares, newAvgCost)
		}

	} else if tradeType == "SELL" {
		if !positionExists || currentShares < shares {
			return nil, fmt.Errorf("insufficient shares: attempting to sell %d but only own %d (Short selling is strictly prohibited)", shares, currentShares)
		}

		netProceeds := grossValue - fees
		cashBalance += netProceeds

		// Calculate Realized PnL
		costOfSharesSold := float64(shares) * avgCost
		rPnL := netProceeds - costOfSharesSold
		realizedPnL = &rPnL

		newShares := currentShares - shares

		// Update Position
		_, err = tx.ExecContext(ctx, `UPDATE portfolio_positions SET shares = $1, updated_at = NOW() WHERE id = $2`, newShares, posID)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to update position: %w", err)
	}

	// 3. Update Portfolio Cash
	_, err = tx.ExecContext(ctx, `UPDATE portfolios SET cash_balance = $1, updated_at = NOW() WHERE id = $2`, cashBalance, portfolioID)
	if err != nil {
		return nil, fmt.Errorf("failed to update portfolio cash: %w", err)
	}

	// 4. Record the Trade
	var tradeID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO portfolio_trades (portfolio_id, symbol_id, trade_type, shares, price, fees_paid, realized_pnl, shariah_status_at_trade,
			trade_date, price_source, price_bar_time, is_backdated)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), $9, NULLIF($10, ''), $11, $12) RETURNING id`,
		portfolioID, symbolID, tradeType, shares, price, fees, realizedPnL, shariahStatus,
		executedAt, meta.PriceSource, meta.PriceBarTime, meta.Backdated,
	).Scan(&tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert trade log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	return &PortfolioTrade{
		ID:           tradeID,
		PortfolioID:  portfolioID,
		SymbolID:     symbolID,
		TradeType:    tradeType,
		Shares:       shares,
		Price:        price,
		FeesPaid:     fees,
		RealizedPnL:  realizedPnL,
		TradeDate:    executedAt,
		IsBackdated:  meta.Backdated,
		PriceBarTime: meta.PriceBarTime,
		PriceSource: func() *string {
			if meta.PriceSource == "" {
				return nil
			}
			return &meta.PriceSource
		}(),
		ShariahStatusAtTrade: func() *string {
			if shariahStatus == "" {
				return nil
			}
			return &shariahStatus
		}(),
	}, nil
}
