// Package alerts evaluates user price alerts against the latest stored prices.
package alerts

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/notifications"
)

// Evaluator periodically fires due alerts and, for the TELEGRAM channel, notifies the operator chat.
type Evaluator struct {
	db       *database.DB
	notifier *notifications.TelegramNotifier
	interval time.Duration
}

func NewEvaluator(db *database.DB, notifier *notifications.TelegramNotifier, interval time.Duration) *Evaluator {
	return &Evaluator{db: db, notifier: notifier, interval: interval}
}

// Run evaluates immediately and then on every tick until ctx is cancelled.
func (e *Evaluator) Run(ctx context.Context) {
	log.Info().Dur("interval", e.interval).Msg("alert evaluator started")
	t := time.NewTicker(e.interval)
	defer t.Stop()
	for {
		e.once(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (e *Evaluator) once(ctx context.Context) {
	fired, err := e.db.EvaluatePriceAlerts(ctx)
	if err != nil {
		log.Error().Err(err).Msg("alert evaluation failed")
	}
	for _, f := range fired {
		log.Info().Int64("alert_id", f.Event.AlertID).Str("symbol", f.Event.Symbol).Msg("alert triggered")
		if f.Channel != "TELEGRAM" || e.notifier == nil {
			continue
		}
		if err := e.notifier.SendAlert(notifications.Alert{
			Type:      notifications.AlertType("PRICE_ALERT"),
			Title:     "Price alert: " + f.Event.Symbol,
			Message:   f.Event.Message,
			Symbol:    f.Event.Symbol,
			Timestamp: f.Event.TriggeredAt,
		}); err != nil {
			log.Warn().Err(err).Int64("alert_id", f.Event.AlertID).Msg("telegram delivery failed; event is still recorded")
		}
	}
}
