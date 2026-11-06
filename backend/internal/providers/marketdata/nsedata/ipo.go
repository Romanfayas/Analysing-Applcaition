package nsedata

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParsedIPO is a normalised NSE current-issue record.
type ParsedIPO struct {
	CompanyName   string
	Symbol        string
	OpenDate      *time.Time
	CloseDate     *time.Time
	PriceBandLow  *float64
	PriceBandHigh *float64
	SharesOffered *float64
	// IssueSizeCrore is derived (shares x upper price band) and nil when the band is unknown.
	IssueSizeCrore *float64
	Status         string
	// Board is "MAINBOARD" or "SME" from NSE's series (EQ / SME); empty when NSE gives another series.
	Board string
	// SubscriptionTimes is the total subscription multiple reported by the list feed (nil when absent).
	SubscriptionTimes *float64
	SharesBid         *float64
}

// BoardFromSeries maps NSE's series code to the listing board.
func BoardFromSeries(series string) string {
	switch strings.ToUpper(strings.TrimSpace(series)) {
	case "SME":
		return "SME"
	case "EQ":
		return "MAINBOARD"
	}
	return ""
}

var priceRe = regexp.MustCompile(`\d+(?:,\d{3})*(?:\.\d+)?`)

// nseDateLayout is the date format used by NSE's IPO endpoints, e.g. "02-Nov-2026".
const nseDateLayout = "02-Jan-2006"

func parseNSEDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(nseDateLayout, s)
	if err != nil {
		return nil, fmt.Errorf("unrecognised NSE date %q", s)
	}
	return &t, nil
}

// parsePriceBand reads "Rs.78 to Rs.82" (band) or "Rs.100" (fixed price). Both bounds nil if unparseable.
func parsePriceBand(s string) (lo, hi *float64) {
	var vals []float64
	for _, m := range priceRe.FindAllString(s, -1) {
		if v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64); err == nil && v > 0 {
			vals = append(vals, v)
		}
	}
	switch len(vals) {
	case 0:
		return nil, nil
	case 1:
		return &vals[0], &vals[0]
	default:
		l, h := vals[0], vals[1]
		if l > h {
			l, h = h, l
		}
		return &l, &h
	}
}

// ipoStatus maps NSE's lifecycle word and dates onto the platform's status vocabulary.
func ipoStatus(nseStatus string, open, close *time.Time, now time.Time) string {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	switch strings.ToLower(strings.TrimSpace(nseStatus)) {
	case "active":
		return "OPEN"
	case "closed":
		return "CLOSED_AWAITING_LISTING"
	case "upcoming":
		return "UPCOMING"
	}
	// No (or unknown) status word: fall back to the dates, and to UPCOMING if there are none.
	switch {
	case close != nil && close.Before(today):
		return "CLOSED_AWAITING_LISTING"
	case open != nil && !open.After(today):
		return "OPEN"
	}
	return "UPCOMING"
}

// ParseIPOIssues normalises raw feed records. Records without a company name or symbol are
// returned in rejected rather than stored; duplicates by symbol keep the "Total" category row.
func ParseIPOIssues(raw []IPOData, now time.Time) (ipos []ParsedIPO, rejected []error) {
	seen := map[string]int{}
	for _, r := range raw {
		name, sym := strings.TrimSpace(r.CompanyName), strings.TrimSpace(r.Symbol)
		if name == "" || sym == "" {
			rejected = append(rejected, fmt.Errorf("record missing company name or symbol (symbol=%q)", sym))
			continue
		}
		open, err1 := parseNSEDate(r.IssueStartDate)
		closeD, err2 := parseNSEDate(r.IssueEndDate)
		if err1 != nil || err2 != nil {
			rejected = append(rejected, fmt.Errorf("%s: unparseable issue dates (%q, %q)", sym, r.IssueStartDate, r.IssueEndDate))
			continue
		}
		p := ParsedIPO{CompanyName: name, Symbol: sym, OpenDate: open, CloseDate: closeD}
		p.PriceBandLow, p.PriceBandHigh = parsePriceBand(r.IssuePrice)
		if shares, err := strconv.ParseFloat(strings.TrimSpace(r.IssueSize), 64); err == nil && shares > 0 {
			p.SharesOffered = &shares
			if p.PriceBandHigh != nil {
				crore := shares * *p.PriceBandHigh / 1e7
				p.IssueSizeCrore = &crore
			}
		}
		p.Status = ipoStatus(r.Status, open, closeD, now)
		p.Board = BoardFromSeries(r.Series)
		if v, err := strconv.ParseFloat(strings.TrimSpace(r.NoOfTime), 64); err == nil && v >= 0 {
			p.SubscriptionTimes = &v
		}
		if v, err := strconv.ParseFloat(strings.TrimSpace(r.NoOfSharesBid), 64); err == nil && v >= 0 {
			p.SharesBid = &v
		}

		if i, dup := seen[sym]; dup {
			if strings.EqualFold(r.Category, "Total") {
				ipos[i] = p
			}
			continue
		}
		seen[sym] = len(ipos)
		ipos = append(ipos, p)
	}
	return ipos, rejected
}
