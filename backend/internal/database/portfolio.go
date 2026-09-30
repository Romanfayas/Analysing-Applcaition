package database

import (
	"context"
	"fmt"
	"time"
)

// PortfolioRecord represents a user's paper trading portfolio.
type PortfolioRecord struct {
	ID              int       `json:"id"`
	UserID          int       `json:"user_id"` // 1 for single-user MVP
	CashBalance     float64   `json:"cash_balance"`
	InitialCapital  float64   `json:"initial_capital"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
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
	ID            int       `json:"id"`
	PortfolioID   int       `json:"portfolio_id"`
	SymbolID      int       `json:"symbol_id"`
	TradeType     string    `json:"trade_type"` // BUY or SELL
	Shares        int       `json:"shares"`
	Price         float64   `json:"price"`
	FeesPaid      float64   `json:"fees_paid"`
	RealizedPnL   *float64  `json:"realized_pnl"` // Only applicable for SELL
	TradeDate     time.Time `json:"trade_date"`
}

// GetOrCreatePortfolio fetches the MVP portfolio (user_id = 1) or creates it with 1,000,000 capital.
func (db *DB) GetOrCreatePortfolio(ctx context.Context, userID int) (*PortfolioRecord, error) {
	query := `
		INSERT INTO portfolios (user_id, cash_balance, initial_capital)
		VALUES ($1, 1000000, 1000000)
		ON CONFLICT (user_id) DO UPDATE SET updated_at = NOW()
		RETURNING id, user_id, cash_balance, initial_capital, created_at, updated_at
	`
	var p PortfolioRecord
	err := db.QueryRowContext(ctx, query, userID).Scan(
		&p.ID, &p.UserID, &p.CashBalance, &p.InitialCapital, &p.CreatedAt, &p.UpdatedAt,
	)
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

	var positions []PortfolioPosition
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

// ExecutePaperTrade performs a BUY or SELL transaction safely, updating balances and positions.
// Enforces Shariah spot rules: No short selling, no margin.
func (db *DB) ExecutePaperTrade(ctx context.Context, portfolioID, symbolID int, tradeType string, shares int, price float64) (*PortfolioTrade, error) {
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

	// 2. Get Current Position
	var posID, currentShares int
	var avgCost float64
	err = tx.QueryRowContext(ctx, `
		SELECT id, shares, avg_cost_basis 
		FROM portfolio_positions 
		WHERE portfolio_id = $1 AND symbol_id = $2 FOR UPDATE`,
		portfolioID, symbolID).Scan(&posID, &currentShares, &avgCost)
	
	positionExists := err == nil

	// STT + Brokerage ~ 0.1% for realistic paper trading
	feeRate := 0.001
	grossValue := float64(shares) * price
	fees := grossValue * feeRate

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
		INSERT INTO portfolio_trades (portfolio_id, symbol_id, trade_type, shares, price, fees_paid, realized_pnl)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		portfolioID, symbolID, tradeType, shares, price, fees, realizedPnL,
	).Scan(&tradeID)
	if err != nil {
		return nil, fmt.Errorf("failed to insert trade log: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("transaction commit failed: %w", err)
	}

	return &PortfolioTrade{
		ID:          tradeID,
		PortfolioID: portfolioID,
		SymbolID:    symbolID,
		TradeType:   tradeType,
		Shares:      shares,
		Price:       price,
		FeesPaid:    fees,
		RealizedPnL: realizedPnL,
		TradeDate:   time.Now(),
	}, nil
}
