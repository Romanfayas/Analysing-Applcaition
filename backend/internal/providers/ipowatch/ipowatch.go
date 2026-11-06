// Package ipowatch reads two public pages of ipowatch.in: the grey-market premium list and an issue's detail page.
//
// GMP has no official source: it is an unofficial, unregulated figure and is always presented as such. Everything else
// here is secondary to NSE's own record and only fills what NSE does not publish (chiefly the shares offered per category
// on SME issues, lot tiers and the issue timetable). The site's robots.txt permits these pages; requests are slow, carry
// an honest client name, and the source is credited wherever the data is shown.
package ipowatch

import (
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	GMPURL     = "https://ipowatch.in/ipo-grey-market-premium-latest-ipo-gmp/"
	SourceName = "IPOWatch (unofficial)"
	clientName = "HalalEquity-Research/1.9.27"
	// RequestGap is the minimum wait between requests to the site.
	RequestGap = 2 * time.Second
)

// GMPRow is one line of the grey-market list.
type GMPRow struct {
	Name       string
	Status     string // Open, Closed, Upcoming, as the site words it
	GMP        *float64
	Price      *float64
	GainPct    *float64
	DetailURL  string
	GMPPageURL string
}

// LotTier is a minimum/maximum application for an investor type.
type LotTier struct {
	Application string `json:"application"`
	Lots        string `json:"lots"`
	Shares      string `json:"shares"`
	Amount      string `json:"amount"`
}

// Reservation is the shares set aside for an investor category.
type Reservation struct {
	Category string   `json:"category"`
	Shares   *float64 `json:"shares"`
	Percent  *float64 `json:"percent"`
}

// Detail is what an issue's page publishes.
type Detail struct {
	URL          string              `json:"url"`
	Listing      string              `json:"listing_at,omitempty"`
	PriceBand    string              `json:"price_band,omitempty"`
	IssueSize    string              `json:"issue_size,omitempty"`
	IssueSizeCr  *float64            `json:"issue_size_cr"`
	FreshIssue   string              `json:"fresh_issue,omitempty"`
	FreshCr      *float64            `json:"fresh_issue_cr"`
	OFS          string              `json:"ofs,omitempty"`
	OFSCr        *float64            `json:"ofs_cr"`
	LotTiers     []LotTier           `json:"lot_tiers,omitempty"`
	Reservations []Reservation       `json:"reservations,omitempty"`
	Anchor       map[string]string   `json:"anchor,omitempty"`
	Timeline     map[string]string   `json:"timeline,omitempty"`
	ListingDate  *time.Time          `json:"listing_date,omitempty"`
	Shareholding []map[string]string `json:"shareholding,omitempty"`
	Objects      []map[string]string `json:"objects,omitempty"`
	Financials   []map[string]string `json:"financials,omitempty"`
	KPIs         map[string]string   `json:"kpis,omitempty"`
}

var (
	reTable   = regexp.MustCompile(`(?is)<table.*?</table>`)
	reRow     = regexp.MustCompile(`(?is)<tr.*?</tr>`)
	reCell    = regexp.MustCompile(`(?is)<t[dh][^>]*>(.*?)</t[dh]>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	reHref    = regexp.MustCompile(`(?i)href="([^"]+)"`)
	reGMPSlug = regexp.MustCompile(`-ipo-gmp[a-z-]*/?$`)
	reBand    = regexp.MustCompile(`₹\s*([\d,.]+)\s*to\s*₹\s*([\d,.]+)`)
	reNum     = regexp.MustCompile(`-?[\d,]+(?:\.\d+)?`)
	rePct     = regexp.MustCompile(`\(?(-?[\d.]+)\s*%`)
	reSpace   = regexp.MustCompile(`\s+`)
	reCrore   = regexp.MustCompile(`(?i)([\d,]+(?:\.\d+)?)\s*Cr`)
)

func text(s string) string {
	s = strings.ReplaceAll(s, "<br>", " ")
	return strings.TrimSpace(reSpace.ReplaceAllString(html.UnescapeString(reTag.ReplaceAllString(s, " ")), " "))
}

func number(s string) *float64 {
	m := reNum.FindString(s)
	if m == "" {
		return nil
	}
	v, err := strconv.ParseFloat(strings.ReplaceAll(m, ",", ""), 64)
	if err != nil {
		return nil
	}
	return &v
}

func crore(s string) *float64 {
	m := reCrore.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	return number(m[1])
}

func tables(page string) [][][]string {
	var out [][][]string
	for _, t := range reTable.FindAllString(page, -1) {
		var rows [][]string
		for _, r := range reRow.FindAllString(t, -1) {
			var cells []string
			for _, c := range reCell.FindAllStringSubmatch(r, -1) {
				cells = append(cells, text(c[1]))
			}
			if len(cells) > 0 {
				rows = append(rows, cells)
			}
		}
		if len(rows) > 0 {
			out = append(out, rows)
		}
	}
	return out
}

func absolute(href string) string {
	if strings.HasPrefix(href, "/") {
		return "https://ipowatch.in" + href
	}
	return href
}

// DetailURLFrom turns a GMP page link ("x-ipo-gmp-today/", "x-ipo-gmp-grey-market-premium/") into the issue's own page.
func DetailURLFrom(gmpURL string) string { return reGMPSlug.ReplaceAllString(gmpURL, "-ipo/") }

// ParseBand reads "₹88 to ₹94 Per Share".
func ParseBand(s string) (low, high *float64) {
	if m := reBand.FindStringSubmatch(s); m != nil {
		return number(m[1]), number(m[2])
	}
	return nil, nil
}

// ParseGMP reads every "Company / GMP" table of the list page.
func ParseGMP(page string) []GMPRow {
	var out []GMPRow
	for _, t := range reTable.FindAllString(page, -1) {
		rows := reRow.FindAllString(t, -1)
		if len(rows) < 2 {
			continue
		}
		head := reCell.FindAllStringSubmatch(rows[0], -1)
		if len(head) < 5 || !strings.EqualFold(text(head[0][1]), "Company") || !strings.HasPrefix(strings.ToUpper(text(head[1][1])), "GMP") {
			continue
		}
		for _, r := range rows[1:] {
			c := reCell.FindAllStringSubmatch(r, -1)
			if len(c) < 5 {
				continue
			}
			name := text(c[0][1])
			row := GMPRow{GMP: number(text(c[1][1])), Price: number(text(c[3][1]))}
			for _, st := range []string{"Closed", "Open", "Upcoming", "Listed"} {
				if strings.HasSuffix(name, " "+st) {
					row.Status, name = st, strings.TrimSpace(strings.TrimSuffix(name, st))
					break
				}
			}
			row.Name = name
			if m := rePct.FindStringSubmatch(text(c[4][1])); m != nil {
				if v, err := strconv.ParseFloat(m[1], 64); err == nil {
					row.GainPct = &v
				}
			}
			if m := reHref.FindStringSubmatch(c[0][1]); m != nil {
				row.GMPPageURL = absolute(m[1])
				row.DetailURL = DetailURLFrom(row.GMPPageURL)
			}
			if row.Name != "" {
				out = append(out, row)
			}
		}
	}
	return out
}

func kv(rows [][]string) map[string]string {
	m := map[string]string{}
	for _, r := range rows {
		if len(r) >= 2 {
			m[strings.TrimRight(strings.TrimSpace(r[0]), ":")] = r[1]
		}
	}
	return m
}

func records(rows [][]string) []map[string]string {
	var out []map[string]string
	for _, r := range rows[1:] {
		rec := map[string]string{}
		for i, h := range rows[0] {
			if i < len(r) {
				rec[h] = r[i]
			}
		}
		out = append(out, rec)
	}
	return out
}

// usable hides the site's "[.]" placeholder for a value it has not published.
func usable(s string) string {
	if strings.Contains(s, "[.]") || strings.TrimSpace(s) == "" {
		return ""
	}
	return s
}

// ParseDetail recognises each table by its content, never by its position on the page.
func ParseDetail(page, url string) *Detail {
	d := &Detail{URL: url}
	for _, t := range tables(page) {
		first := strings.ToLower(t[0][0])
		m := kv(t)
		switch {
		case m["IPO Price Band"] != "":
			d.PriceBand, d.Listing = m["IPO Price Band"], m["IPO Listing"]
			d.IssueSize = usable(m["Issue Size"])
			d.IssueSizeCr = crore(d.IssueSize)
			d.FreshIssue = usable(m["Fresh Issue"])
			d.FreshCr = crore(d.FreshIssue)
			d.OFS = usable(m["Offer for Sale"])
			d.OFSCr = crore(d.OFS)
		case first == "application":
			for _, r := range t[1:] {
				if len(r) >= 4 {
					d.LotTiers = append(d.LotTiers, LotTier{r[0], r[1], r[2], r[3]})
				}
			}
		case first == "investor category":
			for _, r := range t[1:] {
				if len(r) >= 2 {
					res := Reservation{Category: r[0], Shares: number(r[1])}
					if len(r) >= 3 {
						res.Percent = number(r[2])
					}
					d.Reservations = append(d.Reservations, res)
				}
			}
		case strings.HasPrefix(first, "anchor bidding"):
			d.Anchor = m
		case m["Basis of Allotment"] != "":
			d.Timeline = m
			if ts, err := time.Parse("January 2, 2006", m["IPO Listing Date"]); err == nil {
				d.ListingDate = &ts
			}
		case first == "particular":
			d.Shareholding = records(t)
		case first == "purpose":
			d.Objects = records(t)
		case first == "period ended":
			d.Financials = records(t)
		case first == "kpi":
			d.KPIs = m
		}
	}
	return d
}

// Client fetches the pages.
type Client struct {
	http *http.Client
}

// NewClient returns a client with a sensible timeout.
func NewClient() *Client { return &Client{http: &http.Client{Timeout: 30 * time.Second}} }

func (c *Client) get(url string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", clientName)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ipowatch returned HTTP %d for %s", resp.StatusCode, url)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

// FetchGMP returns the current grey-market list.
func (c *Client) FetchGMP() ([]GMPRow, error) {
	page, err := c.get(GMPURL)
	if err != nil {
		return nil, err
	}
	return ParseGMP(page), nil
}

// FetchDetail returns an issue's page.
func (c *Client) FetchDetail(url string) (*Detail, error) {
	page, err := c.get(url)
	if err != nil {
		return nil, err
	}
	return ParseDetail(page, url), nil
}

var reSuffix = regexp.MustCompile(`(?i)\b(limited|ltd|pvt|private|the|and|&|india|ipo)\b`)
var reAlnum = regexp.MustCompile(`[^a-z0-9 ]+`)

// NormalizeName reduces a company name to the words that identify it, so "HGS (India) Limited" and "HGS (India)" match.
func NormalizeName(s string) string {
	s = reAlnum.ReplaceAllString(strings.ToLower(s), " ")
	return strings.Join(strings.Fields(reSuffix.ReplaceAllString(s, " ")), " ")
}

// SameCompany is true when one normalised name equals or begins with the other on a word boundary. The site often shortens
// names ("Maxwell Engineering" for "Maxwell Engineering Solutions Limited").
func SameCompany(a, b string) bool {
	x, y := NormalizeName(a), NormalizeName(b)
	if x == "" || y == "" {
		return false
	}
	if len(x) > len(y) {
		x, y = y, x
	}
	return x == y || (len(strings.Fields(x)) >= 2 && strings.HasPrefix(y, x+" "))
}
