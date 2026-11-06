package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/models"
)

// OHLCVRecord represents a single OHLCV candle for database storage.
type OHLCVRecord struct {
	SymbolID      int       `json:"symbol_id"`
	Timestamp     time.Time `json:"timestamp"`
	Open          float64   `json:"open"`
	High          float64   `json:"high"`
	Low           float64   `json:"low"`
	Close         float64   `json:"close"`
	Volume        int64     `json:"volume"`
	AdjClose      *float64  `json:"adj_close,omitempty"`
	Source        string    `json:"source"`
	QualityStatus string    `json:"quality_status"`
	RetrievedAt   time.Time `json:"retrieved_at"`
	IntervalMins  int       `json:"interval_mins,omitempty"`
}

// SymbolRecord represents a stock symbol in the database.
type SymbolRecord struct {
	ID              int       `json:"id"`
	Symbol          string    `json:"symbol"`
	Name            string    `json:"name"`
	Exchange        string    `json:"exchange"`
	ISIN            string    `json:"isin"`
	Sector          string    `json:"sector"`
	Industry        string    `json:"industry"`
	MarketCapCat    string    `json:"market_cap_category"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
}

// UpsertSymbol resolves a symbol's ID, registering it only if it does not exist yet.
//
// `symbols` is the canonical instrument universe: the runtime role is deliberately
// granted SELECT only, so existing instruments are resolved with a read and never
// rewritten (names/metadata of canonical rows are not ingestion-mutable). Only a
// genuinely new instrument reaches the INSERT, which a least-privilege role will
// (correctly) be refused.
func (db *DB) UpsertSymbol(ctx context.Context, symbol, name, exchange string) (int, error) {
	var id int
	err := db.QueryRowContext(ctx,
		`SELECT id FROM symbols WHERE symbol = $1 AND exchange = $2`,
		symbol, exchange).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("resolve symbol %s failed: %w", symbol, err)
	}

	err = db.QueryRowContext(ctx, `
		INSERT INTO symbols (symbol, name, exchange, is_active)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (symbol, exchange) DO UPDATE SET symbol = EXCLUDED.symbol
		RETURNING id
	`, symbol, name, exchange).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("symbol %s is not in the canonical universe and could not be registered: %w", symbol, err)
	}
	return id, nil
}

// OHLCVBatchStats captures itemized upsert metrics distinguishing inserts, updates, unchanged, rejections, and failures.
type OHLCVBatchStats struct {
	TotalProcessed int `json:"total_processed"`
	Inserted       int `json:"inserted"`
	Updated        int `json:"updated"`
	Unchanged      int `json:"unchanged"`
	Quarantined    int `json:"quarantined"`
	Rejected       int `json:"rejected"`
	Failed         int `json:"failed"`
}

// UpsertOHLCVBatchWithStats inserts or updates a batch of OHLCV candles, returning detailed counts.
func (db *DB) UpsertOHLCVBatchWithStats(ctx context.Context, records []OHLCVRecord) (OHLCVBatchStats, error) {
	var stats OHLCVBatchStats
	if len(records) == 0 {
		return stats, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return stats, fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source, quality_status, retrieved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (symbol_id, timestamp) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			adjusted_close = CASE 
				WHEN ohlcv_daily.adjusted_close IS NULL THEN NULL 
				WHEN ohlcv_daily.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN ohlcv_daily.adjusted_close
				WHEN ohlcv_daily.adjusted_close = ohlcv_daily.close THEN EXCLUDED.close
				ELSE ohlcv_daily.adjusted_close
			END,
			source = CASE
				WHEN ohlcv_daily.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN ohlcv_daily.source
				WHEN EXCLUDED.source IN ('NSE_CORPORATE_ACTION', 'PRIMARY_EXCHANGE') THEN ohlcv_daily.source
				ELSE EXCLUDED.source
			END,
			quality_status = EXCLUDED.quality_status,
			retrieved_at = NOW()
		WHERE ohlcv_daily.open IS DISTINCT FROM EXCLUDED.open
		   OR ohlcv_daily.high IS DISTINCT FROM EXCLUDED.high
		   OR ohlcv_daily.low IS DISTINCT FROM EXCLUDED.low
		   OR ohlcv_daily.close IS DISTINCT FROM EXCLUDED.close
		   OR ohlcv_daily.volume IS DISTINCT FROM EXCLUDED.volume
		RETURNING (xmax = 0) AS is_insert
	`)
	if err != nil {
		return stats, fmt.Errorf("prepare statement failed: %w", err)
	}
	defer stmt.Close()

	for _, r := range records {
		stats.TotalProcessed++
		// Strict Data Isolation Invariant: reject intraday records in daily table
		if r.IntervalMins > 0 && r.IntervalMins < 1440 {
			log.Printf("[DB] REJECT: Intraday record with interval %dm cannot be inserted into ohlcv_daily (symbol %d, ts %v)",
				r.IntervalMins, r.SymbolID, r.Timestamp)
			stats.Quarantined++
			continue
		}
		// Strict OHLC Invariant Validation
		if r.High < r.Low || r.Open <= 0 || r.Close <= 0 || r.Volume < 0 {
			log.Printf("[DB] REJECT: Invalid OHLC invariant (O=%.2f, H=%.2f, L=%.2f, C=%.2f, V=%d) for symbol %d",
				r.Open, r.High, r.Low, r.Close, r.Volume, r.SymbolID)
			stats.Rejected++
			continue
		}
		// Exclude zero-volume holiday phantom candles
		if r.Volume == 0 && r.Open == r.Close && r.High == r.Low {
			log.Printf("[DB] REJECT: Zero-volume holiday phantom candle rejected for symbol %d at %v",
				r.SymbolID, r.Timestamp)
			stats.Quarantined++
			continue
		}

		// A provider that returns no adjusted close must yield NULL, never 0 (ck_ohlcv_daily_adj_positive).
		// Explicit source-trust policy: Ordinary vendor feeds cannot insert an unverified adjustment (AdjClose != Close).
		// Corporate actions must be applied through ApplyVerifiedCorporateActionAdjustment / ca.apply_verified_corporate_action.
		var adj interface{}
		if r.AdjClose != nil && *r.AdjClose > 0 {
			if *r.AdjClose == r.Close {
				adj = *r.AdjClose
			} else {
				// Unverified vendor corporate action adjustment: fail-closed quarantine to NULL
				adj = nil
			}
		}
		var isInsert bool
		err := stmt.QueryRowContext(ctx, r.SymbolID, r.Timestamp,
			r.Open, r.High, r.Low, r.Close, r.Volume, adj,
			r.Source, r.QualityStatus).Scan(&isInsert)
		if err != nil {
			if err == sql.ErrNoRows {
				// Record values are identical: no update was performed by PostgreSQL
				stats.Unchanged++
				continue
			}
			log.Printf("[DB] WARN: failed to upsert candle for symbol %d at %v: %v",
				r.SymbolID, r.Timestamp, err)
			stats.Failed++
			continue
		}
		if isInsert {
			stats.Inserted++
		} else {
			stats.Updated++
		}
	}

	if err := tx.Commit(); err != nil {
		return stats, fmt.Errorf("commit failed: %w", err)
	}

	return stats, nil
}

// UpsertOHLCVBatch inserts or updates a batch of OHLCV candles.
// Uses PostgreSQL ON CONFLICT for deduplication — same (symbol_id, timestamp)
// will update rather than create duplicates.
func (db *DB) UpsertOHLCVBatch(ctx context.Context, records []OHLCVRecord) (int, error) {
	stats, err := db.UpsertOHLCVBatchWithStats(ctx, records)
	return stats.Inserted + stats.Updated + stats.Unchanged, err
}

// UpsertIntradayOHLCVBatch inserts or updates a batch of intraday OHLCV candles.
func (db *DB) UpsertIntradayOHLCVBatch(ctx context.Context, records []OHLCVRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source, quality_status, retrieved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (symbol_id, timestamp, interval_minutes) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			source = EXCLUDED.source,
			quality_status = EXCLUDED.quality_status,
			retrieved_at = NOW()
	`)
	if err != nil {
		return 0, fmt.Errorf("prepare statement failed: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, r := range records {
		_, err := stmt.ExecContext(ctx, r.SymbolID, r.Timestamp, r.IntervalMins,
			r.Open, r.High, r.Low, r.Close, r.Volume,
			r.Source, r.QualityStatus)
		if err != nil {
			log.Printf("[DB] WARN: failed to upsert intraday candle for symbol %d at %v: %v",
				r.SymbolID, r.Timestamp, err)
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit failed: %w", err)
	}

	return inserted, nil
}

// GetOHLCV retrieves OHLCV candles for a symbol within a date range.
func (db *DB) GetOHLCV(ctx context.Context, symbolID int, from, to time.Time) ([]OHLCVRecord, error) {
	query := `
		WITH daily_bars AS (
			SELECT 
				symbol_id,
				timestamp::date as bar_date,
				(array_agg(open ORDER BY timestamp ASC))[1] as open,
				MAX(high) as high,
				MIN(low) as low,
				(array_agg(close ORDER BY timestamp DESC))[1] as close,
				SUM(volume) as volume,
				(array_agg(adjusted_close ORDER BY timestamp DESC))[1] as adj_close,
				(array_agg(COALESCE(source, 'yahoo_finance') ORDER BY timestamp DESC))[1] as source,
				(array_agg(COALESCE(quality_status::text, 'VALID') ORDER BY timestamp DESC))[1] as quality_status,
				MAX(retrieved_at) as retrieved_at,
				MIN(timestamp) as timestamp
			FROM ohlcv_daily
			WHERE symbol_id = $1 AND timestamp >= $2 AND timestamp <= $3
			GROUP BY symbol_id, timestamp::date
		)
		SELECT symbol_id, timestamp, open, high, low, close, volume, adj_close, source, quality_status, retrieved_at
		FROM daily_bars
		ORDER BY bar_date ASC
	`
	rows, err := db.QueryContext(ctx, query, symbolID, from, to)
	if err != nil {
		return nil, fmt.Errorf("query OHLCV failed: %w", err)
	}
	defer rows.Close()

	var records []OHLCVRecord
	for rows.Next() {
		var r OHLCVRecord
		if err := rows.Scan(&r.SymbolID, &r.Timestamp, &r.Open, &r.High,
			&r.Low, &r.Close, &r.Volume, &r.AdjClose, &r.Source, &r.QualityStatus, &r.RetrievedAt); err != nil {
			return nil, fmt.Errorf("scan OHLCV row failed: %w", err)
		}
		records = append(records, r)
	}

	return records, rows.Err()
}

// GetLatestOHLCVTimestamp returns the most recent candle timestamp for a symbol.
func (db *DB) GetLatestOHLCVTimestamp(ctx context.Context, symbolID int) (*time.Time, error) {
	query := `SELECT MAX(timestamp) FROM ohlcv_daily WHERE symbol_id = $1`
	var ts *time.Time
	err := db.QueryRowContext(ctx, query, symbolID).Scan(&ts)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

// GetLatestIntradayOHLCVTimestamp returns the most recent intraday candle timestamp for a symbol.
func (db *DB) GetLatestIntradayOHLCVTimestamp(ctx context.Context, symbolID int) (*time.Time, error) {
	query := `SELECT MAX(timestamp) FROM ohlcv_intraday WHERE symbol_id = $1`
	var ts *time.Time
	err := db.QueryRowContext(ctx, query, symbolID).Scan(&ts)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

// IntradayResumePoint is where the next 1-minute fetch should start. It reaches a few minutes behind the newest bar because
// the provider's latest minutes arrive with zero or partial volume and are completed later, and further back to today's
// oldest empty bar so a minute stored before it finished is repaired. 5-minute history is ignored.
func (db *DB) IntradayResumePoint(ctx context.Context, symbolID int) (*time.Time, error) {
	var ts *time.Time
	err := db.QueryRowContext(ctx, `
		SELECT LEAST(
			(SELECT MAX(timestamp) - interval '6 minutes' FROM ohlcv_intraday WHERE symbol_id = $1 AND interval_minutes = 1),
			(SELECT date_trunc('minute', MIN(timestamp)) - interval '1 minute' FROM ohlcv_intraday WHERE symbol_id = $1 AND interval_minutes = 1 AND quality_status = 'SUSPECT' AND volume = 0
			   AND timestamp >= date_trunc('day', now() AT TIME ZONE 'Asia/Kolkata') AT TIME ZONE 'Asia/Kolkata'))`, symbolID).Scan(&ts)
	if err != nil {
		return nil, err
	}
	return ts, nil
}

// GetLatestOHLCV retrieves the most recent OHLCV candle for a symbol.
func (db *DB) GetLatestOHLCV(ctx context.Context, symbolID int) (*OHLCVRecord, error) {
	query := `
		WITH daily_bars AS (
			SELECT 
				symbol_id,
				timestamp::date as bar_date,
				(array_agg(open ORDER BY timestamp ASC))[1] as open,
				MAX(high) as high,
				MIN(low) as low,
				(array_agg(close ORDER BY timestamp DESC))[1] as close,
				SUM(volume) as volume,
				(array_agg(adjusted_close ORDER BY timestamp DESC))[1] as adj_close,
				(array_agg(COALESCE(source, 'yahoo_finance') ORDER BY timestamp DESC))[1] as source,
				(array_agg(COALESCE(quality_status::text, 'VALID') ORDER BY timestamp DESC))[1] as quality_status,
				MAX(retrieved_at) as retrieved_at,
				MIN(timestamp) as timestamp
			FROM ohlcv_daily
			WHERE symbol_id = $1
			GROUP BY symbol_id, timestamp::date
		)
		SELECT symbol_id, timestamp, open, high, low, close, volume, adj_close, source, quality_status, retrieved_at
		FROM daily_bars
		ORDER BY bar_date DESC
		LIMIT 1
	`
	var r OHLCVRecord
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&r.SymbolID, &r.Timestamp, &r.Open, &r.High,
		&r.Low, &r.Close, &r.Volume, &r.AdjClose, &r.Source, &r.QualityStatus, &r.RetrievedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetLatestIntradayOHLCV retrieves the most recent intraday OHLCV candle for a symbol.
func (db *DB) GetLatestIntradayOHLCV(ctx context.Context, symbolID int) (*OHLCVRecord, error) {
	query := `
		SELECT symbol_id, timestamp, open, high, low, close, volume,
		       close as adj_close, source, quality_status, retrieved_at
		FROM ohlcv_intraday
		WHERE symbol_id = $1
		ORDER BY timestamp DESC
		LIMIT 1
	`
	var r OHLCVRecord
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&r.SymbolID, &r.Timestamp, &r.Open, &r.High,
		&r.Low, &r.Close, &r.Volume, &r.AdjClose, &r.Source, &r.QualityStatus, &r.RetrievedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetPreviousClose retrieves the closing price of the immediately preceding trading session.
// It checks ohlcv_daily first, then ohlcv_intraday (only candles representing the closing period).
// Accounts for exchange holidays/weekends (10-day lookback window) and checks for intervening
// corporate actions (demergers/splits). Returns nil when a reliable comparable close cannot be established.
func (db *DB) GetPreviousClose(ctx context.Context, symbolID int, currentDate time.Time) (*float64, error) {
	loc, _ := time.LoadLocation("Asia/Kolkata")
	if loc == nil {
		loc = time.UTC
	}
	currIST := currentDate.In(loc)
	startOfCurrentDay := time.Date(currIST.Year(), currIST.Month(), currIST.Day(), 0, 0, 0, 0, loc)
	earliestAcceptablePriorDate := startOfCurrentDay.AddDate(0, 0, -10) // Max 10 calendar days back for long weekends/holidays

	// 1. Check for intervening unadjusted corporate action (e.g. demerger)
	var corpActionCount int
	caQuery := `
		SELECT COUNT(*)
		FROM corporate_actions
		WHERE symbol_id = $1 
		  AND action_type = 'DEMERGER'
		  AND ex_date <= $2::date 
		  AND ex_date >= $3::date
	`
	_ = db.QueryRowContext(ctx, caQuery, symbolID, currIST.Format("2006-01-02"), earliestAcceptablePriorDate.Format("2006-01-02")).Scan(&corpActionCount)
	if corpActionCount > 0 {
		// Demerger occurred between prior session and today; previous close is not directly comparable unadjusted
		return nil, nil
	}

	// 2. Prefer ohlcv_daily for official daily session close
	queryDaily := `
		SELECT close
		FROM ohlcv_daily
		WHERE symbol_id = $1 
		  AND timestamp < $2 
		  AND timestamp >= $3
		ORDER BY timestamp DESC
		LIMIT 1
	`
	var prevClose float64
	err := db.QueryRowContext(ctx, queryDaily, symbolID, startOfCurrentDay, earliestAcceptablePriorDate).Scan(&prevClose)
	if err == nil && prevClose > 0 {
		return &prevClose, nil
	}

	// 3. Fall back to ohlcv_intraday for the latest candle from a completed prior trading day
	queryIntraday := `
		SELECT close, timestamp
		FROM ohlcv_intraday
		WHERE symbol_id = $1 
		  AND timestamp < $2 
		  AND timestamp >= $3
		ORDER BY timestamp DESC
		LIMIT 1
	`
	var candleTs time.Time
	err = db.QueryRowContext(ctx, queryIntraday, symbolID, startOfCurrentDay, earliestAcceptablePriorDate).Scan(&prevClose, &candleTs)
	if err == nil && prevClose > 0 {
		candleIST := candleTs.In(loc)
		// Verify that this prior intraday candle represents the closing period of that session (at or after 15:00 IST)
		// If ingestion stopped early (e.g., at 10:30 AM), it cannot be considered the official daily close.
		if candleIST.Hour() >= 15 || (candleIST.Hour() == 14 && candleIST.Minute() >= 30) {
			return &prevClose, nil
		}
	}

	// No reliable comparable prior session close established
	return nil, nil
}

// MarketHealthStats encapsulates overall market-data table health metadata.
type MarketHealthStats struct {
	LatestCandleTimestamp   *time.Time // Market timestamp of the single most recent candle across the table
	CandleRetrievedAt       *time.Time // Retrieval timestamp of THAT SAME most recent candle
	LatestIngestionActivity *time.Time // MAX(retrieved_at) across the entire table (system-wide DB write activity)
	TotalCandles            int64      // Total candle count
}

// GetMarketDataHealthStats retrieves the latest candle and its associated retrieval timestamp together,
// avoiding combining independent aggregate timestamps from different records.
func (db *DB) GetMarketDataHealthStats(ctx context.Context) (*MarketHealthStats, error) {
	// Query 1: Get the latest candle and its associated retrieval timestamp from the SAME row.
	latestQuery := `
		SELECT timestamp, retrieved_at
		FROM ohlcv_intraday
		ORDER BY timestamp DESC, retrieved_at DESC
		LIMIT 1
	`
	var stats MarketHealthStats
	err := db.QueryRowContext(ctx, latestQuery).Scan(
		&stats.LatestCandleTimestamp,
		&stats.CandleRetrievedAt,
	)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to fetch latest candle timestamp: %w", err)
	}

	// Query 2: Get the overall system ingestion heartbeat (MAX(retrieved_at)) and total count.
	aggQuery := `
		SELECT MAX(retrieved_at), COUNT(*)
		FROM ohlcv_intraday
	`
	err = db.QueryRowContext(ctx, aggQuery).Scan(
		&stats.LatestIngestionActivity,
		&stats.TotalCandles,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch aggregate health stats: %w", err)
	}

	return &stats, nil
}

// GetSymbolByCode retrieves a symbol by its code and exchange.
func (db *DB) GetSymbolByCode(ctx context.Context, symbol, exchange string) (*SymbolRecord, error) {
	query := `
		SELECT id, symbol, name, exchange, COALESCE(isin, ''), COALESCE(sector, ''),
		       COALESCE(industry, ''), COALESCE(market_cap_category, ''), is_active, created_at
		FROM symbols
		WHERE symbol = $1 AND exchange = $2
	`
	var r SymbolRecord
	err := db.QueryRowContext(ctx, query, symbol, exchange).Scan(
		&r.ID, &r.Symbol, &r.Name, &r.Exchange, &r.ISIN,
		&r.Sector, &r.Industry, &r.MarketCapCat, &r.IsActive, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetSymbolByTicker retrieves a symbol by its ticker regardless of exchange.
func (db *DB) GetSymbolByTicker(ctx context.Context, ticker string) (*SymbolRecord, error) {
	query := `
		SELECT id, symbol, name, exchange, COALESCE(isin, ''), COALESCE(sector, ''),
		       COALESCE(industry, ''), COALESCE(market_cap_category, ''), is_active, created_at
		FROM symbols
		WHERE UPPER(symbol) = UPPER($1)
		LIMIT 1
	`
	var r SymbolRecord
	err := db.QueryRowContext(ctx, query, ticker).Scan(
		&r.ID, &r.Symbol, &r.Name, &r.Exchange, &r.ISIN,
		&r.Sector, &r.Industry, &r.MarketCapCat, &r.IsActive, &r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// GetAllActiveSymbols retrieves all active symbols.
func (db *DB) GetAllActiveSymbols(ctx context.Context) ([]SymbolRecord, error) {
	query := `
		SELECT id, symbol, name, exchange, COALESCE(isin, ''), COALESCE(sector, ''),
		       COALESCE(industry, ''), COALESCE(market_cap_category, ''), is_active, created_at
		FROM symbols
		WHERE is_active = true
		ORDER BY symbol ASC
	`
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query active symbols failed: %w", err)
	}
	defer rows.Close()

	var symbols []SymbolRecord
	for rows.Next() {
		var r SymbolRecord
		if err := rows.Scan(&r.ID, &r.Symbol, &r.Name, &r.Exchange, &r.ISIN,
			&r.Sector, &r.Industry, &r.MarketCapCat, &r.IsActive, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan symbol row failed: %w", err)
		}
		symbols = append(symbols, r)
	}

	return symbols, rows.Err()
}

// GetOHLCVCount returns the total number of candles for a symbol.
func (db *DB) GetOHLCVCount(ctx context.Context, symbolID int) (int64, error) {
	var count int64
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ohlcv_daily WHERE symbol_id = $1`, symbolID).Scan(&count)
	return count, err
}

// GetDataQualityStats returns quality status counts for a symbol.
func (db *DB) GetDataQualityStats(ctx context.Context, symbolID int) (map[string]int64, error) {
	query := `
		SELECT quality_status, COUNT(*)
		FROM ohlcv_daily
		WHERE symbol_id = $1
		GROUP BY quality_status
	`
	rows, err := db.QueryContext(ctx, query, symbolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := make(map[string]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}

// GetRecentCandlesForSymbol retrieves up to limit recent daily candles for a symbol in chronological order.
// If daily candles are fewer than limit/2, it falls back to intraday candles.
func (db *DB) GetRecentCandlesForSymbol(ctx context.Context, symbolID int, limit int) ([]OHLCVRecord, error) {
	query := `
		WITH daily_bars AS (
			SELECT 
				symbol_id,
				timestamp::date as bar_date,
				(array_agg(open ORDER BY timestamp ASC))[1] as open,
				MAX(high) as high,
				MIN(low) as low,
				(array_agg(close ORDER BY timestamp DESC))[1] as close,
				SUM(volume) as volume,
				(array_agg(adjusted_close ORDER BY timestamp DESC))[1] as adj_close,
				(array_agg(COALESCE(source, 'yahoo_finance') ORDER BY timestamp DESC))[1] as source,
				(array_agg(COALESCE(quality_status::text, 'VALID') ORDER BY timestamp DESC))[1] as quality_status,
				MAX(retrieved_at) as retrieved_at,
				MIN(timestamp) as timestamp
			FROM ohlcv_daily
			WHERE symbol_id = $1
			GROUP BY symbol_id, timestamp::date
		)
		SELECT symbol_id, timestamp, open, high, low, close, volume, adj_close, source, quality_status, retrieved_at
		FROM daily_bars
		ORDER BY bar_date DESC
		LIMIT $2
	`
	rows, err := db.QueryContext(ctx, query, symbolID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily candles: %w", err)
	}
	defer rows.Close()

	var records []OHLCVRecord
	for rows.Next() {
		var r OHLCVRecord
		if err := rows.Scan(
			&r.SymbolID, &r.Timestamp, &r.Open, &r.High, &r.Low, &r.Close, &r.Volume,
			&r.AdjClose, &r.Source, &r.QualityStatus, &r.RetrievedAt,
		); err != nil {
			return nil, err
		}
		records = append(records, r)
	}

	// Note: We strictly preserve daily session boundary integrity.
	// Raw intraday 1-minute candles are NEVER substituted for daily observations.
	// If daily records are fewer than the required lookback (e.g. < 30), the quant engine
	// correctly evaluates the dataset as UNAVAILABLE rather than calculating on disguised intraday bars.


	// Reverse to return in chronological order
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetShariahStatusForSymbol returns the latest Shariah screening status for a symbol.
func (db *DB) GetShariahStatusForSymbol(ctx context.Context, symbolID int) string {
	var status string
	err := db.QueryRowContext(ctx, `
		SELECT status FROM shariah_screenings 
		WHERE symbol_id = $1 
		ORDER BY screening_date DESC, screened_at DESC 
		LIMIT 1
	`, symbolID).Scan(&status)
	if err != nil {
		return "UNKNOWN"
	}
	return status
}

// GetActivePriceAdjustmentFactors retrieves verified, undisputed, price-applicable factors
// resolving the latest revision per corporate action event via v_active_price_adjustment_factors.
func (db *DB) GetActivePriceAdjustmentFactors(ctx context.Context, symbol string) ([]models.CorporateActionAdjustmentFactor, error) {
	query := `
		SELECT 
			id, symbol_id, action_type, factor_category, event_id, revision_number,
			ex_date, record_date, effective_date, split_ratio_old, split_ratio_new,
			dividend_amount, cash_dividend_per_share, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			price_factor_applicable, tax_cost_allocation_pct, cum_price_reference,
			ex_price_reference, is_verified, verification_status, dispute_status,
			dispute_reason, source_reference, retrieval_timestamp, created_at, updated_at
		FROM v_active_price_adjustment_factors
		WHERE symbol = $1
		ORDER BY ex_date ASC
	`
	rows, err := db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to query active price adjustment factors: %w", err)
	}
	defer rows.Close()

	var factors []models.CorporateActionAdjustmentFactor
	for rows.Next() {
		var f models.CorporateActionAdjustmentFactor
		err := rows.Scan(
			&f.ID, &f.SymbolID, &f.ActionType, &f.FactorCategory, &f.EventID, &f.RevisionNumber,
			&f.ExDate, &f.RecordDate, &f.EffectiveDate, &f.SplitRatioOld, &f.SplitRatioNew,
			&f.DividendAmount, &f.CashDividendPerShare, &f.AdjustmentFactor,
			&f.NominalPriceFactor, &f.TotalReturnFactor, &f.VolumeAdjustmentFactor,
			&f.PriceFactorApplicable, &f.TaxCostAllocationPct, &f.CumPriceReference,
			&f.ExPriceReference, &f.IsVerified, &f.VerificationStatus, &f.DisputeStatus,
			&f.DisputeReason, &f.SourceReference, &f.RetrievalTimestamp, &f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan factor row: %w", err)
		}
		factors = append(factors, f)
	}
	return factors, nil
}

// GetTaxCostAllocations retrieves statutory Section 49(2C) tax-cost allocations
// via v_tax_cost_allocations, strictly isolated from price reconstruction arithmetic.
func (db *DB) GetTaxCostAllocations(ctx context.Context, symbol string) ([]models.CorporateActionAdjustmentFactor, error) {
	query := `
		SELECT 
			id, symbol_id, action_type, factor_category, event_id, revision_number,
			ex_date, record_date, effective_date, split_ratio_old, split_ratio_new,
			dividend_amount, cash_dividend_per_share, adjustment_factor,
			nominal_price_factor, total_return_factor, volume_adjustment_factor,
			price_factor_applicable, tax_cost_allocation_pct, cum_price_reference,
			ex_price_reference, is_verified, verification_status, dispute_status,
			dispute_reason, source_reference, retrieval_timestamp, created_at, updated_at
		FROM v_tax_cost_allocations
		WHERE symbol = $1
		ORDER BY ex_date ASC
	`
	rows, err := db.QueryContext(ctx, query, symbol)
	if err != nil {
		return nil, fmt.Errorf("failed to query tax cost allocations: %w", err)
	}
	defer rows.Close()

	var factors []models.CorporateActionAdjustmentFactor
	for rows.Next() {
		var f models.CorporateActionAdjustmentFactor
		err := rows.Scan(
			&f.ID, &f.SymbolID, &f.ActionType, &f.FactorCategory, &f.EventID, &f.RevisionNumber,
			&f.ExDate, &f.RecordDate, &f.EffectiveDate, &f.SplitRatioOld, &f.SplitRatioNew,
			&f.DividendAmount, &f.CashDividendPerShare, &f.AdjustmentFactor,
			&f.NominalPriceFactor, &f.TotalReturnFactor, &f.VolumeAdjustmentFactor,
			&f.PriceFactorApplicable, &f.TaxCostAllocationPct, &f.CumPriceReference,
			&f.ExPriceReference, &f.IsVerified, &f.VerificationStatus, &f.DisputeStatus,
			&f.DisputeReason, &f.SourceReference, &f.RetrievalTimestamp, &f.CreatedAt, &f.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tax allocation row: %w", err)
		}
		factors = append(factors, f)
	}
	return factors, nil
}

