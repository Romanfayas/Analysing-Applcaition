package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaxActiveAlertsPerUser bounds evaluator work and abuse.
const MaxActiveAlertsPerUser = 50

// ErrAlertLimit is returned when a user already holds the maximum number of active alerts.
var ErrAlertLimit = errors.New("active alert limit reached")

// AlertRecord is a user-owned price alert.
type AlertRecord struct {
	ID          int64      `json:"id"`
	Symbol      string     `json:"symbol"`
	SymbolID    int        `json:"-"`
	AlertType   string     `json:"alert_type"`
	Threshold   float64    `json:"threshold"`
	Channel     string     `json:"channel"`
	IsActive    bool       `json:"is_active"`
	CreatedAt   time.Time  `json:"created_at"`
	TriggeredAt *time.Time `json:"triggered_at"`
}

// AlertEvent is one recorded trigger.
type AlertEvent struct {
	ID            int64     `json:"id"`
	AlertID       int64     `json:"alert_id"`
	Symbol        string    `json:"symbol"`
	AlertType     string    `json:"alert_type"`
	Threshold     float64   `json:"threshold"`
	ObservedValue float64   `json:"observed_value"`
	ObservedAt    time.Time `json:"observed_at"`
	Message       string    `json:"message"`
	TriggeredAt   time.Time `json:"triggered_at"`
}

// CreateAlert stores a new active alert, enforcing the per-user cap atomically.
func (db *DB) CreateAlert(ctx context.Context, userID uuid.UUID, symbolID int, alertType string, threshold float64, channel string) (*AlertRecord, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Serialise concurrent creates for the same user so the cap cannot be raced past.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, userID.String()); err != nil {
		return nil, err
	}
	var active int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_alerts WHERE user_id = $1 AND is_active`, userID).Scan(&active); err != nil {
		return nil, err
	}
	if active >= MaxActiveAlertsPerUser {
		return nil, ErrAlertLimit
	}

	var a AlertRecord
	err = tx.QueryRowContext(ctx, `
		INSERT INTO user_alerts (user_id, symbol_id, alert_type, threshold, channel)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, symbol_id, alert_type, threshold::float8, channel, is_active, created_at, triggered_at
	`, userID, symbolID, alertType, threshold, channel).Scan(
		&a.ID, &a.SymbolID, &a.AlertType, &a.Threshold, &a.Channel, &a.IsActive, &a.CreatedAt, &a.TriggeredAt)
	if err != nil {
		return nil, fmt.Errorf("create alert failed: %w", err)
	}
	if err := tx.QueryRowContext(ctx, `SELECT symbol FROM symbols WHERE id = $1`, symbolID).Scan(&a.Symbol); err != nil {
		return nil, err
	}
	return &a, tx.Commit()
}

// ListAlerts returns a user's alerts, newest first.
func (db *DB) ListAlerts(ctx context.Context, userID uuid.UUID) ([]AlertRecord, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, s.symbol, a.symbol_id, a.alert_type, a.threshold::float8, a.channel, a.is_active, a.created_at, a.triggered_at
		FROM user_alerts a JOIN symbols s ON s.id = a.symbol_id
		WHERE a.user_id = $1 ORDER BY a.created_at DESC, a.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRecord{}
	for rows.Next() {
		var a AlertRecord
		if err := rows.Scan(&a.ID, &a.Symbol, &a.SymbolID, &a.AlertType, &a.Threshold, &a.Channel,
			&a.IsActive, &a.CreatedAt, &a.TriggeredAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// DeleteAlert removes a user's own alert. It reports false when no such alert exists for that user.
func (db *DB) DeleteAlert(ctx context.Context, userID uuid.UUID, id int64) (bool, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM user_alerts WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ListAlertEvents returns a user's most recent alert triggers.
func (db *DB) ListAlertEvents(ctx context.Context, userID uuid.UUID, limit int) ([]AlertEvent, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT e.id, e.alert_id, s.symbol, e.alert_type, e.threshold::float8, e.observed_value::float8,
		       e.observed_at, e.message, e.triggered_at
		FROM alert_events e JOIN symbols s ON s.id = e.symbol_id
		WHERE e.user_id = $1 ORDER BY e.triggered_at DESC, e.id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertEvent{}
	for rows.Next() {
		var e AlertEvent
		if err := rows.Scan(&e.ID, &e.AlertID, &e.Symbol, &e.AlertType, &e.Threshold, &e.ObservedValue,
			&e.ObservedAt, &e.Message, &e.TriggeredAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// maxAlertPriceAge bounds how old a price may be and still trigger an alert, so a stalled
// feed cannot fire alerts that read as live.
const maxAlertPriceAge = 4 * 24 * time.Hour

// TriggeredAlert is an alert that fired during evaluation.
type TriggeredAlert struct {
	Event   AlertEvent
	Channel string
}

// EvaluatePriceAlerts fires every active price alert whose condition holds on the latest stored
// price no older than maxAlertPriceAge. Each alert fires at most once: the deactivating UPDATE
// is the single point of truth, so concurrent evaluators cannot double-fire.
func (db *DB) EvaluatePriceAlerts(ctx context.Context) ([]TriggeredAlert, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT a.id, a.user_id, a.symbol_id, s.symbol, a.alert_type, a.threshold::float8, a.channel,
		       p.close::float8, p.ts
		FROM user_alerts a
		JOIN symbols s ON s.id = a.symbol_id
		JOIN LATERAL (
			SELECT close, ts FROM (
				(SELECT close, timestamp AS ts FROM ohlcv_intraday WHERE symbol_id = a.symbol_id ORDER BY timestamp DESC LIMIT 1)
				UNION ALL
				(SELECT close, timestamp AS ts FROM ohlcv_daily WHERE symbol_id = a.symbol_id ORDER BY timestamp DESC LIMIT 1)
			) latest ORDER BY ts DESC LIMIT 1
		) p ON true
		WHERE a.is_active
		  AND p.ts >= now() - $1::interval
		  AND ((a.alert_type = 'PRICE_ABOVE' AND p.close >= a.threshold)
		    OR (a.alert_type = 'PRICE_BELOW' AND p.close <= a.threshold))
	`, fmt.Sprintf("%d seconds", int(maxAlertPriceAge.Seconds())))
	if err != nil {
		return nil, fmt.Errorf("evaluate alerts query failed: %w", err)
	}

	type hit struct {
		id                    int64
		userID                uuid.UUID
		symbolID              int
		symbol, kind, channel string
		threshold, observed   float64
		observedAt            time.Time
	}
	var hits []hit
	for rows.Next() {
		var h hit
		if err := rows.Scan(&h.id, &h.userID, &h.symbolID, &h.symbol, &h.kind, &h.threshold, &h.channel, &h.observed, &h.observedAt); err != nil {
			rows.Close()
			return nil, err
		}
		hits = append(hits, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var fired []TriggeredAlert
	for _, h := range hits {
		op := "at or above"
		if h.kind == "PRICE_BELOW" {
			op = "at or below"
		}
		msg := fmt.Sprintf("%s is %s %.2f: last stored price %.2f (%s)",
			h.symbol, op, h.threshold, h.observed, h.observedAt.UTC().Format(time.RFC3339))

		var ev AlertEvent
		err := func() error {
			tx, err := db.BeginTx(ctx, nil)
			if err != nil {
				return err
			}
			defer tx.Rollback()
			res, err := tx.ExecContext(ctx,
				`UPDATE user_alerts SET is_active = false, triggered_at = now() WHERE id = $1 AND is_active`, h.id)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return sql.ErrNoRows // another evaluator already fired it
			}
			if err := tx.QueryRowContext(ctx, `
				INSERT INTO alert_events (alert_id, user_id, symbol_id, alert_type, threshold, observed_value, observed_at, message)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
				RETURNING id, alert_id, alert_type, threshold::float8, observed_value::float8, observed_at, message, triggered_at`,
				h.id, h.userID, h.symbolID, h.kind, h.threshold, h.observed, h.observedAt, msg).Scan(
				&ev.ID, &ev.AlertID, &ev.AlertType, &ev.Threshold, &ev.ObservedValue, &ev.ObservedAt, &ev.Message, &ev.TriggeredAt); err != nil {
				return err
			}
			return tx.Commit()
		}()
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return fired, fmt.Errorf("fire alert %v failed: %w", h.id, err)
		}
		ev.Symbol = h.symbol
		fired = append(fired, TriggeredAlert{Event: ev, Channel: h.channel})
	}
	return fired, nil
}

// UserSettings are per-user preferences. They never contain secrets.
type UserSettings struct {
	DefaultBenchmark string  `json:"default_benchmark"`
	DefaultRiskPct   float64 `json:"default_risk_pct"`
	DisplayTimezone  string  `json:"display_timezone"`
}

// DefaultUserSettings are returned for users who have not saved anything.
var DefaultUserSettings = UserSettings{DefaultBenchmark: "NIFTY 50", DefaultRiskPct: 1, DisplayTimezone: "Asia/Kolkata"}

// GetUserSettings returns stored settings, or the defaults.
func (db *DB) GetUserSettings(ctx context.Context, userID uuid.UUID) (UserSettings, error) {
	s := DefaultUserSettings
	err := db.QueryRowContext(ctx, `
		SELECT default_benchmark, default_risk_pct::float8, display_timezone FROM user_settings WHERE user_id = $1`, userID).
		Scan(&s.DefaultBenchmark, &s.DefaultRiskPct, &s.DisplayTimezone)
	if errors.Is(err, sql.ErrNoRows) {
		return DefaultUserSettings, nil
	}
	return s, err
}

// SaveUserSettings upserts a user's settings.
func (db *DB) SaveUserSettings(ctx context.Context, userID uuid.UUID, s UserSettings) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO user_settings (user_id, default_benchmark, default_risk_pct, display_timezone, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE SET default_benchmark = EXCLUDED.default_benchmark,
			default_risk_pct = EXCLUDED.default_risk_pct, display_timezone = EXCLUDED.display_timezone, updated_at = now()`,
		userID, s.DefaultBenchmark, s.DefaultRiskPct, s.DisplayTimezone)
	return err
}
