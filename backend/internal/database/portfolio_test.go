package database_test

import (
	"context"
	"sync"
	"testing"

	"github.com/halal-equity/backend/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeliveryFeeCalculationBreakdown verifies the statutory fee schedule (INDIA_EQ_DELIVERY_FEES_2026Q4_V2).
func TestDeliveryFeeCalculationBreakdown(t *testing.T) {
	// 1. BUY Leg on ₹100,000 turnover:
	// - STT (0.1%): ₹100.00
	// - Stamp Duty (0.015%): ₹15.00
	// - Exchange Charges (0.00297%): ₹2.97
	// - SEBI Charges (0.0001%): ₹0.10
	// - Brokerage: ₹0.00
	// - DP Charges: ₹0.00
	// - GST (18% on Exchange + SEBI): 18% of (2.97 + 0.10) = 0.5526 -> ₹0.55
	// - Total: 100.00 + 15.00 + 2.97 + 0.10 + 0.55 = ₹118.62 (11.862 bps)
	buyCosts := database.CalculateDeliveryTransactionCosts("BUY", 100000.00)
	assert.Equal(t, 100000.00, buyCosts.Turnover)
	assert.Equal(t, 100.00, buyCosts.STT)
	assert.Equal(t, 15.00, buyCosts.StampDuty)
	assert.Equal(t, 2.97, buyCosts.ExchangeCharges)
	assert.Equal(t, 0.10, buyCosts.SEBICharges)
	assert.Equal(t, 0.00, buyCosts.Brokerage)
	assert.Equal(t, 0.00, buyCosts.DPCharges)
	assert.Equal(t, 0.55, buyCosts.GST)
	assert.Equal(t, 118.62, buyCosts.TotalFees)
	assert.Equal(t, database.FeeModelVersion, buyCosts.FeeModelVersion)

	// 2. SELL Leg on ₹100,000 turnover:
	// - STT (0.1%): ₹100.00
	// - Stamp Duty: ₹0.00 (exempt on sell)
	// - Exchange Charges (0.00297%): ₹2.97
	// - SEBI Charges (0.0001%): ₹0.10
	// - Brokerage: ₹0.00
	// - DP Charges: ₹13.50
	// - GST (18% on Exchange + SEBI + DP): 18% of (2.97 + 0.10 + 13.50) = 18% of 16.57 = 2.9826 -> ₹2.98
	// - Total: 100.00 + 0.00 + 2.97 + 0.10 + 13.50 + 2.98 = ₹119.55 (11.955 bps)
	sellCosts := database.CalculateDeliveryTransactionCosts("SELL", 100000.00)
	assert.Equal(t, 100000.00, sellCosts.Turnover)
	assert.Equal(t, 100.00, sellCosts.STT)
	assert.Equal(t, 0.00, sellCosts.StampDuty)
	assert.Equal(t, 2.97, sellCosts.ExchangeCharges)
	assert.Equal(t, 0.10, sellCosts.SEBICharges)
	assert.Equal(t, 0.00, sellCosts.Brokerage)
	assert.Equal(t, 13.50, sellCosts.DPCharges)
	assert.Equal(t, 2.98, sellCosts.GST)
	assert.Equal(t, 119.55, sellCosts.TotalFees)
}

// TestPaperTradingLedgerAccountingAndFriction tests BUY, partial SELL, full SELL,
// cost basis, realized P&L, cash balance, and failure atomicity.
func TestPaperTradingLedgerAccountingAndFriction(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var testPortfolioID int
	initialCash := 1000000.00
	err := db.QueryRowContext(ctx, `
		INSERT INTO portfolios (user_id, cash_balance, initial_capital)
		VALUES (99999, $1, $1)
		RETURNING id
	`, initialCash).Scan(&testPortfolioID)
	require.NoError(t, err)

	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolios WHERE id = $1`, testPortfolioID)
	}()

	var tcsSymbolID int
	err = db.QueryRowContext(ctx, `SELECT id FROM symbols WHERE symbol = 'TCS'`).Scan(&tcsSymbolID)
	require.NoError(t, err)

	// 1. Rejected Order Invariant: Short Selling Rejection leaves ledger and cash completely unchanged
	initialTradeCount := 0
	err = db.QueryRowContext(ctx, `SELECT count(*) FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID).Scan(&initialTradeCount)
	require.NoError(t, err)

	_, err = db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "SELL", 10, 2500.0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Short selling is strictly prohibited")

	var afterRejectionTrades int
	var afterRejectionCash float64
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID).Scan(&afterRejectionTrades)
	_ = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&afterRejectionCash)
	assert.Equal(t, initialTradeCount, afterRejectionTrades, "Rejected order must write zero trade ledger records")
	assert.Equal(t, initialCash, afterRejectionCash, "Rejected order must leave cash balance unchanged")

	// 2. Rejected Order: Insufficient Cash leaves ledger unchanged
	_, err = db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "BUY", 1000, 2500.0) // Requires ₹25 Lakhs > ₹10 Lakhs
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient funds")

	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID).Scan(&afterRejectionTrades)
	_ = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&afterRejectionCash)
	assert.Equal(t, initialTradeCount, afterRejectionTrades)
	assert.Equal(t, initialCash, afterRejectionCash)

	// 3. Legitimate BUY Execution
	// Buy 100 shares at ₹2,000.00
	// Gross value: 100 * 2000 = ₹200,000.00
	// Statutory fees:
	// - STT: 200.00
	// - Stamp Duty: 30.00
	// - Exchange: 5.94
	// - SEBI: 0.20
	// - GST: 18% of (5.94 + 0.20) = 1.11
	// Total BUY Fees = 200.00 + 30.00 + 5.94 + 0.20 + 1.11 = ₹237.25
	// Total Cost = 200,000.00 + 237.25 = ₹200,237.25
	// Expected avg cost basis = 200,237.25 / 100 = ₹2,002.3725
	// Expected remaining cash = 1,000,000.00 - 200,237.25 = ₹799,762.75
	buyTrade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "BUY", 100, 2000.0)
	require.NoError(t, err)
	assert.Equal(t, 100, buyTrade.Shares)
	assert.InDelta(t, 2000.0, buyTrade.Price, 0.001)
	assert.InDelta(t, 237.25, buyTrade.FeesPaid, 0.01)
	assert.Nil(t, buyTrade.RealizedPnL)

	positions, err := db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, 100, positions[0].Shares)
	assert.InDelta(t, 2002.3725, positions[0].AvgCostBasis, 0.01)

	var currentCash float64
	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&currentCash)
	require.NoError(t, err)
	assert.InDelta(t, 799762.75, currentCash, 0.01)

	// 4. Partial SELL Execution (Sell 40 shares at ₹2,500.00)
	// Turnover = 40 * 2500 = ₹100,000.00
	// Statutory SELL Fees = ₹119.55
	// Net proceeds = 100,000.00 - 119.55 = ₹99,880.45
	// Cost of 40 shares sold = 40 * 2002.3725 = ₹80,094.90
	// Realized P&L = 99,880.45 - 80,094.90 = ₹19,785.55
	// Expected remaining shares = 100 - 40 = 60
	// Expected cash = 799,762.75 + 99,880.45 = ₹899,643.20
	sellTrade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "SELL", 40, 2500.0)
	require.NoError(t, err)
	assert.Equal(t, 40, sellTrade.Shares)
	assert.InDelta(t, 119.55, sellTrade.FeesPaid, 0.01)
	require.NotNil(t, sellTrade.RealizedPnL)
	assert.InDelta(t, 19785.55, *sellTrade.RealizedPnL, 0.01)

	positions, err = db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, 60, positions[0].Shares)
	assert.InDelta(t, 2002.3725, positions[0].AvgCostBasis, 0.01) // Avg cost basis per share unchanged on partial exit

	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&currentCash)
	require.NoError(t, err)
	assert.InDelta(t, 899643.20, currentCash, 0.01)

	// 5. Full Exit Execution (Sell remaining 60 shares at ₹2,600.00)
	// Turnover = 60 * 2600 = ₹156,000.00
	// Fees:
	// - STT: 156.00
	// - Exchange: 4.63
	// - SEBI: 0.16
	// - DP: 13.50
	// - GST: 18% of (4.63 + 0.16 + 13.50) = 18% of 18.29 = 3.29
	// Total Fees = 156.00 + 4.63 + 0.16 + 13.50 + 3.29 = ₹177.58
	// Net proceeds = 156,000.00 - 177.58 = ₹155,822.42
	// Cost of 60 shares sold = 60 * 2002.3725 = ₹120,142.35
	// Realized P&L = 155,822.42 - 120,142.35 = ₹35,680.07
	// Expected remaining shares = 0
	// Expected cash = 899,643.20 + 155,822.42 = ₹1,055,465.62
	fullSellTrade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "SELL", 60, 2600.0)
	require.NoError(t, err)
	assert.Equal(t, 60, fullSellTrade.Shares)
	assert.InDelta(t, 177.58, fullSellTrade.FeesPaid, 0.01)
	require.NotNil(t, fullSellTrade.RealizedPnL)
	assert.InDelta(t, 35680.07, *fullSellTrade.RealizedPnL, 0.01)

	positions, err = db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	assert.Empty(t, positions, "Positions must be empty after full exit (0 shares)")

	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&currentCash)
	require.NoError(t, err)
	assert.InDelta(t, 1055465.62, currentCash, 0.01)

	// Net Portfolio Capital Change = Total Realized P&L
	totalRealizedPnL := *sellTrade.RealizedPnL + *fullSellTrade.RealizedPnL
	capitalChange := currentCash - initialCash
	assert.InDelta(t, totalRealizedPnL, capitalChange, 0.01, "Final capital change must exactly equal the sum of realized trade P&Ls")
}

// TestConcurrentOrdersAtomicity verifies that concurrent order requests serialize safely via row-level locks,
// preventing double-spending and ledger corruption.
func TestConcurrentOrdersAtomicity(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var testPortfolioID int
	// Set initial cash to exactly enough for 2 orders of ₹10,000 but not 3
	initialCash := 25000.00
	err := db.QueryRowContext(ctx, `
		INSERT INTO portfolios (user_id, cash_balance, initial_capital)
		VALUES (88888, $1, $1)
		RETURNING id
	`, initialCash).Scan(&testPortfolioID)
	require.NoError(t, err)

	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolios WHERE id = $1`, testPortfolioID)
	}()

	var tcsSymbolID int
	err = db.QueryRowContext(ctx, `SELECT id FROM symbols WHERE symbol = 'TCS'`).Scan(&tcsSymbolID)
	require.NoError(t, err)

	// Launch 5 concurrent BUY orders of 5 shares @ ₹2,000 (Turnover ₹10,000 each)
	// Only 2 orders should succeed before cash drops below ₹10,000
	const numGoroutines = 5
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	successCount := 0
	failureCount := 0
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, tradeErr := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "BUY", 5, 2000.0)
			mu.Lock()
			if tradeErr == nil {
				successCount++
			} else {
				failureCount++
			}
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Assert that exactly 2 succeeded and 3 failed due to insufficient cash
	assert.Equal(t, 2, successCount, "Exactly 2 concurrent orders should have sufficient funds")
	assert.Equal(t, 3, failureCount, "Exactly 3 concurrent orders should be rejected cleanly")

	// Verify ledger integrity
	trades, err := db.GetPortfolioTrades(ctx, testPortfolioID)
	require.NoError(t, err)
	assert.Len(t, trades, 2, "Ledger must contain exactly 2 trades")

	// Verify position has exactly 10 shares (2 * 5)
	positions, err := db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, 10, positions[0].Shares)

	// Verify cash balance is positive and exactly matches deductions
	var finalCash float64
	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&finalCash)
	require.NoError(t, err)
	expectedDeduction := (10000.00 + database.CalculateDeliveryTransactionCosts("BUY", 10000.00).TotalFees) * 2
	assert.InDelta(t, initialCash-expectedDeduction, finalCash, 0.01)
}

// TestPaperTradingExactThreeStepLifecycle verifies the exact 3-step lifecycle:
// 1. BUY 100 shares @ ₹1,000.00 (Turnover ₹100,000.00)
// 2. SELL 40 shares @ ₹1,200.00 (Turnover ₹48,000.00, Partial Exit)
// 3. SELL 60 shares @ ₹1,100.00 (Turnover ₹66,000.00, Full Exit)
// Validates every charge, gross turnover, net cash flow, cost basis, realized P&L, holdings, and final cash reconciliation.
func TestPaperTradingExactThreeStepLifecycle(t *testing.T) {
	db := getTestDB(t)
	ctx := context.Background()

	var testPortfolioID int
	initialCash := 1000000.00
	err := db.QueryRowContext(ctx, `
		INSERT INTO portfolios (user_id, cash_balance, initial_capital)
		VALUES (77777, $1, $1)
		RETURNING id
	`, initialCash).Scan(&testPortfolioID)
	require.NoError(t, err)

	defer func() {
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_trades WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolio_positions WHERE portfolio_id = $1`, testPortfolioID)
		_, _ = db.ExecContext(ctx, `DELETE FROM portfolios WHERE id = $1`, testPortfolioID)
	}()

	var tcsSymbolID int
	err = db.QueryRowContext(ctx, `SELECT id FROM symbols WHERE symbol = 'TCS'`).Scan(&tcsSymbolID)
	require.NoError(t, err)

	// Step 1: BUY 100 shares at ₹1,000.00
	// Gross turnover: 100 * 1,000 = ₹100,000.00
	// Fees: STT=100.00, Stamp=15.00, Exch=2.97, SEBI=0.10, DP=0.00, GST=0.55 -> Total Fees = ₹118.62
	// Total cost debited = ₹100,118.62
	// Expected avg cost basis = 100,118.62 / 100 = ₹1,001.1862
	// Expected cash = 1,000,000.00 - 100,118.62 = ₹899,881.38
	buyCosts := database.CalculateDeliveryTransactionCosts("BUY", 100000.00)
	assert.Equal(t, 100.00, buyCosts.STT)
	assert.Equal(t, 15.00, buyCosts.StampDuty)
	assert.Equal(t, 2.97, buyCosts.ExchangeCharges)
	assert.Equal(t, 0.10, buyCosts.SEBICharges)
	assert.Equal(t, 0.00, buyCosts.DPCharges)
	assert.Equal(t, 0.55, buyCosts.GST)
	assert.Equal(t, 118.62, buyCosts.TotalFees)

	buyTrade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "BUY", 100, 1000.0)
	require.NoError(t, err)
	assert.Equal(t, 100, buyTrade.Shares)
	assert.Equal(t, 1000.0, buyTrade.Price)
	assert.Equal(t, 118.62, buyTrade.FeesPaid)
	assert.Nil(t, buyTrade.RealizedPnL)

	positions, err := db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, 100, positions[0].Shares)
	assert.InDelta(t, 1001.1862, positions[0].AvgCostBasis, 0.001)

	var cashAfterBuy float64
	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&cashAfterBuy)
	require.NoError(t, err)
	assert.Equal(t, 899881.38, cashAfterBuy)

	// Step 2: SELL 40 shares at ₹1,200.00 (Partial Exit)
	// Gross turnover: 40 * 1,200 = ₹48,000.00
	// Fees: STT=48.00, Stamp=0.00, Exch=1.43, SEBI=0.05, DP=13.50, GST=2.70 -> Total Fees = ₹65.68
	// Net proceeds credited = 48,000.00 - 65.68 = ₹47,934.32
	// Cost of 40 shares sold = 40 * 1001.1862 = ₹40,047.4480
	// Realized P&L = 47,934.32 - 40,047.4480 = +₹7,886.8720 (or ₹7,886.87)
	// Remaining shares = 60 @ ₹1,001.1862
	// Expected cash = 899,881.38 + 47,934.32 = ₹947,815.70
	sell1Costs := database.CalculateDeliveryTransactionCosts("SELL", 48000.00)
	assert.Equal(t, 48.00, sell1Costs.STT)
	assert.Equal(t, 0.00, sell1Costs.StampDuty)
	assert.Equal(t, 1.43, sell1Costs.ExchangeCharges)
	assert.Equal(t, 0.05, sell1Costs.SEBICharges)
	assert.Equal(t, 13.50, sell1Costs.DPCharges)
	assert.Equal(t, 2.70, sell1Costs.GST)
	assert.Equal(t, 65.68, sell1Costs.TotalFees)

	sell1Trade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "SELL", 40, 1200.0)
	require.NoError(t, err)
	assert.Equal(t, 40, sell1Trade.Shares)
	assert.Equal(t, 1200.0, sell1Trade.Price)
	assert.Equal(t, 65.68, sell1Trade.FeesPaid)
	require.NotNil(t, sell1Trade.RealizedPnL)
	assert.InDelta(t, 7886.872, *sell1Trade.RealizedPnL, 0.01)

	positions, err = db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, 60, positions[0].Shares)
	assert.InDelta(t, 1001.1862, positions[0].AvgCostBasis, 0.001)

	var cashAfterSell1 float64
	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&cashAfterSell1)
	require.NoError(t, err)
	assert.Equal(t, 947815.70, cashAfterSell1)

	// Step 3: SELL 60 shares at ₹1,100.00 (Full Exit)
	// Gross turnover: 60 * 1,100 = ₹66,000.00
	// Fees: STT=66.00, Stamp=0.00, Exch=1.96, SEBI=0.07, DP=13.50, GST=2.80 -> Total Fees = ₹84.33
	// Net proceeds credited = 66,000.00 - 84.33 = ₹65,915.67
	// Cost of 60 shares sold = 60 * 1001.1862 = ₹60,071.1720
	// Realized P&L = 65,915.67 - 60,071.1720 = +₹5,844.4980 (or ₹5,844.50)
	// Remaining shares = 0 (closed)
	// Expected cash = 947,815.70 + 65,915.67 = ₹1,013,731.37
	sell2Costs := database.CalculateDeliveryTransactionCosts("SELL", 66000.00)
	assert.Equal(t, 66.00, sell2Costs.STT)
	assert.Equal(t, 0.00, sell2Costs.StampDuty)
	assert.Equal(t, 1.96, sell2Costs.ExchangeCharges)
	assert.Equal(t, 0.07, sell2Costs.SEBICharges)
	assert.Equal(t, 13.50, sell2Costs.DPCharges)
	assert.Equal(t, 2.80, sell2Costs.GST)
	assert.Equal(t, 84.33, sell2Costs.TotalFees)

	sell2Trade, err := db.ExecutePaperTrade(ctx, testPortfolioID, tcsSymbolID, "SELL", 60, 1100.0)
	require.NoError(t, err)
	assert.Equal(t, 60, sell2Trade.Shares)
	assert.Equal(t, 1100.0, sell2Trade.Price)
	assert.Equal(t, 84.33, sell2Trade.FeesPaid)
	require.NotNil(t, sell2Trade.RealizedPnL)
	assert.InDelta(t, 5844.498, *sell2Trade.RealizedPnL, 0.01)

	// Position should be closed (0 active positions with shares > 0)
	positions, err = db.GetPortfolioPositions(ctx, testPortfolioID)
	require.NoError(t, err)
	assert.Empty(t, positions)

	var finalCash float64
	err = db.QueryRowContext(ctx, `SELECT cash_balance FROM portfolios WHERE id = $1`, testPortfolioID).Scan(&finalCash)
	require.NoError(t, err)
	assert.Equal(t, 1013731.37, finalCash)

	// Total Reconciliation
	totalRealizedPnL := *sell1Trade.RealizedPnL + *sell2Trade.RealizedPnL
	assert.InDelta(t, 13731.37, totalRealizedPnL, 0.01)
	assert.InDelta(t, initialCash+totalRealizedPnL, finalCash, 0.001)
}
