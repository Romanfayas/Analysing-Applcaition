package handlers

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/halal-equity/backend/internal/providers/marketdata/yahoofinance"
	"github.com/halal-equity/backend/pkg/response"
)

// ExchangeQuote is one exchange's latest price for a stock.
type ExchangeQuote struct {
	Exchange  string   `json:"exchange"`
	Available bool     `json:"available"`
	Price     *float64 `json:"price,omitempty"`
	PrevClose *float64 `json:"prev_close,omitempty"`
	Change    *float64 `json:"change,omitempty"`
	ChangePct *float64 `json:"change_pct,omitempty"`
	AsOf      *string  `json:"as_of,omitempty"`
	Source    string   `json:"source"`
	Note      string   `json:"note,omitempty"`
}

type bseCacheEntry struct {
	q  ExchangeQuote
	at time.Time
}

var (
	bseMu       sync.Mutex
	bseCache    = map[string]bseCacheEntry{}
	bseProvider = yahoofinance.NewProvider()
)

const (
	bseCacheTTL     = 45 * time.Second
	bseFailCacheTTL = 20 * time.Second
)

func withChange(q *ExchangeQuote, price, prev float64) {
	q.Available, q.Price = true, &price
	if prev > 0 {
		ch, pct := price-prev, (price/prev-1)*100
		q.PrevClose, q.Change, q.ChangePct = &prev, &ch, &pct
	}
}

// bseQuote reads the BSE listing (Yahoo ticker SYMBOL.BO): the chart header of today's session gives the latest price and the previous close.
func bseQuote(symbol string) ExchangeQuote {
	bseMu.Lock()
	if e, ok := bseCache[symbol]; ok {
		ttl := bseCacheTTL
		if !e.q.Available {
			ttl = bseFailCacheTTL
		}
		if time.Since(e.at) < ttl {
			bseMu.Unlock()
			return e.q
		}
	}
	bseMu.Unlock()

	q := ExchangeQuote{Exchange: "BSE", Source: "Yahoo Finance " + symbol + ".BO (delayed)"}
	snap, err := bseProvider.GetSnapshot(symbol + ".BO")
	if err != nil {
		q.Note = "The BSE price could not be fetched right now."
	} else {
		withChange(&q, snap.Price, snap.PrevClose)
		at := snap.At.In(istZone).Format("2006-01-02 15:04")
		q.AsOf = &at
	}
	bseMu.Lock()
	bseCache[symbol] = bseCacheEntry{q: q, at: time.Now()}
	bseMu.Unlock()
	return q
}

// GetQuote returns the stock's price on NSE (the app's own latest stored price) and on BSE, with the change since the previous close, and the 52-week range.
func (h *StocksHandler) GetQuote(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	bseCh := make(chan ExchangeQuote, 1)
	go func() { bseCh <- bseQuote(sym.Symbol) }()

	nse := ExchangeQuote{Exchange: "NSE", Source: "Latest price stored by this app (delayed market data)"}
	out := map[string]any{"symbol": sym.Symbol, "name": sym.Name}
	if px, err := h.db.GetLatestPrice(ctx, sym.ID); err == nil && px != nil && px.Close > 0 {
		prev := h.closeBefore(ctx, sym.ID, istDate(px.Timestamp))
		withChange(&nse, px.Close, prev)
		at := px.Timestamp.In(istZone).Format("2006-01-02 15:04")
		nse.AsOf = &at
	} else {
		nse.Note = "No price stored for this stock."
	}
	if bars, err := h.db.GetDailyBars(ctx, sym.ID, 252, false); err == nil && len(bars) > 0 {
		hi, lo := bars[0].High, bars[0].Low
		for _, b := range bars {
			hi, lo = max(hi, b.High), min(lo, b.Low)
		}
		out["week52"] = map[string]any{"high": hi, "low": lo, "sessions": len(bars)}
	}
	out["nse"] = nse
	select {
	case b := <-bseCh:
		out["bse"] = b
	case <-ctx.Done():
		out["bse"] = ExchangeQuote{Exchange: "BSE", Note: "The BSE price timed out."}
	}
	response.JSON(w, http.StatusOK, out)
}
