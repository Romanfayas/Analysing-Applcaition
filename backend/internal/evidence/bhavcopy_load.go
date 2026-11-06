package evidence

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/halal-equity/backend/internal/providers/corporateactions/official"
	"github.com/halal-equity/backend/internal/providers/exchange/nsebhav"
)

// BhavcopyStats summarises one load.
type BhavcopyStats struct{ Loaded, Skipped, NoSession, Failed int }

// LoadBhavcopies appends NSE's end-of-day files for [from, to] to the evidence tables, skipping days already stored.
// It needs a role that can insert into them (the runtime role cannot). If the exchange refuses the client it stops: it
// never impersonates a browser.
func LoadBhavcopies(ctx context.Context, db *sql.DB, pipe *official.Pipeline, from, to time.Time, weekends bool) (BhavcopyStats, error) {
	var st BhavcopyStats
	tickers := map[string]int{}
	rows, err := db.QueryContext(ctx, `SELECT id, symbol FROM symbols WHERE exchange = 'NSE'`)
	if err != nil {
		return st, err
	}
	for rows.Next() {
		var id int
		var s string
		_ = rows.Scan(&id, &s)
		tickers[s] = id
	}
	rows.Close()

	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if !weekends && (d.Weekday() == time.Saturday || d.Weekday() == time.Sunday) {
			continue
		}
		var exists bool
		_ = db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM exchange_source_files WHERE trade_date = $1)`, d).Scan(&exists)
		if exists {
			st.Skipped++
			continue
		}
		var body []byte
		var meta *official.AcquisitionMetadata
		var docType, srcURL string
		for _, cand := range nsebhav.URLs(d) {
			body, meta, err = pipe.AcquireFromOfficialURL(ctx, cand.URL)
			if err == nil {
				docType, srcURL = cand.DocType, cand.URL
				break
			}
			if errors.Is(err, official.ErrAccessRestricted) {
				return st, fmt.Errorf("exchange refused the client on %s: %w (stopping; no circumvention)", d.Format("2006-01-02"), err)
			}
		}
		if body == nil {
			if meta != nil && meta.HTTPStatus == 404 {
				st.NoSession++ // holiday / no trading session: the exchange publishes no file
			} else {
				st.Failed++
				log.Printf("%s: %v", d.Format("2006-01-02"), err)
			}
			continue
		}
		if meta.TLSCertSHA256 == "" {
			return st, fmt.Errorf("%s: no verified TLS peer certificate recorded; refusing to store as evidence", d.Format("2006-01-02"))
		}
		parsed, total, err := nsebhav.Parse(body, docType, d)
		if err != nil {
			st.Failed++
			log.Printf("%s: parse: %v", d.Format("2006-01-02"), err)
			continue
		}
		// One row per instrument: EQ series preferred; implausible rows are refused, not repaired.
		pick := map[int]nsebhav.Row{}
		for _, r := range parsed {
			id, ok := tickers[r.Ticker]
			if !ok || !r.Plausible() {
				continue
			}
			if prev, seen := pick[id]; seen && prev.Series == "EQ" {
				continue
			}
			if r.Series == "EQ" || r.Series == "BE" || r.Series == "BZ" {
				pick[id] = r
			}
		}
		if err := storeFile(ctx, db, d, docType, srcURL, body, meta, total, pick); err != nil {
			st.Failed++
			log.Printf("%s: store: %v", d.Format("2006-01-02"), err)
			continue
		}
		st.Loaded++
		if st.Loaded%25 == 0 {
			log.Printf("loaded %d files (through %s)", st.Loaded, d.Format("2006-01-02"))
		}
	}
	return st, nil
}

func storeFile(ctx context.Context, db *sql.DB, d time.Time, docType, srcURL string, body []byte,
	meta *official.AcquisitionMetadata, total int, pick map[int]nsebhav.Row) error {
	// A download, an HTTPS connection or a checksum is provenance, NOT proof of authenticity: authenticity
	// proof stays false here and only a human reviewer (or a verified document signature) can supply it.
	val := official.EvaluateArtifactState(body, meta, true, "", true, "", false, false)
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var fileID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO exchange_source_files (trade_date, document_type, issuing_authority, source_url,
		http_status, byte_size, sha256, tls_peer_subject, tls_peer_issuer, tls_cert_sha256, retrieved_at, rows_total, rows_loaded, parser_version)
		VALUES ($1,$2,'NSE_INDIA',$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) RETURNING id`,
		d, docType, srcURL, meta.HTTPStatus, len(body), val.SHA256Checksum, meta.TLSPeerSubject, meta.TLSPeerIssuer,
		meta.TLSCertSHA256, meta.AcquisitionTimeUTC, total, len(pick), nsebhav.ParserVersion).Scan(&fileID); err != nil {
		return err
	}
	for id, r := range pick {
		if _, err := tx.ExecContext(ctx, `INSERT INTO exchange_daily_bars (symbol_id, trade_date, ticker, isin, series, open, high, low,
			close, prev_close, volume, turnover_inr, trades, source_file_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
			id, d, r.Ticker, r.ISIN, r.Series, r.Open, r.High, r.Low, r.Close, r.PrevClose, r.Volume, r.Turnover, r.Trades, fileID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
