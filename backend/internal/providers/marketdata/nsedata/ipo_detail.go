package nsedata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Subscription is the demand reported by NSE for an issue. Multiples are nil when NSE does not publish them:
// SME issues report only shares bid and applications per category, never per-category multiples, so for those
// only Total (from the list feed) is known and the category multiples stay nil rather than being derived.
type Subscription struct {
	QIB, NII, Retail, Total *float64
	SharesBid               *float64
}

type bidRow struct {
	Category      string `json:"category"`
	SrNo          string `json:"srNo"`
	NoOfTime      string `json:"noOfTime"`
	NoOfSharesBid string `json:"noOfsharesBid"`
	NoOfShareBid  string `json:"noOfshareBid"` // SME spelling
}

func num(s string) *float64 {
	if v, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil && v >= 0 {
		return &v
	}
	return nil
}

// ParseBidDetails reads /api/ipo-detail. Categories are matched by NSE's serial numbers (1 QIB, 2 NII, 3 retail),
// never by position.
func ParseBidDetails(body []byte) (*Subscription, error) {
	var d struct {
		BidDetails []bidRow `json:"bidDetails"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("nse ipo-detail decode failed: %w", err)
	}
	if len(d.BidDetails) == 0 {
		return nil, fmt.Errorf("nse ipo-detail has no bid details")
	}
	var s Subscription
	for _, r := range d.BidDetails {
		switch {
		case r.SrNo == "1":
			s.QIB = num(r.NoOfTime)
		case r.SrNo == "2":
			s.NII = num(r.NoOfTime)
		case r.SrNo == "3":
			s.Retail = num(r.NoOfTime)
		case strings.EqualFold(r.Category, "Total"):
			s.Total = num(r.NoOfTime)
			if s.SharesBid = num(r.NoOfSharesBid); s.SharesBid == nil {
				s.SharesBid = num(r.NoOfShareBid)
			}
		}
	}
	return &s, nil
}

// PastIssue is one entry of /api/public-past-issues: recently closed or listed issues.
type PastIssue struct {
	Company      string `json:"company"`
	Symbol       string `json:"symbol"`
	SecurityType string `json:"securityType"`
	StartDate    string `json:"ipoStartDate"`
	EndDate      string `json:"ipoEndDate"`
	ListingDate  string `json:"listingDate"`
	PriceRange   string `json:"priceRange"`
	IssuePrice   string `json:"issuePrice"`
}

// ParsedPastIssue is a normalised past issue. ListingDate is nil until the exchange has announced one.
type ParsedPastIssue struct {
	CompanyName   string
	Symbol        string
	Board         string
	OpenDate      *time.Time
	CloseDate     *time.Time
	ListingDate   *time.Time
	PriceBandLow  *float64
	PriceBandHigh *float64
	IssuePrice    *float64
}

// ParsePastIssues keeps mainboard (EQ) and SME issues that started within `window` of now; NSE's feed also lists
// debt and other securities, which are ignored.
func ParsePastIssues(raw []PastIssue, now time.Time, window time.Duration) []ParsedPastIssue {
	var out []ParsedPastIssue
	for _, r := range raw {
		board := BoardFromSeries(r.SecurityType)
		name, sym := strings.TrimSpace(r.Company), strings.TrimSpace(r.Symbol)
		if board == "" || name == "" || sym == "" {
			continue
		}
		open, e1 := pastDate(r.StartDate)
		closeD, e2 := pastDate(r.EndDate)
		if e1 != nil || e2 != nil || open == nil || closeD == nil || now.Sub(*open) > window {
			continue
		}
		p := ParsedPastIssue{CompanyName: name, Symbol: sym, Board: board, OpenDate: open, CloseDate: closeD}
		p.ListingDate, _ = pastDate(r.ListingDate)
		p.PriceBandLow, p.PriceBandHigh = parsePriceBand(r.PriceRange)
		if v := num(strings.TrimSpace(r.IssuePrice)); v != nil && *v > 0 {
			p.IssuePrice = v
		}
		out = append(out, p)
	}
	return out
}

// pastDate parses "30-OCT-2026"; "-" and "" mean "not announced" (nil, nil).
func pastDate(s string) (*time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil, nil
	}
	t, err := time.Parse("02-Jan-2006", strings.Title(strings.ToLower(s)))
	if err != nil {
		return nil, fmt.Errorf("unrecognised date %q", s)
	}
	return &t, nil
}

// GetIPODetail fetches the live bid details for one issue.
func (p *Provider) GetIPODetail(symbol, series string) (*Subscription, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("series", series)
	resp, err := p.doRequest(p.baseURL + "/ipo-detail?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("nse ipo-detail failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nse ipo-detail returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return ParseBidDetails(body)
}

// GetPastIssues fetches recently closed and listed issues.
func (p *Provider) GetPastIssues() ([]PastIssue, error) {
	resp, err := p.doRequest(p.baseURL + "/public-past-issues")
	if err != nil {
		return nil, fmt.Errorf("nse past issues failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nse past issues returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var out []PastIssue
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("nse past issues decode failed: %w", err)
	}
	return out, nil
}

// Category keys used for the subscription book.
const (
	CatQIB    = "QIB"
	CatNII    = "NII"
	CatBHNI   = "BHNI" // non-institutional, bids above Rs 10 lakh
	CatSHNI   = "SHNI" // non-institutional, bids of Rs 2-10 lakh
	CatRetail = "RETAIL"
	CatTotal  = "TOTAL"
)

// CategoryBook is one investor category's demand. Offered and Times are nil when NSE does not publish them, which is the
// case for SME issues: their shares offered must come from another source.
type CategoryBook struct {
	Category string   `json:"category"`
	Label    string   `json:"label"`
	Offered  *float64 `json:"shares_offered"`
	Bid      *float64 `json:"shares_bid"`
	Times    *float64 `json:"times"`
	Basis    string   `json:"basis"` // NSE_CONSOLIDATED | NSE_ONLY | DERIVED
}

// IssueStructure is what NSE's "Issue Size" sentence says about the offer.
type IssueStructure struct {
	FreshShares       *float64 `json:"fresh_shares"`
	OFSShares         *float64 `json:"ofs_shares"`
	FreshAmountCr     *float64 `json:"fresh_amount_cr"`
	OFSAmountCr       *float64 `json:"ofs_amount_cr"`
	AnchorShares      *float64 `json:"anchor_shares"`
	MarketMakerShares *float64 `json:"market_maker_shares"`
}

// IPODetail is the official record of an issue from /api/ipo-detail.
type IPODetail struct {
	Categories    []CategoryBook `json:"categories"`
	Structure     IssueStructure `json:"structure"`
	IssueSizeText string         `json:"issue_size_text,omitempty"`
	LotSize       *int           `json:"lot_size"`
	FaceValue     string         `json:"face_value,omitempty"`
	IssueType     string         `json:"issue_type,omitempty"`
	LeadManagers  string         `json:"lead_managers,omitempty"`
	Registrar     string         `json:"registrar,omitempty"`
	SponsorBank   string         `json:"sponsor_bank,omitempty"`
	ProspectusURL string         `json:"prospectus_url,omitempty"`
	AnchorReport  string         `json:"anchor_report_url,omitempty"`
	Industry      string         `json:"industry,omitempty"`
	BookUpdatedAt string         `json:"book_updated_at,omitempty"`
	Subscription  *Subscription  `json:"-"`
}

var (
	reFreshShares = regexp.MustCompile(`(?i)fresh\s+issue[^.;]*?(?:of\s+)?(?:up\s*to\s+|upto\s+)?([\d,]{5,})\s*(?:equity\s+)?shares`)
	reFreshAmt    = regexp.MustCompile(`(?i)fresh\s+issue[^.;]*?(?:aggregating\s+)?(?:up\s*to\s+|upto\s+)?Rs\.?\s*([\d,.]+)\s*(million|crore|cr|lakh)`)
	reOFSShares   = regexp.MustCompile(`(?i)offer\s+for\s+sale[^.;]*?(?:of\s+)?(?:up\s*to\s+|upto\s+)?([\d,]{5,})\s*(?:equity\s+)?shares`)
	reOFSAmt      = regexp.MustCompile(`(?i)offer\s+for\s+sale[^.;]*?(?:aggregating\s+)?(?:up\s*to\s+|upto\s+)?Rs\.?\s*([\d,.]+)\s*(million|crore|cr|lakh)`)
	reAnchor      = regexp.MustCompile(`(?i)anchor[^.;)]*?([\d,]{5,})\s*equity`)
	reMarketMaker = regexp.MustCompile(`(?i)market\s+maker[^.;)]*?([\d,]{5,})\s*equity`)
	reLot         = regexp.MustCompile(`([\d,]+)\s*equity`)
	reHref        = regexp.MustCompile(`<[^>]+>`)
)

func numCommas(s string) *float64 { return num(strings.ReplaceAll(s, ",", "")) }

func crore(amount, unit string) *float64 {
	v := numCommas(amount)
	if v == nil {
		return nil
	}
	f := *v
	switch strings.ToLower(unit) {
	case "million":
		f /= 10
	case "lakh":
		f /= 100
	}
	return &f
}

func firstNum(re *regexp.Regexp, s string) *float64 {
	if m := re.FindStringSubmatch(s); m != nil {
		return numCommas(m[1])
	}
	return nil
}

// ParseIssueStructure reads NSE's sentence, e.g. "Fresh Issue of up to 23,26,000 equity shares and Offer for Sale of up to
// 5,54,000 Equity Shares (including ... Anchor Allocation of 8,13,600 Equity shares)". An absent part stays nil.
func ParseIssueStructure(text string) IssueStructure {
	var s IssueStructure
	// Keep the anchor and market-maker bracket away from the fresh/OFS share patterns.
	head := text
	if i := strings.Index(head, "("); i >= 0 {
		head = head[:i]
	}
	s.FreshShares, s.OFSShares = firstNum(reFreshShares, head), firstNum(reOFSShares, head)
	if m := reFreshAmt.FindStringSubmatch(head); m != nil {
		s.FreshAmountCr = crore(m[1], m[2])
	}
	if m := reOFSAmt.FindStringSubmatch(head); m != nil {
		s.OFSAmountCr = crore(m[1], m[2])
	}
	s.AnchorShares, s.MarketMakerShares = firstNum(reAnchor, text), firstNum(reMarketMaker, text)
	return s
}

func clean(s string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(reHref.ReplaceAllString(s, "")), `"`))
}

type infoRow struct {
	Title *string `json:"title"`
	Value string  `json:"value"`
}

type activeRow struct {
	Category string  `json:"category"`
	SrNo     *string `json:"srNo"`
	Offered  string  `json:"noOfShareOffered"`
	Bid      string  `json:"noOfSharesBid"`
	Times    string  `json:"noOfTotalMeant"`
}

func positive(v *float64) *float64 {
	if v != nil && *v > 0 {
		return v
	}
	return nil
}

var catBySr = map[string][2]string{
	"1": {CatQIB, "QIB"}, "2": {CatNII, "NII (HNI) total"}, "2.1": {CatBHNI, "bHNI (above Rs 10 lakh)"}, "2.2": {CatSHNI, "sHNI (Rs 2-10 lakh)"}, "3": {CatRetail, "Retail"},
}

// ParseIPODetail reads the whole /api/ipo-detail response. NSE publishes two books: activeCat is consolidated across
// exchanges and is preferred when it carries shares offered; bidDetails is NSE's own window and fills in the rest.
func ParseIPODetail(body []byte) (*IPODetail, error) {
	var d struct {
		MetaInfo struct {
			Industry string `json:"industry"`
		} `json:"metaInfo"`
		IssueInfo struct {
			DataList []infoRow `json:"dataList"`
		} `json:"issueInfo"`
		ActiveCat struct {
			DataList   []activeRow `json:"dataList"`
			UpdateTime string      `json:"updateTime"`
		} `json:"activeCat"`
		BidDetails []bidRow `json:"bidDetails"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("nse ipo-detail decode failed: %w", err)
	}
	out := &IPODetail{Industry: d.MetaInfo.Industry, BookUpdatedAt: strings.TrimPrefix(d.ActiveCat.UpdateTime, "Updated as on ")}
	for _, r := range d.IssueInfo.DataList {
		if r.Title == nil {
			continue
		}
		v := clean(r.Value)
		switch strings.TrimSpace(*r.Title) {
		case "Issue Size":
			out.IssueSizeText, out.Structure = v, ParseIssueStructure(v)
		case "Bid Lot", "Lot Size":
			if m := reLot.FindStringSubmatch(strings.ToLower(v)); m != nil {
				if n := numCommas(m[1]); n != nil {
					lot := int(*n)
					out.LotSize = &lot
				}
			}
		case "Face Value":
			out.FaceValue = v
		case "Issue Type":
			out.IssueType = v
		case "Book Running Lead Managers":
			out.LeadManagers = v
		case "Name of the Registrar":
			out.Registrar = v
		case "Sponsor Bank":
			out.SponsorBank = v
		case "Red Herring Prospectus":
			out.ProspectusURL = v
		case "Anchor Allocation Report":
			out.AnchorReport = v
		}
	}

	books := map[string]*CategoryBook{}
	order := []string{"1", "2", "2.1", "2.2", "3"}
	for _, r := range d.ActiveCat.DataList {
		key := ""
		if r.SrNo != nil {
			key = *r.SrNo
		} else if strings.EqualFold(r.Category, "Total") {
			key = "total"
		}
		off := positive(num(r.Offered))
		if off == nil { // SME: the consolidated table has no shares offered and no multiples
			continue
		}
		if meta, ok := catBySr[key]; ok {
			books[key] = &CategoryBook{Category: meta[0], Label: meta[1], Offered: off, Bid: num(r.Bid), Times: num(r.Times), Basis: "NSE_CONSOLIDATED"}
		} else if key == "total" {
			books[key] = &CategoryBook{Category: CatTotal, Label: "Total", Offered: off, Bid: num(r.Bid), Times: num(r.Times), Basis: "NSE_CONSOLIDATED"}
		}
	}
	for _, r := range d.BidDetails {
		key := r.SrNo
		if strings.EqualFold(r.Category, "Total") {
			key = "total"
		}
		bid := num(r.NoOfSharesBid)
		if bid == nil {
			bid = num(r.NoOfShareBid)
		}
		if _, have := books[key]; have {
			continue
		}
		if meta, ok := catBySr[key]; ok {
			books[key] = &CategoryBook{Category: meta[0], Label: meta[1], Bid: bid, Times: positive(num(r.NoOfTime)), Basis: "NSE_ONLY"}
		} else if key == "total" {
			books[key] = &CategoryBook{Category: CatTotal, Label: "Total", Bid: bid, Times: positive(num(r.NoOfTime)), Basis: "NSE_ONLY"}
		}
	}
	for _, k := range append(order, "total") {
		if b := books[k]; b != nil {
			out.Categories = append(out.Categories, *b)
		}
	}
	sub := &Subscription{}
	for _, c := range out.Categories {
		switch c.Category {
		case CatQIB:
			sub.QIB = c.Times
		case CatNII:
			sub.NII = c.Times
		case CatRetail:
			sub.Retail = c.Times
		case CatTotal:
			sub.Total, sub.SharesBid = c.Times, c.Bid
		}
	}
	out.Subscription = sub
	return out, nil
}

// GetIPOFullDetail fetches the official record of one issue.
func (p *Provider) GetIPOFullDetail(symbol, series string) (*IPODetail, error) {
	q := url.Values{}
	q.Set("symbol", symbol)
	q.Set("series", series)
	resp, err := p.doRequest(p.baseURL + "/ipo-detail?" + q.Encode())
	if err != nil {
		return nil, fmt.Errorf("nse ipo-detail failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("nse ipo-detail returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	return ParseIPODetail(body)
}
