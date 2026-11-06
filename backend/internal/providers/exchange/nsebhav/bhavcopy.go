// Package nsebhav parses NSE capital-market bhavcopies: the exchange's own end-of-day record.
package nsebhav

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

const (
	DocUDiFF  = "NSE_CM_BHAVCOPY_UDIFF"
	DocLegacy = "NSE_CM_BHAVCOPY_LEGACY"
	// ParserVersion is recorded with every loaded file.
	ParserVersion = "nsebhav-1.0"
)

// UDiFFStart is the first trading day of the UDiFF format; the legacy archive ends before it.
var UDiFFStart = time.Date(2024, 7, 8, 0, 0, 0, 0, time.UTC)

// ErrWrongTradeDate means the file's own trade date differs from the date it was requested for.
var ErrWrongTradeDate = errors.New("bhavcopy trade date does not match the requested date")

// Row is one instrument's end-of-day record as published by the exchange.
type Row struct {
	TradeDate time.Time
	Ticker    string
	Series    string
	ISIN      string
	Open      float64
	High      float64
	Low       float64
	Close     float64
	PrevClose *float64
	Volume    int64
	Turnover  *float64
	Trades    *int64
}

// URLs returns the candidate archive URLs for a date, most authoritative format first.
func URLs(d time.Time) []struct{ URL, DocType string } {
	udiff := struct{ URL, DocType string }{
		fmt.Sprintf("https://nsearchives.nseindia.com/content/cm/BhavCopy_NSE_CM_0_0_0_%s_F_0000.csv.zip", d.Format("20060102")),
		DocUDiFF,
	}
	if !d.Before(UDiFFStart) {
		return []struct{ URL, DocType string }{udiff}
	}
	mon := strings.ToUpper(d.Format("Jan"))
	legacy := struct{ URL, DocType string }{
		fmt.Sprintf("https://nsearchives.nseindia.com/content/historical/EQUITIES/%d/%s/cm%02d%s%dbhav.csv.zip", d.Year(), mon, d.Day(), mon, d.Year()),
		DocLegacy,
	}
	return []struct{ URL, DocType string }{udiff, legacy}
}

// Parse extracts every row of a zipped bhavcopy and verifies it belongs to `want`.
func Parse(zipBytes []byte, docType string, want time.Time) (rows []Row, total int, err error) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, 0, fmt.Errorf("not a zip archive: %w", err)
	}
	if len(zr.File) != 1 {
		return nil, 0, fmt.Errorf("expected exactly one file in archive, found %d", len(zr.File))
	}
	f, err := zr.File[0].Open()
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	// Bhavcopies are a few MB uncompressed; cap to refuse zip bombs.
	data, err := io.ReadAll(io.LimitReader(f, 64<<20))
	if err != nil {
		return nil, 0, err
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, 0, fmt.Errorf("csv: %w", err)
	}
	if len(records) < 2 {
		return nil, 0, errors.New("bhavcopy has no data rows")
	}
	col := map[string]int{}
	for i, h := range records[0] {
		col[strings.TrimSpace(h)] = i
	}

	var names map[string]string
	var dateLayout string
	switch docType {
	case DocUDiFF:
		names = map[string]string{"date": "TradDt", "ticker": "TckrSymb", "series": "SctySrs", "isin": "ISIN", "open": "OpnPric",
			"high": "HghPric", "low": "LwPric", "close": "ClsPric", "prev": "PrvsClsgPric", "vol": "TtlTradgVol",
			"turnover": "TtlTrfVal", "trades": "TtlNbOfTxsExctd"}
		dateLayout = "2006-01-02"
	case DocLegacy:
		names = map[string]string{"date": "TIMESTAMP", "ticker": "SYMBOL", "series": "SERIES", "isin": "ISIN", "open": "OPEN",
			"high": "HIGH", "low": "LOW", "close": "CLOSE", "prev": "PREVCLOSE", "vol": "TOTTRDQTY",
			"turnover": "TOTTRDVAL", "trades": "TOTALTRADES"}
		dateLayout = "02-Jan-2006"
	default:
		return nil, 0, fmt.Errorf("unknown document type %q", docType)
	}
	for _, n := range names {
		if _, ok := col[n]; !ok {
			return nil, 0, fmt.Errorf("bhavcopy is missing column %q", n)
		}
	}

	get := func(rec []string, key string) string {
		i := col[names[key]]
		if i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}
	for _, rec := range records[1:] {
		if len(rec) == 0 || (len(rec) == 1 && rec[0] == "") {
			continue
		}
		total++
		td, err := time.Parse(dateLayout, strings.Title(strings.ToLower(get(rec, "date"))))
		if err != nil {
			return nil, total, fmt.Errorf("row %d: bad trade date %q", total, get(rec, "date"))
		}
		if !td.Equal(time.Date(want.Year(), want.Month(), want.Day(), 0, 0, 0, 0, time.UTC)) {
			return nil, total, fmt.Errorf("%w: file says %s, requested %s", ErrWrongTradeDate, td.Format("2006-01-02"), want.Format("2006-01-02"))
		}
		row := Row{TradeDate: td, Ticker: get(rec, "ticker"), Series: get(rec, "series"), ISIN: get(rec, "isin")}
		var perr error
		num := func(key string) float64 {
			v, err := strconv.ParseFloat(get(rec, key), 64)
			if err != nil && perr == nil {
				perr = fmt.Errorf("row %d (%s): bad %s %q", total, row.Ticker, key, get(rec, key))
			}
			return v
		}
		row.Open, row.High, row.Low, row.Close = num("open"), num("high"), num("low"), num("close")
		vol := num("vol")
		if perr != nil {
			return nil, total, perr
		}
		row.Volume = int64(vol)
		if v, err := strconv.ParseFloat(get(rec, "prev"), 64); err == nil {
			row.PrevClose = &v
		}
		if v, err := strconv.ParseFloat(get(rec, "turnover"), 64); err == nil {
			row.Turnover = &v
		}
		if v, err := strconv.ParseInt(get(rec, "trades"), 10, 64); err == nil {
			row.Trades = &v
		}
		rows = append(rows, row)
	}
	return rows, total, nil
}

// Plausible reports whether an equity row is internally consistent enough to store as evidence.
func (r Row) Plausible() bool {
	return r.Open > 0 && r.Low > 0 && r.Close > 0 && r.High >= r.Low &&
		r.High >= r.Open && r.High >= r.Close && r.Low <= r.Open && r.Low <= r.Close && r.Volume >= 0
}
