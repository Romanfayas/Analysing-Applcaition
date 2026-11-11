// Package deals keeps the official NSE bulk and block deals. NSE replaces the two files every trading day, so they are fetched often and
// each run stores only what is new.
package deals

import (
	"context"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/largedeals"
)

// Worker downloads both files on a timer.
type Worker struct {
	db       *database.DB
	hc       *http.Client
	interval time.Duration
}

func NewWorker(db *database.DB, interval time.Duration) *Worker {
	return &Worker{db: db, hc: &http.Client{Timeout: 45 * time.Second}, interval: interval}
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

// Tick fetches both files and stores the deals of every stock the app follows.
func (w *Worker) Tick(ctx context.Context) {
	syms, err := w.db.GetAllActiveSymbols(ctx)
	if err != nil {
		log.Error().Err(err).Msg("deals: cannot list stocks")
		return
	}
	ids := map[string]int{}
	for _, s := range syms {
		ids[s.Symbol] = s.ID
	}
	for _, f := range []struct{ url, kind string }{{largedeals.BulkURL, "BULK"}, {largedeals.BlockURL, "BLOCK"}} {
		deals, skipped, err := largedeals.Fetch(ctx, w.hc, f.url, f.kind)
		if err != nil {
			log.Warn().Err(err).Str("kind", f.kind).Msg("deals: NSE file unavailable")
			continue
		}
		bySymbol := map[int][]largedeals.Deal{}
		for _, d := range deals {
			if id, ok := ids[d.Symbol]; ok {
				bySymbol[id] = append(bySymbol[id], d)
			}
		}
		added := 0
		for id, list := range bySymbol {
			n, err := w.db.SaveLargeDeals(ctx, id, list)
			if err != nil {
				log.Error().Err(err).Msg("deals: store failed")
				continue
			}
			added += n
		}
		log.Info().Str("kind", f.kind).Int("rows_in_file", len(deals)).Int("skipped", skipped).Int("followed_stocks", len(bySymbol)).Int("new", added).Msg("deals: pass done")
	}
}
