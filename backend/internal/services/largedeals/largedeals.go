// Package largedeals reads the official NSE bulk-deal and block-deal files. They name the client behind every large trade, which is the only
// public evidence of who bought: 1-minute price bars carry total volume and never the identity of a buyer.
package largedeals

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	BulkURL  = "https://nsearchives.nseindia.com/content/equities/bulk.csv"
	BlockURL = "https://nsearchives.nseindia.com/content/equities/block.csv"
)

// Deal is one row of an NSE bulk or block deal file.
type Deal struct {
	Date    time.Time
	Symbol  string
	Client  string
	Side    string // BUY | SELL
	Qty     int64
	Price   float64
	Kind    string // BULK | BLOCK
	Remarks string
	Type    string // name-based guess, see Classify
}

// Value is the traded value in rupees.
func (d Deal) Value() float64 { return float64(d.Qty) * d.Price }

// Client types. They are a guess from the registered name, nothing more.
const (
	TypeMutualFund = "MUTUAL_FUND"
	TypeInsurance  = "INSURANCE"
	TypeForeign    = "FOREIGN_INVESTOR"
	TypeBank       = "BANK_OR_FINANCIAL"
	TypeOther      = "OTHER"
)

// IsInstitution says whether a client type is one the user would call institutional (mutual funds, insurers, foreign funds).
func IsInstitution(t string) bool {
	return t == TypeMutualFund || t == TypeInsurance || t == TypeForeign
}

// Classify guesses the kind of investor from the client's registered name. Family trusts, partnerships and private companies stay OTHER:
// a name can suggest an institution, it cannot prove one.
func Classify(name string) string {
	n := " " + strings.ToUpper(strings.Join(strings.Fields(name), " ")) + " "
	has := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(n, w) {
				return true
			}
		}
		return false
	}
	switch {
	case has(" MUTUAL FUND", " MF ", " ASSET MANAGEMENT", " AMC ", " TRUSTEE "):
		return TypeMutualFund
	case has(" INSURANCE", " ASSURANCE", " LIFE ", " PENSION"):
		return TypeInsurance
	case has(" FPI ", " FOREIGN ", " PTE ", " LLC", " L.L.C", " S.A.", " SICAV", " MAURITIUS", " SINGAPORE", " CAYMAN", " IRELAND", " LUXEMBOURG", " GLOBAL ", " INTERNATIONAL ", " MASTER FUND", " OFFSHORE", " EMERGING MARKETS", " INVESTMENT FUND", " FUND "):
		return TypeForeign
	case has(" BANK", " SECURITIES ", " FINANCE", " FINANCIAL", " BROKING", " CAPITAL MARKET"):
		return TypeBank
	}
	return TypeOther
}

// ParseCSV reads one NSE deal file. Rows that cannot be read are skipped and counted, never guessed.
func ParseCSV(r io.Reader, kind string) (deals []Deal, skipped int, err error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	head, err := cr.Read()
	if err != nil {
		return nil, 0, fmt.Errorf("deal file is empty: %w", err)
	}
	col := map[string]int{}
	for i, h := range head {
		h = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))
		switch {
		case h == "date":
			col["date"] = i
		case h == "symbol":
			col["symbol"] = i
		case h == "client name":
			col["client"] = i
		case strings.HasPrefix(h, "buy/sell"):
			col["side"] = i
		case strings.HasPrefix(h, "quantity"):
			col["qty"] = i
		case strings.HasPrefix(h, "trade price"):
			col["price"] = i
		case h == "remarks":
			col["remarks"] = i
		}
	}
	for _, k := range []string{"date", "symbol", "client", "side", "qty", "price"} {
		if _, ok := col[k]; !ok {
			return nil, 0, errors.New("not an NSE deal file: missing column " + k)
		}
	}
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			skipped++
			continue
		}
		get := func(k string) string {
			i, ok := col[k]
			if !ok || i >= len(rec) {
				return ""
			}
			return strings.TrimSpace(rec[i])
		}
		day, derr := time.Parse("02-Jan-2006", get("date"))
		qty, qerr := strconv.ParseInt(strings.ReplaceAll(get("qty"), ",", ""), 10, 64)
		price, perr := strconv.ParseFloat(strings.ReplaceAll(get("price"), ",", ""), 64)
		side := strings.ToUpper(get("side"))
		if side == "B" {
			side = "BUY"
		} else if side == "S" {
			side = "SELL"
		}
		if derr != nil || qerr != nil || perr != nil || qty <= 0 || price <= 0 || get("symbol") == "" || get("client") == "" || (side != "BUY" && side != "SELL") {
			skipped++
			continue
		}
		deals = append(deals, Deal{Date: day, Symbol: strings.ToUpper(get("symbol")), Client: get("client"), Side: side, Qty: qty, Price: price,
			Kind: kind, Remarks: strings.Trim(get("remarks"), "- "), Type: Classify(get("client"))})
	}
	return deals, skipped, nil
}

// Fetch downloads and parses one file. An HTML block page is an error, never an empty day.
func Fetch(ctx context.Context, hc *http.Client, url, kind string) ([]Deal, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	req.Header.Set("Accept", "text/csv,*/*")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("NSE returned HTTP %d", resp.StatusCode)
	}
	return ParseCSV(io.LimitReader(resp.Body, 20<<20), kind)
}
