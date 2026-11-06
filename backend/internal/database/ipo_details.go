package database

import (
	"context"
	"encoding/json"
	"time"
)

// ActiveIPO is an issue still worth refreshing.
type ActiveIPO struct {
	ID            int
	CompanyName   string
	Symbol        string
	Board         string
	Status        string
	PriceBandHigh *float64
	ListingDate   *time.Time
	ListingPrice  *float64
	HasDetails    bool
}

// IPOsToRefresh returns upcoming, open and unlisted issues, plus those listed in the last `listedWithin`.
func (db *DB) IPOsToRefresh(ctx context.Context, listedWithin time.Duration) ([]ActiveIPO, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT i.id, i.company_name, COALESCE(i.symbol, ''), COALESCE(i.metadata->>'board', ''), i.status, i.price_band_high::float8,
		       i.listing_date, i.listing_price::float8, d.ipo_id IS NOT NULL
		FROM ipos i LEFT JOIN ipo_details d ON d.ipo_id = i.id
		WHERE i.status IN ('UPCOMING', 'OPEN', 'ACTIVE', 'CLOSED_AWAITING_LISTING')
		   OR (i.status = 'LISTED' AND i.listing_date >= $1::date)
		ORDER BY i.open_date DESC NULLS LAST`, time.Now().Add(-listedWithin).Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ActiveIPO
	for rows.Next() {
		var a ActiveIPO
		if err := rows.Scan(&a.ID, &a.CompanyName, &a.Symbol, &a.Board, &a.Status, &a.PriceBandHigh, &a.ListingDate, &a.ListingPrice, &a.HasDetails); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpsertIPODetails stores the merged record of an issue.
func (db *DB) UpsertIPODetails(ctx context.Context, ipoID int, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO ipo_details (ipo_id, data) VALUES ($1, $2::jsonb)
		ON CONFLICT (ipo_id) DO UPDATE SET data = EXCLUDED.data, updated_at = now()`, ipoID, string(raw))
	return err
}

// IPOFacts are the headline figures the issue row carries. Nil leaves a value as it is.
type IPOFacts struct {
	IssueSizeCr, FreshCr, OFSCr, MinInvestment, BandLow, BandHigh *float64
	LotSize                                                       *int
	Sector                                                        *string
}

// ApplyIPOFacts updates the headline figures of an issue.
func (db *DB) ApplyIPOFacts(ctx context.Context, ipoID int, f IPOFacts) error {
	_, err := db.ExecContext(ctx, `UPDATE ipos SET issue_size = COALESCE($2, issue_size), fresh_issue = COALESCE($3, fresh_issue), ofs = COALESCE($4, ofs),
		min_investment = COALESCE($5, min_investment), lot_size = COALESCE($6, lot_size), sector = COALESCE(NULLIF($7, ''), sector),
		price_band_low = COALESCE(price_band_low, $8), price_band_high = COALESCE(price_band_high, $9), updated_at = NOW() WHERE id = $1`,
		ipoID, f.IssueSizeCr, f.FreshCr, f.OFSCr, f.MinInvestment, f.LotSize, f.Sector, f.BandLow, f.BandHigh)
	return err
}

// SetIPOListed records that an issue has listed, with its first day's traded prices.
func (db *DB) SetIPOListed(ctx context.Context, ipoID int, day time.Time, open, closeP float64) error {
	_, err := db.ExecContext(ctx, `UPDATE ipos SET status = 'LISTED', listing_date = COALESCE(listing_date, $2::date), listing_price = $3, listing_day_close = $4, updated_at = NOW() WHERE id = $1`,
		ipoID, day.Format("2006-01-02"), open, closeP)
	return err
}

// RecordIPOGMP appends a reading when it differs from the latest one or that one is over an hour old, building a history.
func (db *DB) RecordIPOGMP(ctx context.Context, ipoID int, value float64, pct *float64, source, url string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO ipo_gmp (ipo_id, gmp_value, gmp_percentage, source, source_url)
		SELECT $1, $2, $3, $4, $5
		WHERE NOT EXISTS (SELECT 1 FROM (SELECT gmp_value, recorded_at FROM ipo_gmp WHERE ipo_id = $1 ORDER BY recorded_at DESC LIMIT 1) l
		                  WHERE l.gmp_value = $2 AND l.recorded_at > now() - interval '1 hour')`, ipoID, value, pct, source, url)
	return err
}

// IPOGMPPoint is one stored GMP reading.
type IPOGMPPoint struct {
	Value      float64   `json:"value"`
	Percentage *float64  `json:"percentage"`
	RecordedAt time.Time `json:"recorded_at"`
}

// IPOGMPHistory returns an issue's GMP readings, oldest first.
func (db *DB) IPOGMPHistory(ctx context.Context, ipoID int) ([]IPOGMPPoint, error) {
	rows, err := db.QueryContext(ctx, `SELECT gmp_value::float8, gmp_percentage::float8, recorded_at FROM ipo_gmp WHERE ipo_id = $1 ORDER BY recorded_at LIMIT 200`, ipoID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IPOGMPPoint{}
	for rows.Next() {
		var p IPOGMPPoint
		if err := rows.Scan(&p.Value, &p.Percentage, &p.RecordedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SweepIPOLifecycle moves issues on from dates alone, so a closed or listed issue never stays "open" because a feed was
// quiet: past its close date it is awaiting listing, and past its listing date it is listed.
func (db *DB) SweepIPOLifecycle(ctx context.Context, today time.Time) (int64, error) {
	d := today.Format("2006-01-02")
	r1, err := db.ExecContext(ctx, `UPDATE ipos SET status = 'CLOSED_AWAITING_LISTING', updated_at = NOW()
		WHERE status IN ('OPEN', 'ACTIVE', 'UPCOMING') AND close_date < $1::date`, d)
	if err != nil {
		return 0, err
	}
	r2, err := db.ExecContext(ctx, `UPDATE ipos SET status = 'LISTED', updated_at = NOW()
		WHERE status = 'CLOSED_AWAITING_LISTING' AND listing_date <= $1::date`, d)
	if err != nil {
		return 0, err
	}
	a, _ := r1.RowsAffected()
	b, _ := r2.RowsAffected()
	return a + b, nil
}
