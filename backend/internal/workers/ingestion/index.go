package ingestion

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/halal-equity/backend/internal/providers/exchange/nseindex"
	"github.com/halal-equity/backend/internal/services/market"
)

// indexClientName identifies this client honestly to NSE; it never presents itself as a browser.
const indexClientName = "HalalEquity-Statutory-Evidence-Harvester/1.9.24"

const indexRequestGap = 1200 * time.Millisecond

// IngestIndexBars loads NSE's official index files for every trading day in [from, to] that is not stored yet.
// A 404 means the day has no file yet (holiday, or not published); a 401/403 means NSE refused this client, and the
// run stops instead of working around it.
func (p *Pipeline) IngestIndexBars(ctx context.Context, from, to time.Time) (days, rows int, err error) {
	have, err := p.db.IndexDatesStored(ctx, from, to)
	if err != nil {
		return 0, 0, err
	}
	client := &http.Client{Timeout: 40 * time.Second}
	// Newest first, so the sessions that matter most (and the "latest" level shown in the app) arrive before old history.
	for d := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC); !d.Before(from); d = d.AddDate(0, 0, -1) {
		if ctx.Err() != nil {
			return days, rows, ctx.Err()
		}
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday || have[d.Format("2006-01-02")] {
			continue
		}
		if h, _ := market.IsTradingHoliday(d); h {
			continue
		}
		url := nseindex.URL(d)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		req.Header.Set("User-Agent", indexClientName)
		resp, e := client.Do(req)
		if e != nil {
			log.Printf("[INDEX] %s: %v", d.Format("2006-01-02"), e)
			time.Sleep(indexRequestGap)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			// not published (yet)
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return days, rows, fmt.Errorf("NSE refused this client (HTTP %d); stopping instead of working around it", resp.StatusCode)
		case resp.StatusCode != http.StatusOK:
			log.Printf("[INDEX] %s: HTTP %d", d.Format("2006-01-02"), resp.StatusCode)
		default:
			parsed, perr := nseindex.Parse(bytes.NewReader(body), d)
			if perr != nil || len(parsed) == 0 {
				log.Printf("[INDEX] %s: unusable file (%v, %d rows)", d.Format("2006-01-02"), perr, len(parsed))
				break
			}
			sum := sha256.Sum256(body)
			n, serr := p.db.UpsertIndexBars(ctx, parsed, url, hex.EncodeToString(sum[:]))
			if serr != nil {
				return days, rows, serr
			}
			days++
			rows += n
		}
		time.Sleep(indexRequestGap)
	}
	return days, rows, nil
}
