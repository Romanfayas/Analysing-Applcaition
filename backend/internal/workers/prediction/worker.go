// Package prediction schedules the intraday prediction engine: issue forecasts during the session,
// resolve them once their horizon has passed, and re-validate the models once per trading day.
package prediction

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/quant"
)

// trainAfter is when (IST) the daily walk-forward re-validation runs, after the EOD close.
const trainAfterHour, trainAfterMinute = 16, 15

type Worker struct {
	db       *database.DB
	quant    *quant.Client
	interval time.Duration
}

func NewWorker(db *database.DB, q *quant.Client, interval time.Duration) *Worker {
	return &Worker{db: db, quant: q, interval: interval}
}

func (w *Worker) Run(ctx context.Context) {
	log.Info().Dur("interval", w.interval).Msg("prediction worker started")
	if w.needsTraining(ctx, time.Now()) {
		w.call(ctx, "train")
	}
	t := time.NewTicker(w.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			w.tick(ctx, now)
		}
	}
}

func (w *Worker) tick(ctx context.Context, now time.Time) {
	if market.GetCurrentSession(now) == market.SessionOpen {
		w.call(ctx, "generate")
	}
	w.call(ctx, "resolve") // also after the close, so the last forecasts of the day get their outcomes
	if w.needsTraining(ctx, now) {
		w.call(ctx, "train")
	}
}

// needsTraining: no active model yet, or it is after the close on a trading day and today's
// re-validation has not run.
func (w *Worker) needsTraining(ctx context.Context, now time.Time) bool {
	var last *time.Time
	if err := w.db.QueryRowContext(ctx, `SELECT max(trained_at) FROM prediction_model_runs`).Scan(&last); err != nil {
		log.Error().Err(err).Msg("prediction worker: cannot read model runs")
		return false
	}
	if last == nil {
		return true
	}
	ist := now.In(market.Location)
	holiday, _ := market.IsTradingHoliday(ist)
	if holiday || ist.Weekday() == time.Saturday || ist.Weekday() == time.Sunday {
		return false
	}
	cutoff := time.Date(ist.Year(), ist.Month(), ist.Day(), trainAfterHour, trainAfterMinute, 0, 0, market.Location)
	return ist.After(cutoff) && last.Before(cutoff)
}

func (w *Worker) call(ctx context.Context, op string) {
	out, err := w.quant.PredictionCall(ctx, op)
	if err != nil {
		log.Error().Err(err).Str("op", op).Msg("prediction engine call failed")
		return
	}
	log.Info().Str("op", op).Interface("result", out).Msg("prediction engine")
}
