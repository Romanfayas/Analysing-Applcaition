// Package news refreshes headlines, official announcements and events for stocks, and serves on-demand refreshes for the stock page.
package news

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/stocknews"
)

// Freshness is how long stored data is served before a page view triggers a refresh.
const Freshness = 15 * time.Minute

// Refresher fetches and stores one stock's feeds. A per-symbol lock stops two requests fetching the same stock at once.
type Refresher struct {
	db      *database.DB
	fetcher *stocknews.Fetcher
	mu      sync.Mutex
	locks   map[int]*sync.Mutex
}

// NewRefresher creates a refresher.
func NewRefresher(db *database.DB) *Refresher {
	return &Refresher{db: db, fetcher: stocknews.NewFetcher(), locks: map[int]*sync.Mutex{}}
}

func (r *Refresher) lock(id int) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.locks[id] == nil {
		r.locks[id] = &sync.Mutex{}
	}
	return r.locks[id]
}

// Result reports what a refresh did, per feed.
type Result struct {
	News, Filings, Events int
	Errors                map[string]string
}

// Refresh fetches every feed of a stock that is older than maxAge (0 = force) and stores the results. Failures are logged per feed and
// returned, never hidden: a failed feed leaves the stored data in place and the page says when it last succeeded.
func (r *Refresher) Refresh(ctx context.Context, sym database.SymbolRecord, maxAge time.Duration) Result {
	l := r.lock(sym.ID)
	l.Lock()
	defer l.Unlock()
	res := Result{Errors: map[string]string{}}
	now := time.Now()
	due := func(feed string) bool {
		if maxAge == 0 {
			return true
		}
		t, err := r.db.LastNewsFetch(ctx, sym.ID, feed)
		return err != nil || now.Sub(t) >= maxAge
	}
	if due("NEWS") {
		items, err := r.fetcher.NewsFor(ctx, companyQuery(sym))
		if err == nil {
			res.News, err = r.db.SaveNews(ctx, sym.ID, items)
		}
		r.db.LogNewsFetch(ctx, sym.ID, "NEWS", err == nil, len(items), errText(err))
		if err != nil {
			res.Errors["news"] = err.Error()
		}
	}
	if due("FILINGS") {
		items, err := r.fetcher.FilingsFor(ctx, sym.Symbol, now)
		if err == nil {
			res.Filings, err = r.db.SaveFilings(ctx, sym.ID, items)
		}
		r.db.LogNewsFetch(ctx, sym.ID, "FILINGS", err == nil, len(items), errText(err))
		if err != nil {
			res.Errors["filings"] = err.Error()
		}
	}
	if due("EVENTS") {
		items, err := r.fetcher.EventsFor(ctx, sym.Symbol, now)
		if len(items) > 0 {
			var n int
			var serr error
			n, serr = r.db.SaveEvents(ctx, sym.ID, items)
			res.Events = n
			if err == nil {
				err = serr
			}
		}
		r.db.LogNewsFetch(ctx, sym.ID, "EVENTS", err == nil, len(items), errText(err))
		if err != nil {
			res.Errors["events"] = err.Error()
		}
	}
	return res
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// companyQuery turns the stored name into a search phrase; falls back to the ticker.
func companyQuery(s database.SymbolRecord) string {
	if s.Name != "" && s.Name != s.Symbol {
		return s.Name
	}
	return s.Symbol + " NSE"
}

// Worker refreshes a batch of the stalest stocks every tick, tracked breakout stocks first.
type Worker struct {
	r        *Refresher
	db       *database.DB
	interval time.Duration
	batch    int
}

// NewWorker creates the worker (a batch of 10 every 15 minutes covers the 52-stock universe about every 80 minutes).
func NewWorker(db *database.DB, r *Refresher, interval time.Duration) *Worker {
	return &Worker{r: r, db: db, interval: interval, batch: 10}
}

// Run ticks until the context ends.
func (w *Worker) Run(ctx context.Context) {
	w.Tick(ctx)
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.Tick(ctx)
		}
	}
}

// Tick refreshes the next batch.
func (w *Worker) Tick(ctx context.Context) {
	syms, err := w.db.NewsDueSymbols(ctx, w.batch)
	if err != nil {
		log.Error().Err(err).Msg("news: cannot choose stocks to refresh")
		return
	}
	var news, filings, events, failed int
	for _, s := range syms {
		cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		res := w.r.Refresh(cctx, s, 10*time.Minute)
		cancel()
		news, filings, events, failed = news+res.News, filings+res.Filings, events+res.Events, failed+len(res.Errors)
	}
	log.Info().Str("summary", fmt.Sprintf("stocks=%d new_headlines=%d new_filings=%d new_events=%d feed_errors=%d", len(syms), news, filings, events, failed)).Msg("news: refresh pass")
}
