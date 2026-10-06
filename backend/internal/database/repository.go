package database

import (
	"context"
	"fmt"
	"log"
	"time"
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
	AdjClose      float64   `json:"adj_close"`
	Source        string    `json:"source"`
	QualityStatus string    `json:"quality_status"`
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

// UpsertSymbol inserts or updates a symbol and returns its ID.
func (db *DB) UpsertSymbol(ctx context.Context, symbol, name, exchange string) (int, error) {
	query := `
		INSERT INTO symbols (symbol, name, exchange, is_active)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (symbol, exchange) DO UPDATE
			SET name = EXCLUDED.name, updated_at = NOW()
		RETURNING id
	`
	var id int
	err := db.QueryRowContext(ctx, query, symbol, name, exchange).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert symbol %s failed: %w", symbol, err)
	}
	return id, nil
}

// UpsertOHLCVBatch inserts or updates a batch of OHLCV candles.
// Uses PostgreSQL ON CONFLICT for deduplication — same (symbol_id, timestamp)
// will update rather than create duplicates.
func (db *DB) UpsertOHLCVBatch(ctx context.Context, records []OHLCVRecord) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin transaction failed: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source, quality_status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (symbol_id, timestamp) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			adjusted_close = EXCLUDED.adjusted_close,
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
		_, err := stmt.ExecContext(ctx, r.SymbolID, r.Timestamp,
			r.Open, r.High, r.Low, r.Close, r.Volume, r.AdjClose,
			r.Source, r.QualityStatus)
		if err != nil {
			log.Printf("[DB] WARN: failed to upsert candle for symbol %d at %v: %v",
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
		SELECT symbol_id, timestamp, open, high, low, close, volume,
		       COALESCE(adj_close, close) as adj_close, source, quality_status
		FROM ohlcv_daily
		WHERE symbol_id = $1 AND timestamp >= $2 AND timestamp <= $3
		ORDER BY timestamp ASC
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
			&r.Low, &r.Close, &r.Volume, &r.AdjClose, &r.Source, &r.QualityStatus); err != nil {
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

// GetLatestOHLCV retrieves the most recent OHLCV candle for a symbol.
func (db *DB) GetLatestOHLCV(ctx context.Context, symbolID int) (*OHLCVRecord, error) {
	query := `
		SELECT symbol_id, timestamp, open, high, low, close, volume,
		       COALESCE(adj_close, close) as adj_close, source, quality_status
		FROM ohlcv_daily
		WHERE symbol_id = $1
		ORDER BY timestamp DESC
		LIMIT 1
	`
	var r OHLCVRecord
	err := db.QueryRowContext(ctx, query, symbolID).Scan(
		&r.SymbolID, &r.Timestamp, &r.Open, &r.High,
		&r.Low, &r.Close, &r.Volume, &r.AdjClose, &r.Source, &r.QualityStatus)
	if err != nil {
		return nil, err
	}
	return &r, nil
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
