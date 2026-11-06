// Package nseindex reads NSE's official end-of-day index file (ind_close_all_DDMMYYYY.csv): open, high, low, close,
// volume, turnover, P/E, P/B and dividend yield for every NSE index on one trading day.
package nseindex

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Row is one index on one day. Fields NSE leaves blank or "-" are nil rather than zero.
type Row struct {
	Name         string
	Date         time.Time
	Open, High   *float64
	Low          *float64
	Close        float64
	PointsChange *float64
	Volume       *int64
	TurnoverCr   *float64
	PE, PB, DY   *float64
}

// URL is where NSE publishes the file for a trading day.
func URL(d time.Time) string {
	return fmt.Sprintf("https://nsearchives.nseindia.com/content/indices/ind_close_all_%s.csv", d.Format("02012006"))
}

func f(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

// Parse reads the file by header name (never by position) and keeps rows dated `want`. NSE occasionally serves a
// file for another date, which must not be stored under the requested one.
func Parse(r io.Reader, want time.Time) ([]Row, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	head, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("index file has no header: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\uFEFF")))] = i
	}
	for _, need := range []string{"index name", "index date", "closing index value"} {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("index file is missing column %q", need)
		}
	}
	get := func(rec []string, name string) string {
		if i, ok := col[name]; ok && i < len(rec) {
			return rec[i]
		}
		return ""
	}
	var out []Row
	for {
		rec, err := cr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		d, err := time.Parse("02-01-2006", strings.TrimSpace(get(rec, "index date")))
		if err != nil || !d.Equal(want) {
			continue
		}
		closeV := f(get(rec, "closing index value"))
		name := strings.TrimSpace(get(rec, "index name"))
		if name == "" || closeV == nil || *closeV <= 0 {
			continue
		}
		row := Row{Name: name, Date: d, Close: *closeV,
			Open: f(get(rec, "open index value")), High: f(get(rec, "high index value")), Low: f(get(rec, "low index value")),
			PointsChange: f(get(rec, "points change")), TurnoverCr: f(get(rec, "turnover (rs. cr.)")),
			PE: f(get(rec, "p/e")), PB: f(get(rec, "p/b")), DY: f(get(rec, "div yield"))}
		if v := f(get(rec, "volume")); v != nil {
			iv := int64(*v)
			row.Volume = &iv
		}
		if !row.Plausible() {
			continue
		}
		out = append(out, row)
	}
	return out, nil
}

// Plausible rejects rows whose own numbers contradict each other.
func (r Row) Plausible() bool {
	if r.Close <= 0 {
		return false
	}
	if r.High != nil && r.Low != nil && *r.High < *r.Low {
		return false
	}
	if r.High != nil && r.Close > *r.High*1.0001 {
		return false
	}
	if r.Low != nil && r.Close < *r.Low*0.9999 {
		return false
	}
	return true
}
