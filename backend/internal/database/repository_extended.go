package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/models"
	"github.com/halal-equity/backend/internal/services/market"
)

// ShariahScreeningRecord represents a stored Shariah screening result.
type ShariahScreeningRecord struct {
	ID                   int       `json:"id"`
	SymbolID             int       `json:"symbol_id"`
	RuleSetID            int       `json:"rule_set_id"`
	Status               string    `json:"status"` // PASS, FAIL, REVIEW_REQUIRED, DATA_INSUFFICIENT
	DebtRatio            *float64  `json:"debt_ratio"`
	DebtRatioPass        *bool     `json:"debt_ratio_pass"`
	InterestIncomeRatio  *float64  `json:"interest_income_ratio"`
	InterestIncomePass   *bool     `json:"interest_income_pass"`
	CashDepositRatio     *float64  `json:"cash_deposit_ratio"`
	CashDepositPass      *bool     `json:"cash_deposit_pass"`
	ReceivablesRatio     *float64  `json:"receivables_ratio"`
	ReceivablesPass      *bool     `json:"receivables_pass"`
	BusinessActivityPass *bool     `json:"business_activity_pass"`
	PurificationPerShare *float64  `json:"purification_per_share"`
	ScreenedAt           time.Time `json:"screened_at"`
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
	ID            int        `json:"id"`
	CompanyName   string     `json:"company_name"`
	Symbol        *string    `json:"symbol"`
	Exchange      string     `json:"exchange"`
	Sector        *string    `json:"sector"`
	OpenDate      *time.Time `json:"open_date"`
	CloseDate     *time.Time `json:"close_date"`
	ListingDate   *time.Time `json:"listing_date"`
	PriceBandLow  *float64   `json:"price_band_low"`
	PriceBandHigh *float64   `json:"price_band_high"`
	IssuePrice    *float64   `json:"issue_price"`
	ListingPrice  *float64   `json:"listing_price"`
	LotSize       *int       `json:"lot_size"`
	IssueSize     *float64   `json:"issue_size"`
	Status        string     `json:"status"`
	ShariahStatus *string    `json:"shariah_status"`
	Source        *string    `json:"source"`
	Board         *string    `json:"board"` // MAINBOARD | SME, from the exchange's series; nil when unknown
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

// GetLatestFundamentals retrieves the most recent financial statement for a symbol.
func (db *DB) GetLatestFundamentals(ctx context.Context, symbolID int) (*models.FinancialStatement, error) {
	query := `
		SELECT id, symbol_id, period_type, period_end, publication_date, pit_mode, 
		       revenue, ebitda, pat, total_assets, total_debt, eps, retrieved_at, quality_status
		FROM financial_statements
		WHERE symbol_id = $1
		ORDER BY period_end DESC, publication_date DESC
		LIMIT 1
	`
	var fs models.FinancialStatement
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&fs.ID,
		&fs.SymbolID,
		&fs.PeriodType,
		&fs.PeriodEnd,
		&fs.PublicationDate,
		&fs.PITMode,
		&fs.Revenue,
		&fs.EBITDA,
		&fs.PAT,
		&fs.TotalAssets,
		&fs.TotalDebt,
		&fs.EPS,
		&fs.RetrievedAt,
		&fs.QualityStatus,
	)
	if err != nil {
		return nil, err
	}
	return &fs, nil
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

// metadataJSON is the descriptive metadata stored with the issue (board and listing venue, when known).
func (r IPORecord) metadataJSON() string {
	m := map[string]string{}
	if r.Board != nil && *r.Board != "" {
		m["board"] = *r.Board
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// UpsertIPO stores an IPO record.
func (db *DB) UpsertIPO(ctx context.Context, r IPORecord) (int, error) {
	id, _, err := db.UpsertIPOReportingInsert(ctx, r)
	return id, err
}

// UpsertIPOReportingInsert stores an IPO and reports whether the row was newly created.
// Existing rows are refreshed with the latest dates, price band and size, but a row that has
// already reached a terminal historical state is never moved back to an active status.
func (db *DB) UpsertIPOReportingInsert(ctx context.Context, r IPORecord) (id int, inserted bool, err error) {
	query := `
		INSERT INTO ipos (company_name, symbol, exchange, sector,
			open_date, close_date, listing_date,
			price_band_low, price_band_high, issue_price, listing_price,
			lot_size, issue_size, status, shariah_status, source, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
		        $17::jsonb)
		ON CONFLICT (company_name, exchange)
		DO UPDATE SET
			symbol = COALESCE(EXCLUDED.symbol, ipos.symbol),
			open_date = COALESCE(EXCLUDED.open_date, ipos.open_date),
			close_date = COALESCE(EXCLUDED.close_date, ipos.close_date),
			price_band_low = COALESCE(EXCLUDED.price_band_low, ipos.price_band_low),
			price_band_high = COALESCE(EXCLUDED.price_band_high, ipos.price_band_high),
			issue_size = COALESCE(EXCLUDED.issue_size, ipos.issue_size),
			status = CASE WHEN ipos.status IN ('HISTORICAL_LISTED', 'LISTED', 'ARCHIVED')
			              THEN ipos.status ELSE EXCLUDED.status END,
			listing_date = COALESCE(EXCLUDED.listing_date, ipos.listing_date),
			listing_price = COALESCE(EXCLUDED.listing_price, ipos.listing_price),
			shariah_status = COALESCE(EXCLUDED.shariah_status, ipos.shariah_status),
			source = COALESCE(EXCLUDED.source, ipos.source),
			metadata = COALESCE(ipos.metadata, '{}'::jsonb) || EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING id, (xmax = 0)
	`
	err = db.QueryRowContext(ctx, query,
		r.CompanyName, r.Symbol, r.Exchange, r.Sector,
		r.OpenDate, r.CloseDate, r.ListingDate,
		r.PriceBandLow, r.PriceBandHigh, r.IssuePrice, r.ListingPrice,
		r.LotSize, r.IssueSize, r.Status, r.ShariahStatus, r.Source, r.metadataJSON(),
	).Scan(&id, &inserted)
	if err != nil {
		return 0, false, fmt.Errorf("upsert IPO failed: %w", err)
	}
	return id, inserted, nil
}

// GetIPOs retrieves IPOs filtered by status.
func (db *DB) GetIPOs(ctx context.Context, status string) ([]IPORecord, error) {
	query := `
		SELECT id, company_name, symbol, exchange, sector,
			open_date, close_date, listing_date,
			price_band_low, price_band_high, issue_price, listing_price,
			lot_size, issue_size, status,
			COALESCE(metadata->>'shariah_status', 'DATA_UNAVAILABLE') as shariah_status
		FROM ipos
	`
	var args []interface{}
	if status != "" && status != "active" {
		query += ` WHERE status = $1`
		args = append(args, status)
	} else if status == "active" {
		query += ` WHERE status IN ('UPCOMING', 'OPEN', 'CLOSED', 'ALLOTMENT')`
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

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ExpectedListingDate is the SEBI T+3 listing timeline: an issue must list within three working days of closing.
// It is an expectation derived from a regulatory rule, not an exchange announcement, and is always labelled as such.
func ExpectedListingDate(closeDate time.Time) *time.Time {
	d := time.Date(closeDate.Year(), closeDate.Month(), closeDate.Day(), 0, 0, 0, 0, market.Location)
	for n := 0; n < 3; {
		d = d.AddDate(0, 0, 1)
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if holiday, _ := market.IsTradingHoliday(d); holiday {
			continue
		}
		n++
	}
	out := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	return &out
}

// GetEnrichedIPOs retrieves all IPO records with joined GMP, subscriptions, financials, and predictions.
func (db *DB) GetEnrichedIPOs(ctx context.Context, statusFilter string) ([]map[string]any, error) {
	query := `
		SELECT 
			i.id, i.company_name, COALESCE(i.symbol, ''), i.exchange, COALESCE(i.sector, ''),
			i.open_date, i.close_date, i.listing_date,
			i.price_band_low, i.price_band_high, i.issue_price, i.listing_price,
			i.lot_size, i.min_investment, i.issue_size, i.fresh_issue, i.ofs,
			i.status, COALESCE(i.source, 'OFFICIAL_FILINGS'), COALESCE(i.metadata, '{}'::jsonb),
			-- Latest Subscriptions
			sub.qib_subscription, sub.nii_subscription, sub.retail_subscription, sub.total_subscription, sub.recorded_at,
			-- Latest GMP
			gmp.gmp_value, gmp.gmp_percentage, gmp.source, gmp.recorded_at,
			-- Latest Prediction with Point-In-Time fields
			pred.predicted_price, pred.predicted_return, pred.bull_case, pred.base_case, pred.bear_case, pred.confidence_score, pred.factors,
			pred.predicted_at, pred.data_cutoff_at, pred.lifecycle_state_at_prediction, pred.inputs,
			pred.actual_listing_price, pred.actual_listing_return, pred.prediction_error_pct,
			-- Latest Financials
			fin.revenue, fin.pat, fin.eps,
			det.data, i.listing_day_close
		FROM ipos i
		LEFT JOIN ipo_details det ON det.ipo_id = i.id
		LEFT JOIN LATERAL (
			SELECT qib_subscription, nii_subscription, retail_subscription, total_subscription, recorded_at
			FROM ipo_subscriptions WHERE ipo_id = i.id ORDER BY recorded_at DESC LIMIT 1
		) sub ON true
		LEFT JOIN LATERAL (
			SELECT gmp_value, gmp_percentage, source, recorded_at
			FROM ipo_gmp WHERE ipo_id = i.id ORDER BY recorded_at DESC LIMIT 1
		) gmp ON true
		LEFT JOIN LATERAL (
			SELECT predicted_price, predicted_return, bull_case, base_case, bear_case, confidence_score, factors,
			       predicted_at, data_cutoff_at, lifecycle_state_at_prediction, inputs,
			       actual_listing_price, actual_listing_return, prediction_error_pct
			FROM ipo_predictions WHERE ipo_id = i.id ORDER BY predicted_at DESC LIMIT 1
		) pred ON true
		LEFT JOIN LATERAL (
			SELECT revenue, pat, eps
			FROM ipo_financials WHERE ipo_id = i.id ORDER BY period_end DESC LIMIT 1
		) fin ON true
		ORDER BY COALESCE(i.open_date, i.listing_date, i.created_at) DESC
	`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query enriched IPOs: %w", err)
	}
	defer rows.Close()

	var results []map[string]any
	for rows.Next() {
		var id int
		var compName, symbol, exchange, sector, source string
		var openDate, closeDate, listingDate *time.Time
		var pbLow, pbHigh, issuePrice, listPrice, minInv, issueSize, freshIssue, ofs *float64
		var lotSize *int
		var status string
		var metadata, detailsRaw []byte
		var listingDayClose *float64

		// Subscriptions
		var qibSub, niiSub, retailSub, totalSub *float64
		var subRecordedAt *time.Time

		// GMP
		var gmpVal, gmpPct *float64
		var gmpSource *string
		var gmpRecordedAt *time.Time

		// Predictions
		var predPrice, predReturn, bullCase, baseCase, bearCase, confScore *float64
		var predFactors []byte
		var predAt, predCutoffAt *time.Time
		var predLifecycleState *string
		var predInputs []byte
		var actualListPrice, actualListReturn, predErrorPct *float64

		// Financials
		var rev, pat, eps *float64

		if err := rows.Scan(
			&id, &compName, &symbol, &exchange, &sector,
			&openDate, &closeDate, &listingDate,
			&pbLow, &pbHigh, &issuePrice, &listPrice,
			&lotSize, &minInv, &issueSize, &freshIssue, &ofs,
			&status, &source, &metadata,
			&qibSub, &niiSub, &retailSub, &totalSub, &subRecordedAt,
			&gmpVal, &gmpPct, &gmpSource, &gmpRecordedAt,
			&predPrice, &predReturn, &bullCase, &baseCase, &bearCase, &confScore, &predFactors,
			&predAt, &predCutoffAt, &predLifecycleState, &predInputs,
			&actualListPrice, &actualListReturn, &predErrorPct,
			&rev, &pat, &eps,
			&detailsRaw, &listingDayClose,
		); err != nil {
			return nil, fmt.Errorf("scan enriched IPO failed: %w", err)
		}

		priceBandStr := ""
		if pbLow != nil && pbHigh != nil {
			priceBandStr = fmt.Sprintf("₹%.0f - ₹%.0f", *pbLow, *pbHigh)
		} else if issuePrice != nil {
			priceBandStr = fmt.Sprintf("₹%.0f", *issuePrice)
		}

		issueSizeStr := ""
		if issueSize != nil {
			issueSizeStr = fmt.Sprintf("₹%.2f Cr", *issueSize)
		}

		// Subscription object
		var subObj map[string]any
		if totalSub != nil {
			subObj = map[string]any{
				"qib":         qibSub,
				"nii":         niiSub,
				"retail":      retailSub,
				"total":       *totalSub,
				"recorded_at": subRecordedAt,
			}
		}

		// GMP object with disclaimer
		var gmpObj map[string]any
		if gmpVal != nil {
			gmpObj = map[string]any{
				"value":      *gmpVal,
				"percentage": gmpPct,
				"source":     gmpSource,
				"updated_at": gmpRecordedAt,
				"disclaimer": "Unofficial third-party estimate. Not exchange-certified.",
			}
		}

		// Prediction object
		var predObj map[string]any
		var shariahStatus = "UNKNOWN"
		if len(metadata) > 0 {
			var metaMap map[string]any
			if json.Unmarshal(metadata, &metaMap) == nil {
				if sh, ok := metaMap["shariah_status"].(string); ok {
					shariahStatus = sh
				}
			}
		}

		if predPrice != nil {
			rec := "REVIEW_REQUIRED"
			expl := "Point-in-time algorithmic research scenario based on pre-listing fundamentals and subscription cutoff."
			if len(predFactors) > 0 {
				var facMap map[string]any
				if json.Unmarshal(predFactors, &facMap) == nil {
					if r, ok := facMap["recommendation"].(string); ok {
						rec = r
					}
					if e, ok := facMap["explanation"].(string); ok {
						expl = e
					}
					if s, ok := facMap["shariah_status"].(string); ok {
						shariahStatus = s
					}
				}
			}

			var inputsMap map[string]any
			if len(predInputs) > 0 {
				_ = json.Unmarshal(predInputs, &inputsMap)
			}

			predObj = map[string]any{
				"predicted_price":               predPrice,
				"listing_gains_pct":             predReturn,
				"base_case":                     baseCase,
				"bull_case":                     bullCase,
				"bear_case":                     bearCase,
				"confidence":                    confScore,
				"recommendation":                rec,
				"explanation":                   expl,
				"shariah_status":                shariahStatus,
				"predicted_at":                  predAt,
				"data_cutoff_at":                predCutoffAt,
				"lifecycle_state_at_prediction": predLifecycleState,
				"inputs":                        inputsMap,
				"actual_listing_price":          actualListPrice,
				"actual_listing_return":         actualListReturn,
				"prediction_error_pct":          predErrorPct,
			}
		}

		// Financials object
		var finObj map[string]any
		if rev != nil {
			finObj = map[string]any{
				"revenue": rev,
				"pat":     pat,
				"eps":     eps,
			}
		}

		board, boardSrc := "", ""
		if len(metadata) > 0 {
			var mm map[string]any
			if json.Unmarshal(metadata, &mm) == nil {
				if b, ok := mm["board"].(string); ok {
					board = b
				}
			}
		}
		if board == "" {
			// Older rows predate board capture; derive only where the source identifies the board unambiguously.
			switch source {
			case "NSE_EMERGE":
				board, boardSrc = "SME", "SOURCE"
			case "NSE_OFFICIAL_FILING":
				board, boardSrc = "MAINBOARD", "SOURCE"
			}
		} else {
			boardSrc = "NSE_SERIES"
		}
		var expectedListing *time.Time
		if listingDate == nil && closeDate != nil && (status == "OPEN" || status == "CLOSED_AWAITING_LISTING") {
			expectedListing = ExpectedListingDate(*closeDate)
		}

		var details any
		listingAt, scheduled := "", (*time.Time)(nil)
		if len(detailsRaw) > 0 {
			var dm map[string]any
			if json.Unmarshal(detailsRaw, &dm) == nil {
				details = dm
				if v, ok := dm["listing_at"].(string); ok {
					listingAt = v
				}
				if v, ok := dm["scheduled_listing_date"].(string); ok {
					if ts, err := time.Parse(time.RFC3339, v); err == nil {
						scheduled = &ts
					}
				}
			}
		}
		listingBasis := any(nil)
		switch {
		case listingDate != nil:
			listingBasis = "ANNOUNCED"
		case scheduled != nil:
			expectedListing, listingBasis = scheduled, "SCHEDULED_IN_PROSPECTUS"
		case expectedListing != nil:
			listingBasis = "EXPECTED_SEBI_T_PLUS_3"
		}
		item := map[string]any{
			"listing_at":        nilIfEmpty(listingAt),
			"details":           details,
			"listing_price":     listPrice,
			"listing_day_close": listingDayClose,
			"issue_price": func() *float64 {
				if issuePrice != nil {
					return issuePrice
				}
				return pbHigh // issues are normally allotted at the top of the band until the final price is published
			}(),
			"board":                 nilIfEmpty(board),
			"board_basis":           nilIfEmpty(boardSrc),
			"expected_listing_date": expectedListing,
			"listing_date_basis":    listingBasis,
			"id":                    fmt.Sprintf("%d", id),
			"name":                  compName,
			"symbol":                symbol,
			"exchange":              exchange,
			"sector":                sector,
			"open_date":             openDate,
			"close_date":            closeDate,
			"listing_date":          listingDate,
			"price_band":            priceBandStr,
			"issue_size":            issueSizeStr,
			"issue_size_raw":        issueSize,
			"lot_size":              lotSize,
			"min_investment":        minInv,
			"fresh_issue":           freshIssue,
			"ofs":                   ofs,
			"status":                status,
			"shariah_status":        shariahStatus,
			"subscription":          subObj,
			"gmp":                   gmpObj,
			"prediction":            predObj,
			"financials":            finObj,
			"source":                source,
		}

		// Apply temporal/lifecycle status filter strictly
		if statusFilter != "" && statusFilter != "ALL" && statusFilter != "all" {
			switch strings.ToUpper(statusFilter) {
			case "UPCOMING":
				if status != "UPCOMING" {
					continue
				}
			case "OPEN":
				if status != "OPEN" {
					continue
				}
			case "ACTIVE", "CURRENT":
				if status != "UPCOMING" && status != "OPEN" && status != "CLOSED_AWAITING_LISTING" {
					continue
				}
			case "DRHP", "DRHP_FILED", "PROPOSED":
				if status != "DRHP_FILED" && status != "PROPOSED" {
					continue
				}
			case "HISTORICAL", "HISTORICAL_LISTED", "LISTED", "ARCHIVED":
				if status != "HISTORICAL_LISTED" && status != "LISTED" && status != "ARCHIVED" {
					continue
				}
			default:
				if !strings.EqualFold(status, statusFilter) {
					continue
				}
			}
		}

		results = append(results, item)
	}

	return results, nil
}

// IPOIngestionRun represents a structured ingestion execution log.
type IPOIngestionRun struct {
	ID                        int        `json:"id"`
	Provider                  string     `json:"provider"`
	StartedAt                 time.Time  `json:"started_at"`
	CompletedAt               *time.Time `json:"completed_at"`
	Status                    string     `json:"status"` // SUCCESS, EMPTY, FAILED
	RecordsFetched            int        `json:"records_fetched"`
	RecordsInserted           int        `json:"records_inserted"`
	RecordsUpdated            int        `json:"records_updated"`
	RecordsRejected           int        `json:"records_rejected"`
	LastSuccessfulObservation *time.Time `json:"last_successful_observation"`
	ErrorDetails              string     `json:"error_details"`
	SourceURL                 string     `json:"source_url"`
	DataFreshness             string     `json:"data_freshness"`
}

// RecordIPOIngestionRun records a structured ingestion telemetry run.
func (db *DB) RecordIPOIngestionRun(ctx context.Context, r IPOIngestionRun) (int, error) {
	query := `
		INSERT INTO ipo_ingestion_runs (
			provider, started_at, completed_at, status,
			records_fetched, records_inserted, records_updated, records_rejected,
			last_successful_observation, error_details, source_url, data_freshness
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		r.Provider, r.StartedAt, r.CompletedAt, r.Status,
		r.RecordsFetched, r.RecordsInserted, r.RecordsUpdated, r.RecordsRejected,
		r.LastSuccessfulObservation, r.ErrorDetails, r.SourceURL, r.DataFreshness,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("record IPO ingestion run failed: %w", err)
	}
	return id, nil
}

// ListIPOIngestionRuns retrieves recent ingestion runs.
func (db *DB) ListIPOIngestionRuns(ctx context.Context, limit int) ([]IPOIngestionRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT 
			id, provider, started_at, completed_at, status,
			records_fetched, records_inserted, records_updated, records_rejected,
			last_successful_observation, COALESCE(error_details, ''), COALESCE(source_url, ''), COALESCE(data_freshness, 'UP_TO_DATE')
		FROM ipo_ingestion_runs
		ORDER BY started_at DESC
		LIMIT $1
	`
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list IPO ingestion runs failed: %w", err)
	}
	defer rows.Close()

	var runs []IPOIngestionRun
	for rows.Next() {
		var r IPOIngestionRun
		if err := rows.Scan(
			&r.ID, &r.Provider, &r.StartedAt, &r.CompletedAt, &r.Status,
			&r.RecordsFetched, &r.RecordsInserted, &r.RecordsUpdated, &r.RecordsRejected,
			&r.LastSuccessfulObservation, &r.ErrorDetails, &r.SourceURL, &r.DataFreshness,
		); err != nil {
			return nil, fmt.Errorf("scan IPO ingestion run failed: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, rows.Err()
}

// GetLatestIPOIngestionRun retrieves the most recent ingestion run.
func (db *DB) GetLatestIPOIngestionRun(ctx context.Context) (*IPOIngestionRun, error) {
	runs, err := db.ListIPOIngestionRuns(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return &runs[0], nil
}

// PaperObservation represents a recorded candidate opportunity with predetermined evaluation horizons.
type PaperObservation struct {
	ID                     int              `json:"id"`
	SymbolID               int              `json:"symbol_id"`
	Symbol                 string           `json:"symbol"`
	SetupType              string           `json:"setup_type"`
	CandidateTimestamp     time.Time        `json:"candidate_timestamp"`
	SignalTimestamp        time.Time        `json:"signal_timestamp"`
	DataCutoffAt           time.Time        `json:"data_cutoff_at"`
	Provider               string           `json:"provider"`
	RuleVersion            string           `json:"rule_version"`
	EntryTriggerPrice      float64          `json:"entry_trigger_price"`
	InvalidationPrice      float64          `json:"invalidation_price"`
	TargetPrice            float64          `json:"target_price"`
	RiskRewardRatio        float64          `json:"risk_reward_ratio"`
	ShariahStatus          string           `json:"shariah_status"`
	Sector                 string           `json:"sector"`
	LiquidityBucket        string           `json:"liquidity_bucket"`
	MarketRegime           string           `json:"market_regime"`
	PredeterminedHorizons  map[string]any   `json:"predetermined_horizons"`
	SubsequentObservations []map[string]any `json:"subsequent_observations"`
	Outcome                string           `json:"outcome"`
	Status                 string           `json:"status"`
	StatusReason           *string          `json:"status_reason,omitempty"`
	EstimatedCostsBps      *float64         `json:"estimated_costs_bps"`
	RealizedGrossReturn    *float64         `json:"realized_gross_return,omitempty"`
	RealizedNetReturn      *float64         `json:"realized_net_return,omitempty"`
	IsComplete             bool             `json:"is_complete"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

// CreatePaperObservation inserts a newly observed paper candidate. The decision-time snapshot and its SHA-256 are written
// once, in the INSERT. A duplicate (symbol_id, candidate_timestamp, rule_version) returns the existing id with created=false
// and does not modify the stored row (no updated_at bump). Lifecycle fields are always initialised server-side.
func (db *DB) CreatePaperObservation(ctx context.Context, obs *PaperObservation) (id int, created bool, err error) {
	snap := map[string]any{
		"symbol_id": obs.SymbolID, "symbol": obs.Symbol, "setup_type": obs.SetupType,
		"candidate_timestamp": obs.CandidateTimestamp.UTC().Format(time.RFC3339Nano),
		"signal_timestamp":    obs.SignalTimestamp.UTC().Format(time.RFC3339Nano),
		"data_cutoff_at":      obs.DataCutoffAt.UTC().Format(time.RFC3339Nano),
		"provider":            obs.Provider, "rule_version": obs.RuleVersion, "entry_trigger_price": obs.EntryTriggerPrice,
		"invalidation_price": obs.InvalidationPrice, "target_price": obs.TargetPrice, "shariah_status": obs.ShariahStatus,
		"sector": obs.Sector, "liquidity_bucket": obs.LiquidityBucket, "market_regime": obs.MarketRegime,
	}
	snapJSON, _ := json.Marshal(snap)
	query := `
		INSERT INTO paper_observations (
			symbol_id, symbol, setup_type, candidate_timestamp, signal_timestamp,
			data_cutoff_at, provider, rule_version, entry_trigger_price, invalidation_price,
			target_price, risk_reward_ratio, shariah_status, sector, liquidity_bucket,
			market_regime, decision_snapshot, decision_snapshot_sha256,
			subsequent_observations, status, outcome, is_complete
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			$17::jsonb, encode(sha256(convert_to($17::jsonb::text, 'UTF8')), 'hex'),
			'[]'::jsonb, 'PENDING', 'PENDING', false
		)
		ON CONFLICT (symbol_id, candidate_timestamp, rule_version) DO NOTHING
		RETURNING id
	`
	err = db.QueryRowContext(ctx, query,
		obs.SymbolID, obs.Symbol, obs.SetupType, obs.CandidateTimestamp, obs.SignalTimestamp,
		obs.DataCutoffAt, obs.Provider, obs.RuleVersion, obs.EntryTriggerPrice, obs.InvalidationPrice,
		obs.TargetPrice, obs.RiskRewardRatio, obs.ShariahStatus, obs.Sector, obs.LiquidityBucket,
		obs.MarketRegime, string(snapJSON),
	).Scan(&id)
	if err == sql.ErrNoRows {
		err = db.QueryRowContext(ctx, `SELECT id FROM paper_observations WHERE symbol_id=$1 AND candidate_timestamp=$2 AND rule_version=$3`,
			obs.SymbolID, obs.CandidateTimestamp, obs.RuleVersion).Scan(&id)
		if err != nil {
			return 0, false, fmt.Errorf("lookup of existing paper observation failed: %w", err)
		}
		return id, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("insert paper observation failed: %w", err)
	}
	return id, true, nil
}

// ListPaperObservations retrieves stored paper observations with optional filtering.
func (db *DB) ListPaperObservations(ctx context.Context, setupType string, limit int) ([]PaperObservation, error) {
	if limit <= 0 {
		limit = 100
	}
	query := `
		SELECT 
			id, symbol_id, symbol, setup_type, candidate_timestamp, signal_timestamp,
			data_cutoff_at, provider, rule_version, entry_trigger_price, invalidation_price,
			target_price, risk_reward_ratio, shariah_status, COALESCE(sector, ''),
			COALESCE(liquidity_bucket, ''), COALESCE(market_regime, ''),
			predetermined_horizons, subsequent_observations, outcome, status, status_reason, estimated_costs_bps,
			realized_gross_return, realized_net_return, is_complete, created_at, updated_at
		FROM paper_observations
		WHERE ($1 = '' OR setup_type = $1)
		ORDER BY candidate_timestamp DESC
		LIMIT $2
	`
	rows, err := db.QueryContext(ctx, query, setupType, limit)
	if err != nil {
		return nil, fmt.Errorf("query paper observations failed: %w", err)
	}
	defer rows.Close()

	var list []PaperObservation
	for rows.Next() {
		var o PaperObservation
		var hBytes, sBytes []byte
		if err := rows.Scan(
			&o.ID, &o.SymbolID, &o.Symbol, &o.SetupType, &o.CandidateTimestamp, &o.SignalTimestamp,
			&o.DataCutoffAt, &o.Provider, &o.RuleVersion, &o.EntryTriggerPrice, &o.InvalidationPrice,
			&o.TargetPrice, &o.RiskRewardRatio, &o.ShariahStatus, &o.Sector,
			&o.LiquidityBucket, &o.MarketRegime, &hBytes, &sBytes, &o.Outcome, &o.Status, &o.StatusReason,
			&o.EstimatedCostsBps, &o.RealizedGrossReturn, &o.RealizedNetReturn, &o.IsComplete,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan paper observation failed: %w", err)
		}
		_ = json.Unmarshal(hBytes, &o.PredeterminedHorizons)
		_ = json.Unmarshal(sBytes, &o.SubsequentObservations)
		list = append(list, o)
	}
	return list, rows.Err()
}

// GetPaperObservationSummary reports counts by lifecycle status. Return/win-rate metrics are nil (JSON null) when their
// sample is empty; they are never reported as 0. Returns exist only for COMPLETED trades (target/stop/time-stop).
func (db *DB) GetPaperObservationSummary(ctx context.Context) (map[string]any, error) {
	query := `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'PENDING'),
			COUNT(*) FILTER (WHERE status = 'COMPLETED'),
			COUNT(*) FILTER (WHERE status = 'CENSORED'),
			COUNT(*) FILTER (WHERE status = 'INVALIDATED'),
			COUNT(*) FILTER (WHERE status = 'DATA_UNAVAILABLE'),
			COUNT(*) FILTER (WHERE outcome = 'NOT_TRIGGERED'),
			COUNT(*) FILTER (WHERE outcome IN ('TARGET_HIT','STOPPED_OUT','EXPIRED_TIME_STOP')),
			COUNT(*) FILTER (WHERE outcome = 'TARGET_HIT'),
			COUNT(*) FILTER (WHERE outcome = 'STOPPED_OUT'),
			AVG(realized_net_return) FILTER (WHERE outcome IN ('TARGET_HIT','STOPPED_OUT','EXPIRED_TIME_STOP')),
			AVG(realized_gross_return) FILTER (WHERE outcome IN ('TARGET_HIT','STOPPED_OUT','EXPIRED_TIME_STOP')),
			COUNT(*) FILTER (WHERE outcome IN ('TARGET_HIT','STOPPED_OUT','EXPIRED_TIME_STOP') AND realized_net_return > 0)
		FROM paper_observations
	`
	var total, pending, completed, censored, invalidated, unavailable, notTrig, trades, hits, stops, wins int64
	var avgNet, avgGross sql.NullFloat64
	if err := db.QueryRowContext(ctx, query).Scan(&total, &pending, &completed, &censored, &invalidated, &unavailable,
		&notTrig, &trades, &hits, &stops, &avgNet, &avgGross, &wins); err != nil {
		return nil, err
	}
	nullable := func(v sql.NullFloat64, scale float64) any {
		if !v.Valid {
			return nil
		}
		return v.Float64 * scale
	}
	var winRate any
	if trades > 0 {
		winRate = float64(wins) / float64(trades) * 100.0
	}
	return map[string]any{
		"total_candidates":       total,
		"pending":                pending,
		"completed":              completed,
		"completed_trades":       trades,
		"not_triggered":          notTrig,
		"censored":               censored,
		"invalidated":            invalidated,
		"data_unavailable":       unavailable,
		"target_hits":            hits,
		"stopped_outs":           stops,
		"win_rate_pct":           winRate,
		"avg_net_return_pct":     nullable(avgNet, 100.0),
		"avg_gross_return_pct":   nullable(avgGross, 100.0),
		"net_return_source":      "itemized cost ledger (COST_MODEL_2026_Q4_V2) at fixed paper notional",
		"evaluation_methodology": "Immutable decision-time snapshot; buy-stop entry (no assumed fill); stop wins same-session ambiguity; see docs/P1.7_PREREGISTERED_EVALUATION_SPEC.md",
	}, nil
}

// CreateEODIngestionRun records an initial EOD market data ingestion run.
func (db *DB) CreateEODIngestionRun(ctx context.Context, r *models.EODIngestionRun) (int, error) {
	if r.ReconciliationStatus == "" {
		r.ReconciliationStatus = "UNRECONCILED"
	}
	query := `
		INSERT INTO eod_ingestion_runs (
			run_id, trigger_type, provider, started_at, status, reconciliation_status,
			symbols_attempted, candles_fetched, candles_stored, candles_failed,
			retry_count, is_exchange_verified, error_details, details
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		) RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query,
		r.RunID, r.TriggerType, r.Provider, r.StartedAt, r.Status, r.ReconciliationStatus,
		r.SymbolsAttempted, r.CandlesFetched, r.CandlesStored, r.CandlesFailed,
		r.RetryCount, r.IsExchangeVerified, r.ErrorDetails, r.Details,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create EOD ingestion run failed: %w", err)
	}
	r.ID = id
	return id, nil
}

// UpdateEODIngestionRun updates completion status and metrics for an EOD ingestion run.
func (db *DB) UpdateEODIngestionRun(ctx context.Context, r *models.EODIngestionRun) error {
	if r.ReconciliationStatus == "" {
		r.ReconciliationStatus = "UNRECONCILED"
	}
	query := `
		UPDATE eod_ingestion_runs
		SET completed_at = $1,
		    status = $2,
		    reconciliation_status = $3,
		    symbols_attempted = $4,
		    candles_fetched = $5,
		    candles_stored = $6,
		    candles_failed = $7,
		    retry_count = $8,
		    is_exchange_verified = $9,
		    error_details = $10,
		    details = $11
		WHERE run_id = $12
	`
	_, err := db.ExecContext(ctx, query,
		r.CompletedAt, r.Status, r.ReconciliationStatus,
		r.SymbolsAttempted, r.CandlesFetched, r.CandlesStored, r.CandlesFailed,
		r.RetryCount, r.IsExchangeVerified, r.ErrorDetails, r.Details,
		r.RunID,
	)
	if err != nil {
		return fmt.Errorf("update EOD ingestion run failed: %w", err)
	}
	return nil
}

// ListEODIngestionRuns retrieves recent EOD ingestion runs.
func (db *DB) ListEODIngestionRuns(ctx context.Context, limit int) ([]models.EODIngestionRun, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT id, run_id, trigger_type, provider, started_at, completed_at, status, reconciliation_status,
		       symbols_attempted, candles_fetched, candles_stored, candles_failed,
		       retry_count, is_exchange_verified, COALESCE(error_details, ''), created_at
		FROM eod_ingestion_runs
		ORDER BY started_at DESC
		LIMIT $1
	`
	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list EOD ingestion runs failed: %w", err)
	}
	defer rows.Close()

	var runs []models.EODIngestionRun
	for rows.Next() {
		var r models.EODIngestionRun
		if err := rows.Scan(
			&r.ID, &r.RunID, &r.TriggerType, &r.Provider, &r.StartedAt, &r.CompletedAt, &r.Status, &r.ReconciliationStatus,
			&r.SymbolsAttempted, &r.CandlesFetched, &r.CandlesStored, &r.CandlesFailed,
			&r.RetryCount, &r.IsExchangeVerified, &r.ErrorDetails, &r.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan EOD ingestion run failed: %w", err)
		}
		runs = append(runs, r)
	}
	return runs, nil
}
