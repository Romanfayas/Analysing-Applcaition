package database

import (
	"context"
	"fmt"
	"time"
)

// ShariahScreeningRecord represents a stored Shariah screening result.
type ShariahScreeningRecord struct {
	ID                   int        `json:"id"`
	SymbolID             int        `json:"symbol_id"`
	RuleSetID            int        `json:"rule_set_id"`
	Status               string     `json:"status"` // PASS, FAIL, REVIEW_REQUIRED, DATA_INSUFFICIENT
	DebtRatio            *float64   `json:"debt_ratio"`
	DebtRatioPass        *bool      `json:"debt_ratio_pass"`
	InterestIncomeRatio  *float64   `json:"interest_income_ratio"`
	InterestIncomePass   *bool      `json:"interest_income_pass"`
	CashDepositRatio     *float64   `json:"cash_deposit_ratio"`
	CashDepositPass      *bool      `json:"cash_deposit_pass"`
	ReceivablesRatio     *float64   `json:"receivables_ratio"`
	ReceivablesPass      *bool      `json:"receivables_pass"`
	BusinessActivityPass *bool      `json:"business_activity_pass"`
	PurificationPerShare *float64   `json:"purification_per_share"`
	ScreenedAt           time.Time  `json:"screened_at"`
}

// SignalRecord represents a stored signal.
type SignalRecord struct {
	ID                int       `json:"id"`
	SymbolID          int       `json:"symbol_id"`
	Signal            string    `json:"signal"` // STRONG_BUY, BUY, WATCH, AVOID, STRONG_AVOID
	OverallScore      float64   `json:"overall_score"`
	FundamentalScore  *float64  `json:"fundamental_score"`
	TechnicalScore    *float64  `json:"technical_score"`
	CandlestickScore  *float64  `json:"candlestick_score"`
	MomentumScore     *float64  `json:"momentum_score"`
	VolumeScore       *float64  `json:"volume_score"`
	ValuationScore    *float64  `json:"valuation_score"`
	MarketRegimeScore *float64  `json:"market_regime_score"`
	RiskScore         *float64  `json:"risk_score"`
	QualityScore      *float64  `json:"quality_score"`
	ShariahStatus     string    `json:"shariah_status"`
	Explanation       string    `json:"explanation"`
	CalculatedAt      time.Time `json:"calculated_at"`
}

// IPORecord represents a stored IPO.
type IPORecord struct {
	ID                int        `json:"id"`
	CompanyName       string     `json:"company_name"`
	Symbol            *string    `json:"symbol"`
	Exchange          string     `json:"exchange"`
	Sector            *string    `json:"sector"`
	OpenDate          *time.Time `json:"open_date"`
	CloseDate         *time.Time `json:"close_date"`
	ListingDate       *time.Time `json:"listing_date"`
	PriceBandLow      *float64   `json:"price_band_low"`
	PriceBandHigh     *float64   `json:"price_band_high"`
	IssuePrice        *float64   `json:"issue_price"`
	ListingPrice      *float64   `json:"listing_price"`
	LotSize           *int       `json:"lot_size"`
	IssueSize         *float64   `json:"issue_size"`
	Status            string     `json:"status"`
	ShariahStatus     *string    `json:"shariah_status"`
}

// UpsertShariahScreening stores a Shariah screening result.
func (db *DB) UpsertShariahScreening(ctx context.Context, r ShariahScreeningRecord) (int, error) {
	query := `
		INSERT INTO shariah_screenings (symbol_id, rule_set_id, status,
			debt_ratio, debt_ratio_pass,
			interest_income_ratio, interest_income_pass,
			cash_deposit_ratio, cash_deposit_pass,
			receivables_ratio, receivables_pass,
			business_activity_pass, purification_per_share)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (symbol_id, rule_set_id)
		WHERE screened_at < NOW() - INTERVAL '1 day'
		DO UPDATE SET
			status = EXCLUDED.status,
			debt_ratio = EXCLUDED.debt_ratio,
			debt_ratio_pass = EXCLUDED.debt_ratio_pass,
			interest_income_ratio = EXCLUDED.interest_income_ratio,
			interest_income_pass = EXCLUDED.interest_income_pass,
			cash_deposit_ratio = EXCLUDED.cash_deposit_ratio,
			cash_deposit_pass = EXCLUDED.cash_deposit_pass,
			receivables_ratio = EXCLUDED.receivables_ratio,
			receivables_pass = EXCLUDED.receivables_pass,
			business_activity_pass = EXCLUDED.business_activity_pass,
			purification_per_share = EXCLUDED.purification_per_share,
			screened_at = NOW()
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		r.SymbolID, r.RuleSetID, r.Status,
		r.DebtRatio, r.DebtRatioPass,
		r.InterestIncomeRatio, r.InterestIncomePass,
		r.CashDepositRatio, r.CashDepositPass,
		r.ReceivablesRatio, r.ReceivablesPass,
		r.BusinessActivityPass, r.PurificationPerShare,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert shariah screening failed: %w", err)
	}
	return id, nil
}

// GetLatestShariahScreening retrieves the most recent screening for a symbol.
func (db *DB) GetLatestShariahScreening(ctx context.Context, symbolID int) (*ShariahScreeningRecord, error) {
	query := `
		SELECT id, symbol_id, rule_set_id, status,
			debt_ratio, debt_ratio_pass,
			interest_income_ratio, interest_income_pass,
			cash_deposit_ratio, cash_deposit_pass,
			receivables_ratio, receivables_pass,
			business_activity_pass, purification_per_share, screened_at
		FROM shariah_screenings
		WHERE symbol_id = $1
		ORDER BY screened_at DESC
		LIMIT 1
	`
	var r ShariahScreeningRecord
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&r.ID, &r.SymbolID, &r.RuleSetID, &r.Status,
		&r.DebtRatio, &r.DebtRatioPass,
		&r.InterestIncomeRatio, &r.InterestIncomePass,
		&r.CashDepositRatio, &r.CashDepositPass,
		&r.ReceivablesRatio, &r.ReceivablesPass,
		&r.BusinessActivityPass, &r.PurificationPerShare, &r.ScreenedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetShariahScreeningAsOf retrieves the most recent screening for a symbol on or before the given date.
func (db *DB) GetShariahScreeningAsOf(ctx context.Context, symbolID int, asOf time.Time) (*ShariahScreeningRecord, error) {
	query := `
		SELECT id, symbol_id, rule_set_id, status,
			debt_ratio, debt_ratio_pass,
			interest_income_ratio, interest_income_pass,
			cash_deposit_ratio, cash_deposit_pass,
			receivables_ratio, receivables_pass,
			business_activity_pass, purification_per_share, screened_at
		FROM shariah_screenings
		WHERE symbol_id = $1 AND screened_at <= $2
		ORDER BY screened_at DESC
		LIMIT 1
	`
	var r ShariahScreeningRecord
	err := db.QueryRowContext(ctx, query, symbolID, asOf).Scan(
		&r.ID, &r.SymbolID, &r.RuleSetID, &r.Status,
		&r.DebtRatio, &r.DebtRatioPass,
		&r.InterestIncomeRatio, &r.InterestIncomePass,
		&r.CashDepositRatio, &r.CashDepositPass,
		&r.ReceivablesRatio, &r.ReceivablesPass,
		&r.BusinessActivityPass, &r.PurificationPerShare, &r.ScreenedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// UpsertSignal stores a signal calculation result.
func (db *DB) UpsertSignal(ctx context.Context, r SignalRecord) (int, error) {
	query := `
		INSERT INTO stock_signals (symbol_id, signal, overall_score,
			fundamental_score, technical_score, candlestick_score,
			momentum_score, volume_score, valuation_score,
			market_regime_score, risk_score, quality_score,
			shariah_status, explanation)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		r.SymbolID, r.Signal, r.OverallScore,
		r.FundamentalScore, r.TechnicalScore, r.CandlestickScore,
		r.MomentumScore, r.VolumeScore, r.ValuationScore,
		r.MarketRegimeScore, r.RiskScore, r.QualityScore,
		r.ShariahStatus, r.Explanation,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert signal failed: %w", err)
	}
	return id, nil
}

// GetLatestSignal retrieves the most recent signal for a symbol.
func (db *DB) GetLatestSignal(ctx context.Context, symbolID int) (*SignalRecord, error) {
	query := `
		SELECT id, symbol_id, signal, overall_score,
			fundamental_score, technical_score, candlestick_score,
			momentum_score, volume_score, valuation_score,
			market_regime_score, risk_score, quality_score,
			shariah_status, explanation, calculated_at
		FROM stock_signals
		WHERE symbol_id = $1
		ORDER BY calculated_at DESC
		LIMIT 1
	`
	var r SignalRecord
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&r.ID, &r.SymbolID, &r.Signal, &r.OverallScore,
		&r.FundamentalScore, &r.TechnicalScore, &r.CandlestickScore,
		&r.MomentumScore, &r.VolumeScore, &r.ValuationScore,
		&r.MarketRegimeScore, &r.RiskScore, &r.QualityScore,
		&r.ShariahStatus, &r.Explanation, &r.CalculatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetActiveSignals retrieves all current active signals.
func (db *DB) GetActiveSignals(ctx context.Context) ([]SignalRecord, error) {
	query := `
		SELECT DISTINCT ON (symbol_id)
			id, symbol_id, signal, overall_score,
			fundamental_score, technical_score, candlestick_score,
			momentum_score, volume_score, valuation_score,
			market_regime_score, risk_score, quality_score,
			shariah_status, explanation, calculated_at
		FROM stock_signals
		ORDER BY symbol_id, calculated_at DESC
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query active signals failed: %w", err)
	}
	defer rows.Close()

	var signals []SignalRecord
	for rows.Next() {
		var r SignalRecord
		if err := rows.Scan(
			&r.ID, &r.SymbolID, &r.Signal, &r.OverallScore,
			&r.FundamentalScore, &r.TechnicalScore, &r.CandlestickScore,
			&r.MomentumScore, &r.VolumeScore, &r.ValuationScore,
			&r.MarketRegimeScore, &r.RiskScore, &r.QualityScore,
			&r.ShariahStatus, &r.Explanation, &r.CalculatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan signal row failed: %w", err)
		}
		signals = append(signals, r)
	}
	return signals, rows.Err()
}

// UpsertIPO stores an IPO record.
func (db *DB) UpsertIPO(ctx context.Context, r IPORecord) (int, error) {
	query := `
		INSERT INTO ipos (company_name, symbol, exchange, sector,
			open_date, close_date, listing_date,
			price_band_low, price_band_high, issue_price, listing_price,
			lot_size, issue_size, status, shariah_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		ON CONFLICT (company_name, exchange)
		DO UPDATE SET
			symbol = COALESCE(EXCLUDED.symbol, ipos.symbol),
			status = EXCLUDED.status,
			listing_price = COALESCE(EXCLUDED.listing_price, ipos.listing_price),
			shariah_status = COALESCE(EXCLUDED.shariah_status, ipos.shariah_status),
			updated_at = NOW()
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		r.CompanyName, r.Symbol, r.Exchange, r.Sector,
		r.OpenDate, r.CloseDate, r.ListingDate,
		r.PriceBandLow, r.PriceBandHigh, r.IssuePrice, r.ListingPrice,
		r.LotSize, r.IssueSize, r.Status, r.ShariahStatus,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert IPO failed: %w", err)
	}
	return id, nil
}

// GetIPOs retrieves IPOs filtered by status.
func (db *DB) GetIPOs(ctx context.Context, status string) ([]IPORecord, error) {
	query := `
		SELECT id, company_name, symbol, exchange, sector,
			open_date, close_date, listing_date,
			price_band_low, price_band_high, issue_price, listing_price,
			lot_size, issue_size, status, shariah_status
		FROM ipos
	`
	var args []interface{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY COALESCE(open_date, listing_date, created_at) DESC`

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query IPOs failed: %w", err)
	}
	defer rows.Close()

	var ipos []IPORecord
	for rows.Next() {
		var r IPORecord
		if err := rows.Scan(
			&r.ID, &r.CompanyName, &r.Symbol, &r.Exchange, &r.Sector,
			&r.OpenDate, &r.CloseDate, &r.ListingDate,
			&r.PriceBandLow, &r.PriceBandHigh, &r.IssuePrice, &r.ListingPrice,
			&r.LotSize, &r.IssueSize, &r.Status, &r.ShariahStatus,
		); err != nil {
			return nil, fmt.Errorf("scan IPO row failed: %w", err)
		}
		ipos = append(ipos, r)
	}
	return ipos, rows.Err()
}
