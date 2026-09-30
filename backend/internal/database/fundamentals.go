package database

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// UpsertFinancialStatements inserts or updates a batch of financial statements.
// Uses PostgreSQL ON CONFLICT for deduplication — same (symbol_id, period_type, period_end)
// will update rather than create duplicates, ensuring historical preservation.
func (db *DB) UpsertFinancialStatements(ctx context.Context, records []models.FinancialStatement) error {
	if len(records) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO financial_statements (
			symbol_id, period_type, period_end, publication_date, pit_mode, fiscal_year, 
			revenue, ebitda, pat, eps, total_debt, total_equity, 
			total_assets, operating_cash_flow, free_cash_flow, 
			interest_expense, interest_income, cash_and_equivalents, 
			total_receivables, shares_outstanding, source, quality_status, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, 
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, NOW()
		) ON CONFLICT (symbol_id, period_type, period_end) DO UPDATE SET
			publication_date = EXCLUDED.publication_date,
			pit_mode = EXCLUDED.pit_mode,
			fiscal_year = EXCLUDED.fiscal_year,
			revenue = EXCLUDED.revenue,
			ebitda = EXCLUDED.ebitda,
			pat = EXCLUDED.pat,
			eps = EXCLUDED.eps,
			total_debt = EXCLUDED.total_debt,
			total_equity = EXCLUDED.total_equity,
			total_assets = EXCLUDED.total_assets,
			operating_cash_flow = EXCLUDED.operating_cash_flow,
			free_cash_flow = EXCLUDED.free_cash_flow,
			interest_expense = EXCLUDED.interest_expense,
			interest_income = EXCLUDED.interest_income,
			cash_and_equivalents = EXCLUDED.cash_and_equivalents,
			total_receivables = EXCLUDED.total_receivables,
			shares_outstanding = EXCLUDED.shares_outstanding,
			source = EXCLUDED.source,
			quality_status = EXCLUDED.quality_status,
			retrieved_at = NOW()
	`)
	if err != nil {
		return fmt.Errorf("prepare statement failed: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.ExecContext(ctx,
			r.SymbolID, r.PeriodType, r.PeriodEnd, r.PublicationDate, r.PITMode, r.FiscalYear,
			r.Revenue, r.EBITDA, r.PAT, r.EPS, r.TotalDebt, r.TotalEquity,
			r.TotalAssets, r.OperatingCashFlow, r.FreeCashFlow,
			r.InterestExpense, r.InterestIncome, r.Cash,
			r.TotalReceivables, r.SharesOutstanding, r.Source, string(r.QualityStatus),
		)
		if err != nil {
			log.Printf("[DB] WARN: failed to upsert financial statement for symbol %d at %v: %v",
				r.SymbolID, r.PeriodEnd, err)
			continue
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	return nil
}

// GetFinancialStatementsAsOf retrieves the most recently published financial statement on or before a given date.
func (db *DB) GetFinancialStatementsAsOf(ctx context.Context, symbolID int, asOf time.Time) (*models.FinancialStatement, error) {
	query := `
		SELECT id, symbol_id, period_type, period_end, publication_date, pit_mode, fiscal_year,
			revenue, ebitda, pat, eps, total_debt, total_equity,
			total_assets, operating_cash_flow, free_cash_flow,
			interest_expense, interest_income, cash_and_equivalents,
			total_receivables, shares_outstanding
		FROM financial_statements
		WHERE symbol_id = $1 AND publication_date <= $2
		ORDER BY publication_date DESC, period_end DESC
		LIMIT 1
	`
	var r models.FinancialStatement
	err := db.QueryRowContext(ctx, query, symbolID, asOf).Scan(
		&r.ID, &r.SymbolID, &r.PeriodType, &r.PeriodEnd, &r.PublicationDate, &r.PITMode, &r.FiscalYear,
		&r.Revenue, &r.EBITDA, &r.PAT, &r.EPS, &r.TotalDebt, &r.TotalEquity,
		&r.TotalAssets, &r.OperatingCashFlow, &r.FreeCashFlow,
		&r.InterestExpense, &r.InterestIncome, &r.Cash,
		&r.TotalReceivables, &r.SharesOutstanding,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}
