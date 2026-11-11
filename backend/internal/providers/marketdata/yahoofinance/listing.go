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
