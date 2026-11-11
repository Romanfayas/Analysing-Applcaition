package database

import (
	"context"
	"time"

	"github.com/halal-equity/backend/internal/services/largedeals"
)

// SaveLargeDeals stores a stock's deals; one already stored (same day, client, side, quantity and price) is left alone. Returns the new rows.
func (db *DB) SaveLargeDeals(ctx context.Context, symbolID int, deals []largedeals.Deal) (int, error) {
	added := 0
	for _, d := range deals {
		res, err := db.ExecContext(ctx, `
			INSERT INTO large_deals (trade_date, symbol_id, kind, client_name, side, quantity, price, value_inr, remarks, client_type)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10)
			ON CONFLICT DO NOTHING`,
			d.Date.Format("2006-01-02"), symbolID, d.Kind, d.Client, d.Side, d.Qty, d.Price, d.Value(), d.Remarks, d.Type)
		if err != nil {
			return added, err
		}
		if n, _ := res.RowsAffected(); n > 0 {
			added++
		}
	}
	return added, nil
}

// LargeDealRow is one stored deal.
type LargeDealRow struct {
	Date       string  `json:"trade_date"`
	Kind       string  `json:"kind"`
	Client     string  `json:"client_name"`
	Side       string  `json:"side"`
	Quantity   int64   `json:"quantity"`
	Price      float64 `json:"price"`
	ValueINR   float64 `json:"value_inr"`
	Remarks    *string `json:"remarks"`
	ClientType string  `json:"client_type"`
}

// ListLargeDeals returns a stock's deals since a date, newest first.
func (db *DB) ListLargeDeals(ctx context.Context, symbolID int, since time.Time, limit int) ([]LargeDealRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT to_char(trade_date, 'YYYY-MM-DD'), kind, client_name, side, quantity, price::float8, value_inr::float8, remarks, client_type
		FROM large_deals WHERE symbol_id = $1 AND trade_date >= $2::date
		ORDER BY trade_date DESC, value_inr DESC LIMIT $3`, symbolID, since.Format("2006-01-02"), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LargeDealRow{}
	for rows.Next() {
		var r LargeDealRow
		if err := rows.Scan(&r.Date, &r.Kind, &r.Client, &r.Side, &r.Quantity, &r.Price, &r.ValueINR, &r.Remarks, &r.ClientType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// LargeDealsFirstDay is the oldest day with stored deals, so the page can say how far back the record goes.
func (db *DB) LargeDealsFirstDay(ctx context.Context) (string, error) {
	var d *string
	err := db.QueryRowContext(ctx, `SELECT to_char(min(trade_date), 'YYYY-MM-DD') FROM large_deals`).Scan(&d)
	if err != nil || d == nil {
		return "", err
	}
	return *d, nil
}
