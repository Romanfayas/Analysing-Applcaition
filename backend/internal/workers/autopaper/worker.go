package autopaper

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
)

// Worker keeps the automatic paper portfolios current. It runs often because it is cheap and idempotent: it acts only when the exchange
// has recorded a new session (the NSE daily file arrives in the evening) or a new signal appeared.
type Worker struct {
	db       *database.DB
	store    Store
	interval time.Duration
}

// NewWorker creates the worker.
func NewWorker(db *database.DB, interval time.Duration) *Worker {
	return &Worker{db: db, store: DBStore{DB: db}, interval: interval}
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

// Specs returns the portfolios to run: the frozen control always, plus the learner's active version when there is one.
func (w *Worker) Specs(ctx context.Context) ([]Spec, error) {
	ctl := map[string]string{"plan": ControlPlan, "entry": ControlEntry, "market": ControlMarket}
	label := "AUTO · current plan (frozen control)"
	pid, err := w.db.EnsureAutoPortfolio(ctx, label, ctl)
	if err != nil {
		return nil, err
	}
	sp, _ := SpecFromParams(pid, label, ctl)
	specs := []Spec{sp}
	name, params, ok, err := w.db.ActivePaperStrategy(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		al := "AUTO · learner's active version " + name
		apid, err := w.db.EnsureAutoPortfolio(ctx, al, params)
		if err != nil {
			return nil, err
		}
		if as, ok := SpecFromParams(apid, al, params); ok {
			specs = append(specs, as)
		} else {
			log.Warn().Str("version", name).Msg("auto paper: active version uses a plan this build does not know; not traded")
		}
	}
	return specs, nil
}

// Tick processes every portfolio once.
func (w *Worker) Tick(ctx context.Context) {
	specs, err := w.Specs(ctx)
	if err != nil {
		log.Error().Err(err).Msg("auto paper: cannot prepare portfolios")
		return
	}
	for _, sp := range specs {
		res, err := Process(ctx, w.store, sp)
		if err != nil {
			log.Error().Err(err).Str("portfolio", sp.Name).Msg("auto paper: pass failed")
			continue
		}
		if res.Entries+res.Exits+res.Skips > 0 {
			log.Info().Str("portfolio", sp.Name).Str("summary", fmt.Sprintf("entries=%d exits=%d skipped=%d sessions=%d", res.Entries, res.Exits, res.Skips, res.Sessions)).Msg("auto paper: updated")
		}
	}
}
