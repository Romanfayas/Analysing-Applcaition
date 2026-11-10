package database

import (
	"context"
	"time"

	"github.com/halal-equity/backend/internal/services/stocknews"
)

// SaveNews stores headlines once each (unique by link) and returns how many were new.
func (db *DB) SaveNews(ctx context.Context, symbolID int, items []stocknews.News) (int, error) {
	n := 0
	for _, it := range items {
		res, err := db.ExecContext(ctx, `INSERT INTO stock_news (symbol_id, source, title, publisher, url, url_sha256, published_at)
			VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6, $7) ON CONFLICT (symbol_id, url_sha256) DO NOTHING`,
			symbolID, it.Source, it.Title, it.Publisher, it.URL, stocknews.Hash(it.URL), it.PublishedAt)
		if err != nil {
			return n, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, nil
}

// SaveFilings stores official announcements once each.
func (db *DB) SaveFilings(ctx context.Context, symbolID int, items []stocknews.Filing) (int, error) {
	n := 0
	for _, it := range items {
		res, err := db.ExecContext(ctx, `INSERT INTO stock_filings (symbol_id, announced_at, category, subject, attachment_url, key_sha256)
			VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), $6) ON CONFLICT (symbol_id, key_sha256) DO NOTHING`,
			symbolID, it.AnnouncedAt, it.Category, it.Subject, it.AttachmentURL, stocknews.Hash(it.AnnouncedAt.Format(time.RFC3339), it.Subject))
		if err != nil {
			return n, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, nil
}

// SaveEvents stores events once each.
func (db *DB) SaveEvents(ctx context.Context, symbolID int, items []stocknews.Event) (int, error) {
	n := 0
	for _, it := range items {
		res, err := db.ExecContext(ctx, `INSERT INTO stock_events (symbol_id, kind, event_date, title, detail, source, key_sha256)
			VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7) ON CONFLICT (symbol_id, key_sha256) DO NOTHING`,
			symbolID, it.Kind, it.Date.In(time.FixedZone("IST", 19800)).Format("2006-01-02"), it.Title, it.Detail, it.Source, stocknews.Hash(it.Kind, it.Date.Format("2006-01-02"), it.Title))
		if err != nil {
			return n, err
		}
		if c, _ := res.RowsAffected(); c > 0 {
			n++
		}
	}
	return n, nil
}

// LogNewsFetch records one fetch attempt, successful or not.
func (db *DB) LogNewsFetch(ctx context.Context, symbolID int, feed string, ok bool, items int, errText string) {
	_, _ = db.ExecContext(ctx, `INSERT INTO news_fetch_log (symbol_id, feed, ok, items, error) VALUES ($1, $2, $3, $4, NULLIF($5, ''))`, symbolID, feed, ok, items, errText)
}

// LastNewsFetch returns the time of the newest SUCCESSFUL fetch of a feed for a symbol (zero if never).
func (db *DB) LastNewsFetch(ctx context.Context, symbolID int, feed string) (time.Time, error) {
	var t *time.Time
	err := db.QueryRowContext(ctx, `SELECT max(fetched_at) FROM news_fetch_log WHERE symbol_id = $1 AND feed = $2 AND ok`, symbolID, feed).Scan(&t)
	if t == nil {
		return time.Time{}, err
	}
	return *t, err
}

// LastNewsError returns the newest failure message for a feed if it is newer than the last success.
func (db *DB) LastNewsError(ctx context.Context, symbolID int, feed string) string {
	var msg string
	_ = db.QueryRowContext(ctx, `SELECT COALESCE(error, '') FROM news_fetch_log l WHERE symbol_id = $1 AND feed = $2 AND NOT ok
		AND fetched_at > COALESCE((SELECT max(fetched_at) FROM news_fetch_log WHERE symbol_id = $1 AND feed = $2 AND ok), 'epoch') ORDER BY fetched_at DESC LIMIT 1`, symbolID, feed).Scan(&msg)
	return msg
}

// NewsRow is a stored headline.
type NewsRow struct {
	Title       string     `json:"title"`
	Publisher   *string    `json:"publisher"`
	URL         string     `json:"url"`
	Source      string     `json:"source"`
	PublishedAt *time.Time `json:"published_at"`
}

// FilingRow is a stored official announcement.
type FilingRow struct {
	AnnouncedAt   time.Time `json:"announced_at"`
	Category      *string   `json:"category"`
	Subject       string    `json:"subject"`
	AttachmentURL *string   `json:"attachment_url"`
}

// EventRow is a stored event.
type EventRow struct {
	Kind      string  `json:"kind"`
	EventDate string  `json:"event_date"`
	Title     string  `json:"title"`
	Detail    *string `json:"detail"`
	Source    string  `json:"source"`
}

// ListNews returns the newest headlines.
func (db *DB) ListNews(ctx context.Context, symbolID, limit int) ([]NewsRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT title, publisher, url, source, published_at FROM stock_news WHERE symbol_id = $1 ORDER BY published_at DESC NULLS LAST, id DESC LIMIT $2`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NewsRow{}
	for rows.Next() {
		var n NewsRow
		if err := rows.Scan(&n.Title, &n.Publisher, &n.URL, &n.Source, &n.PublishedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ListFilings returns the newest official announcements.
func (db *DB) ListFilings(ctx context.Context, symbolID, limit int) ([]FilingRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT announced_at, category, subject, attachment_url FROM stock_filings WHERE symbol_id = $1 ORDER BY announced_at DESC LIMIT $2`, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FilingRow{}
	for rows.Next() {
		var f FilingRow
		if err := rows.Scan(&f.AnnouncedAt, &f.Category, &f.Subject, &f.AttachmentURL); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// ListEvents returns events on or after `from` (upcoming) or before it (recent), nearest first.
func (db *DB) ListEvents(ctx context.Context, symbolID int, upcoming bool, limit int) ([]EventRow, error) {
	q := `SELECT kind, to_char(event_date, 'YYYY-MM-DD'), title, detail, source FROM stock_events WHERE symbol_id = $1 AND event_date >= (now() AT TIME ZONE 'Asia/Kolkata')::date ORDER BY event_date, id LIMIT $2`
	if !upcoming {
		q = `SELECT kind, to_char(event_date, 'YYYY-MM-DD'), title, detail, source FROM stock_events WHERE symbol_id = $1 AND event_date < (now() AT TIME ZONE 'Asia/Kolkata')::date ORDER BY event_date DESC, id LIMIT $2`
	}
	rows, err := db.QueryContext(ctx, q, symbolID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventRow{}
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.Kind, &e.EventDate, &e.Title, &e.Detail, &e.Source); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// NewsDueSymbols returns active symbols ordered by how long ago their headlines were last refreshed (never first), with tracked
// breakout stocks (being watched, or with an open signal) ahead of the rest.
func (db *DB) NewsDueSymbols(ctx context.Context, limit int) ([]SymbolRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.symbol, s.name FROM symbols s
		WHERE s.is_active
		ORDER BY (EXISTS (SELECT 1 FROM breakout_watch w WHERE w.symbol_id = s.id AND w.status = 'WATCHING')
		          OR EXISTS (SELECT 1 FROM breakout_tracking t WHERE t.symbol_id = s.id AND t.resolved_at IS NULL)) DESC,
		         COALESCE((SELECT max(fetched_at) FROM news_fetch_log l WHERE l.symbol_id = s.id AND l.feed = 'NEWS' AND l.ok), 'epoch') ASC, s.symbol
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SymbolRecord
	for rows.Next() {
		var s SymbolRecord
		if err := rows.Scan(&s.ID, &s.Symbol, &s.Name); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
