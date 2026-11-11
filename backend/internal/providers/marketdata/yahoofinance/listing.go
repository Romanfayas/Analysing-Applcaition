package yahoofinance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ListingQuote is one search hit for a company name.
type ListingQuote struct {
	Symbol, Name, Exchange string
}

// SearchListings looks a company up by name and returns its NSE (.NS) and BSE (.BO) equity tickers. Yahoo sometimes carries
// only a long name or none at all for a fresh SME listing, so both name fields are returned.
func (p *Provider) SearchListings(company string) ([]ListingQuote, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v1/finance/search?q=%s&quotesCount=8&newsCount=0&enableFuzzyQuery=false", url.QueryEscape(company))
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo search failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo search returned %d", resp.StatusCode)
	}
	var body struct {
		Quotes []struct {
			Symbol    string `json:"symbol"`
			ShortName string `json:"shortname"`
			LongName  string `json:"longname"`
			Exchange  string `json:"exchange"`
			Type      string `json:"quoteType"`
		} `json:"quotes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("yahoo search decode: %w", err)
	}
	var out []ListingQuote
	for _, q := range body.Quotes {
		if q.Type != "EQUITY" || !(strings.HasSuffix(q.Symbol, ".BO") || strings.HasSuffix(q.Symbol, ".NS")) {
			continue
		}
		name := q.LongName
		if name == "" {
			name = q.ShortName
		}
		out = append(out, ListingQuote{Symbol: q.Symbol, Name: name, Exchange: q.Exchange})
	}
	return out, nil
}

// GetDailyByTicker returns daily bars for a full Yahoo ticker (".BO" or ".NS" included) between two dates.
func (p *Provider) GetDailyByTicker(ticker string, from, to time.Time) ([]OHLCV, error) {
	return p.fetchChartTicker(ticker, func(yahooSymbol string) string {
		return fmt.Sprintf("%s/chart/%s?period1=%d&period2=%d&interval=1d", p.baseURL, yahooSymbol, from.Unix(), to.Unix())
	})
}

// Snapshot is a listing's latest price and the previous session's close.
type Snapshot struct {
	Price, PrevClose float64
	At               time.Time
}

// GetSnapshot reads the latest price and previous close of a full Yahoo ticker (for example "CIPLA.BO") from the chart header of today's session.
// (Yahoo's daily series for .BO tickers carries only one bar, so the previous close has to come from here.)
func (p *Provider) GetSnapshot(ticker string) (*Snapshot, error) {
	u := fmt.Sprintf("%s/chart/%s?range=1d&interval=5m", p.baseURL, ticker)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("yahoo snapshot failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo snapshot returned %d for %s", resp.StatusCode, ticker)
	}
	var body struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Price     float64 `json:"regularMarketPrice"`
					PrevClose float64 `json:"chartPreviousClose"`
					Time      int64   `json:"regularMarketTime"`
				} `json:"meta"`
			} `json:"result"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("yahoo snapshot decode: %w", err)
	}
	if len(body.Chart.Result) == 0 || body.Chart.Result[0].Meta.Price <= 0 {
		return nil, fmt.Errorf("yahoo returned no price for %s", ticker)
	}
	m := body.Chart.Result[0].Meta
	return &Snapshot{Price: m.Price, PrevClose: m.PrevClose, At: time.Unix(m.Time, 0)}, nil
}
