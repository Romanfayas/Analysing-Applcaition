// Package breakouts records every confirmed breakout and follows it, so the platform can show whether breakouts actually
// happened rather than only announcing them.
package breakouts

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/handlers"
	svc "github.com/halal-equity/backend/internal/services/breakouts"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/quant"
)

// Worker runs the tracker on a timer.
type Worker struct {
	db           *database.DB
	quant        *quant.Client
	interval     time.Duration
	lastStudyDay string
}

// NewWorker creates the tracker.
func NewWorker(db *database.DB, q *quant.Client, interval time.Duration) *Worker {
	return &Worker{db: db, quant: q, interval: interval}
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

// Tick records today's confirmed breakouts and updates every open one.
func (w *Worker) Tick(ctx context.Context) {
	w.maybeRefreshStudy(ctx)
	if err := w.record(ctx); err != nil {
		log.Error().Err(err).Msg("breakout tracker: scan failed")
	}
	open, err := w.db.OpenBreakouts(ctx)
	if err != nil {
		log.Error().Err(err).Msg("breakout tracker: cannot list open signals")
		return
	}
	now := time.Now()
	for _, o := range open {
		bars, err := w.db.FollowBars(ctx, o.SymbolID, o.SignalDate, now)
		if err != nil {
			log.Error().Err(err).Int("signal", o.ID).Msg("breakout tracker: no path")
			continue
		}
		p := svc.Follow(o.Levels, bars, svc.LimitSessions)
		var nifty *float64
		if p.EntryDate != nil {
			end := now
			if p.ExitDate != nil {
				end = *p.ExitDate
			}
			if a, okA, _ := w.db.IndexCloseOnOrBefore(ctx, database.IndexNifty50, p.EntryDate.In(market.Location)); okA {
				if b, okB, _ := w.db.IndexCloseOnOrBefore(ctx, database.IndexNifty50, end.In(market.Location)); okB && b.Date.After(a.Date) {
					v := (b.Close/a.Close - 1) * 100
					nifty = &v
				}
			}
		}
		if err := w.db.UpdateBreakout(ctx, o.ID, p, nifty); err != nil {
			log.Error().Err(err).Int("signal", o.ID).Msg("breakout tracker: update failed")
		}
	}
}

func (w *Worker) record(ctx context.Context) error {
	if w.quant == nil {
		return nil
	}
	scan, err := handlers.RunBreakoutScan(ctx, w.db, w.quant, time.Now())
	if err != nil {
		return err
	}
	if scan.DataAsOf == nil {
		return nil
	}
	day, err := time.Parse("2006-01-02", *scan.DataAsOf)
	if err != nil {
		return err
	}
	symbols, err := w.db.GetAllActiveSymbols(ctx)
	if err != nil {
		return err
	}
	ids := map[string]int{}
	for _, s := range symbols {
		ids[s.Symbol] = s.ID
	}
	for _, c := range scan.Candidates {
		if len(c.EntryZone) != 2 {
			continue
		}
		id, ok := ids[c.Symbol]
		if !ok {
			continue
		}
		switch c.Stage {
		case "CONFIRMED_BREAKOUT":
			if err := w.db.RecordBreakout(ctx, database.BreakoutSignal{SymbolID: id, SignalDate: day, SignalClose: c.CurrentPrice, EntryLow: c.EntryZone[0], EntryHigh: c.EntryZone[1],
				Stop: c.InvalidationLevel, T1: c.Target1, T2: c.Target2, Score: c.StructureScore, RVOL: c.RVOL, Shariah: c.ShariahStatus,
				Reasons: c.Reasons, Stage: c.Stage, Resistance: c.ResistanceLevel, ATR: c.ATR, Breakdown: c.ScoreBreakdown}); err != nil {
				return err
			}
		case "BREAKOUT_CANDIDATE":
			// An upcoming breakout: tracked from the day it is flagged, and scored later on whether the engine's own confirmed breakout followed.
			if err := w.db.RecordWatchFlag(ctx, database.WatchFlag{SymbolID: id, FlaggedDate: day, Close: c.CurrentPrice, Resistance: c.ResistanceLevel, Stop: c.InvalidationLevel,
				EntryLow: c.EntryZone[0], EntryHigh: c.EntryZone[1], Target1: c.Target1, Target2: c.Target2, Score: c.StructureScore, RVOL: c.RVOL, ATR: c.ATR,
				Reasons: c.Reasons, Breakdown: c.ScoreBreakdown, Shariah: c.ShariahStatus}); err != nil {
				return err
			}
		}
	}
	return nil
}

// maybeRefreshStudy re-runs the historical study once each evening, after the day's NSE file has normally arrived.
func (w *Worker) maybeRefreshStudy(ctx context.Context) {
	ist := time.Now().In(market.Location)
	day := ist.Format("2006-01-02")
	if w.quant == nil || w.lastStudyDay == day || ist.Hour() < 21 {
		return
	}
	w.lastStudyDay = day
	if err := w.quant.RefreshBreakoutEvidence(ctx); err != nil {
		log.Error().Err(err).Msg("breakout tracker: study refresh failed")
	}
	// After the study: lessons from newly resolved signals, then the walk-forward test and promotion/probation decision.
	if err := w.quant.RefreshLearning(ctx); err != nil {
		log.Error().Err(err).Msg("learning loop: run failed")
	}
}
