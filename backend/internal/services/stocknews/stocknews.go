// Package stocknews fetches and parses what a stock page needs beside prices: news headlines, official exchange announcements (filings),
// and scheduled events (board meetings, dividends, splits).
//
// Sources and what they are:
//   - NSE corporate announcements, event calendar and corporate actions: OFFICIAL exchange records (nseindia.com public API).
//   - Google News RSS: headlines from third-party publishers, aggregated by Google; UNOFFICIAL, may be wrong or promotional, no sentiment is claimed.
//
// Everything is stored with its source and link; nothing is rewritten, scored or summarised.
package stocknews

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ist = time.FixedZone("IST", 5*3600+30*60)

// News is one headline.
type News struct {
	Title, Publisher, URL, Source string
	PublishedAt                   time.Time
}

// Filing is one official exchange announcement.
type Filing struct {
	AnnouncedAt                      time.Time
	Category, Subject, AttachmentURL string
}

// Event is one scheduled or recent company event.
type Event struct {
	Kind, Title, Detail, Source string
	Date                        time.Time
}

// Hash is the stable key used to store an item once.
func Hash(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x1f")))
	return hex.EncodeToString(h[:])
}

func clean(s string) string {
	return strings.TrimSpace(html.UnescapeString(strings.Join(strings.Fields(s), " ")))
}

// ---------------------------------------------------------------------------------------------------- Google News RSS

type rss struct {
	XMLName xml.Name `xml:"rss"` // anything else (an HTML block page, say) is an error, never an empty success
	Items   []struct {
		Title   string `xml:"title"`
		Link    string `xml:"link"`
		PubDate string `xml:"pubDate"`
		Source  string `xml:"source"`
	} `xml:"channel>item"`
}

// ParseGoogleNews reads a Google News RSS document. Google appends " - Publisher" to titles; the separate source element is the publisher.
func ParseGoogleNews(b []byte) ([]News, error) {
	var d rss
	if err := xml.Unmarshal(b, &d); err != nil {
		return nil, fmt.Errorf("google news feed is not valid RSS: %w", err)
	}
	var out []News
	for _, it := range d.Items {
		title, pub := clean(it.Title), clean(it.Source)
		if pub != "" {
			title = strings.TrimSuffix(title, " - "+pub)
		}
		t, err := time.Parse(time.RFC1123, strings.TrimSpace(it.PubDate))
		if err != nil || title == "" || it.Link == "" {
			continue
		}
		out = append(out, News{Title: title, Publisher: pub, URL: strings.TrimSpace(it.Link), Source: "GOOGLE_NEWS", PublishedAt: t.UTC()})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].PublishedAt.After(out[j].PublishedAt) })
	return out, nil
}

// GoogleNewsURL builds the search feed for a company. The query is the company name plus "share" so index and unrelated pages rank lower.
func GoogleNewsURL(company string) string {
	q := url.QueryEscape(company + " share when:30d")
	return "https://news.google.com/rss/search?q=" + q + "&hl=en-IN&gl=IN&ceid=IN:en"
}

// ---------------------------------------------------------------------------------------------------- NSE feeds

func parseNSETime(s string) (time.Time, error) {
	for _, layout := range []string{"02-Jan-2006 15:04:05", "02-Jan-2006", "2006-01-02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, strings.TrimSpace(s), ist); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised NSE date %q", s)
}

// ParseAnnouncements reads NSE corporate announcements (official filings), newest first.
func ParseAnnouncements(b []byte) ([]Filing, error) {
	var raw []struct {
		AnDt string `json:"an_dt"`
		Desc string `json:"desc"`
		Text string `json:"attchmntText"`
		File string `json:"attchmntFile"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("NSE announcements are not a JSON list: %w", err)
	}
	var out []Filing
	for _, r := range raw {
		t, err := parseNSETime(r.AnDt)
		if err != nil || clean(r.Text) == "" {
			continue
		}
		out = append(out, Filing{AnnouncedAt: t, Category: clean(r.Desc), Subject: clean(r.Text), AttachmentURL: strings.TrimSpace(r.File)})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].AnnouncedAt.After(out[j].AnnouncedAt) })
	return out, nil
}

// ParseEvents reads the NSE event calendar (board meetings), keeping events within [from, to].
func ParseEvents(b []byte, from, to time.Time) ([]Event, error) {
	var raw []struct {
		Purpose string `json:"purpose"`
		Desc    string `json:"bm_desc"`
		Date    string `json:"date"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("NSE event calendar is not a JSON list: %w", err)
	}
	var out []Event
	for _, r := range raw {
		t, err := parseNSETime(r.Date)
		if err != nil || t.Before(from) || t.After(to) {
			continue
		}
		out = append(out, Event{Kind: "BOARD_MEETING", Title: clean(r.Purpose), Detail: clean(r.Desc), Source: "NSE_EVENT_CALENDAR", Date: t})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// ParseActions reads NSE corporate actions (dividends, splits, bonus...), keeping those with an ex-date within [from, to].
func ParseActions(b []byte, from, to time.Time) ([]Event, error) {
	var raw []struct {
		Subject string `json:"subject"`
		ExDate  string `json:"exDate"`
		RecDate string `json:"recDate"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("NSE corporate actions are not a JSON list: %w", err)
	}
	var out []Event
	for _, r := range raw {
		t, err := parseNSETime(r.ExDate)
		if err != nil || t.Before(from) || t.After(to) {
			continue
		}
		detail := ""
		if clean(r.RecDate) != "" && clean(r.RecDate) != "-" {
			detail = "Record date " + clean(r.RecDate)
		}
		out = append(out, Event{Kind: "CORPORATE_ACTION", Title: clean(r.Subject), Detail: detail, Source: "NSE_CORPORATE_ACTIONS", Date: t})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

// ---------------------------------------------------------------------------------------------------- fetching

// Fetcher gets the raw documents. It identifies itself honestly and spaces its NSE requests.
type Fetcher struct {
	HTTP      *http.Client
	UserAgent string
	NSEDelay  time.Duration
	last      time.Time
}

// NewFetcher builds a fetcher with sensible limits.
func NewFetcher() *Fetcher {
	return &Fetcher{HTTP: &http.Client{Timeout: 20 * time.Second}, UserAgent: "HalalEquityResearch/1.0 (personal research)", NSEDelay: 1200 * time.Millisecond}
}

func (f *Fetcher) get(ctx context.Context, u string, nse bool) ([]byte, error) {
	if nse {
		if wait := f.NSEDelay - time.Since(f.last); wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		f.last = time.Now()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	req.Header.Set("Accept", "application/json, application/rss+xml, */*")
	resp, err := f.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// NewsFor fetches headlines for a company name.
func (f *Fetcher) NewsFor(ctx context.Context, company string) ([]News, error) {
	b, err := f.get(ctx, GoogleNewsURL(company), false)
	if err != nil {
		return nil, err
	}
	n, err := ParseGoogleNews(b)
	if len(n) > 30 {
		n = n[:30]
	}
	return n, err
}

// FilingsFor fetches NSE announcements for the last 90 days.
func (f *Fetcher) FilingsFor(ctx context.Context, symbol string, now time.Time) ([]Filing, error) {
	u := fmt.Sprintf("https://www.nseindia.com/api/corporate-announcements?index=equities&symbol=%s&from_date=%s&to_date=%s",
		url.QueryEscape(symbol), now.AddDate(0, 0, -90).In(ist).Format("02-01-2006"), now.In(ist).Format("02-01-2006"))
	b, err := f.get(ctx, u, true)
	if err != nil {
		return nil, err
	}
	return ParseAnnouncements(b)
}

// EventsFor fetches board meetings (60 days back to 120 days ahead) and corporate actions (1 year back to 6 months ahead).
func (f *Fetcher) EventsFor(ctx context.Context, symbol string, now time.Time) ([]Event, error) {
	var all []Event
	bm, err := f.get(ctx, "https://www.nseindia.com/api/event-calendar?index=equities&symbol="+url.QueryEscape(symbol), true)
	if err != nil {
		return nil, err
	}
	ev, err := ParseEvents(bm, now.AddDate(0, 0, -60), now.AddDate(0, 0, 120))
	if err != nil {
		return nil, err
	}
	all = append(all, ev...)
	ca, err := f.get(ctx, "https://www.nseindia.com/api/corporates-corporateActions?index=equities&symbol="+url.QueryEscape(symbol), true)
	if err != nil {
		return all, err
	}
	ac, err := ParseActions(ca, now.AddDate(-1, 0, 0), now.AddDate(0, 6, 0))
	if err != nil {
		return all, err
	}
	return append(all, ac...), nil
}
