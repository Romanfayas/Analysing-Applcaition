package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaxUserAddedSymbols bounds how many owner-added instruments the platform will track.
const MaxUserAddedSymbols = 30

// Onboarding states stored in symbols.metadata.onboarding.state.
const (
	OnboardRunning = "RUNNING"
	OnboardDone    = "DONE"
	OnboardFailed  = "FAILED"
)

// OnboardStep records one stage of adding a stock and whether it worked.
type OnboardStep struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// OnboardStatus is the progress of adding a stock, stored on the symbol so it survives restarts.
type OnboardStatus struct {
	State     string        `json:"state"`
	Steps     []OnboardStep `json:"steps"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// AddUserSymbol registers a new instrument. It never touches an existing row: created is false when the symbol is
// already tracked (whether canonical or added earlier).
func (db *DB) AddUserSymbol(ctx context.Context, ticker, name string, addedBy uuid.UUID) (id int, created bool, err error) {
	var n int
	if err = db.QueryRowContext(ctx, `SELECT count(*) FROM symbols WHERE metadata->>'user_added' = 'true'`).Scan(&n); err != nil {
		return 0, false, err
	}
	meta, _ := json.Marshal(map[string]any{"user_added": true, "added_by": addedBy.String(), "added_at": time.Now().UTC().Format(time.RFC3339)})
	err = db.QueryRowContext(ctx, `
		INSERT INTO symbols (symbol, name, exchange, is_active, metadata)
		SELECT $1, $2, 'NSE', true, $3::jsonb WHERE $4::int < $5::int
		ON CONFLICT (symbol, exchange) DO NOTHING
		RETURNING id`, ticker, name, string(meta), n, MaxUserAddedSymbols).Scan(&id)
	if err == nil {
		return id, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, false, fmt.Errorf("register symbol %s: %w", ticker, err)
	}
	if n >= MaxUserAddedSymbols {
		var exists bool
		_ = db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM symbols WHERE symbol = $1 AND exchange = 'NSE')`, ticker).Scan(&exists)
		if !exists {
			return 0, false, fmt.Errorf("the limit of %d added stocks has been reached", MaxUserAddedSymbols)
		}
	}
	if err = db.QueryRowContext(ctx, `SELECT id FROM symbols WHERE symbol = $1 AND exchange = 'NSE'`, ticker).Scan(&id); err != nil {
		return 0, false, err
	}
	return id, false, nil
}

// UserAddedSymbols lists the instruments the owner added (they are ingested alongside the NIFTY 50).
func (db *DB) UserAddedSymbols(ctx context.Context) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT symbol, name FROM symbols WHERE is_active AND metadata->>'user_added' = 'true'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var s, n string
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// SetOnboardStatus stores the progress of adding a stock on the symbol's metadata.
func (db *DB) SetOnboardStatus(ctx context.Context, symbolID int, st OnboardStatus) error {
	st.UpdatedAt = time.Now().UTC()
	raw, _ := json.Marshal(st)
	_, err := db.ExecContext(ctx, `UPDATE symbols SET metadata = jsonb_set(COALESCE(metadata, '{}'::jsonb), '{onboarding}', $2::jsonb), updated_at = NOW() WHERE id = $1`, symbolID, string(raw))
	return err
}

// GetOnboardStatus returns the stored progress, or nil when the symbol has none (a canonical instrument).
func (db *DB) GetOnboardStatus(ctx context.Context, ticker string) (*OnboardStatus, bool, error) {
	var raw []byte
	var userAdded bool
	err := db.QueryRowContext(ctx, `SELECT COALESCE(metadata->'onboarding', 'null'::jsonb), COALESCE(metadata->>'user_added' = 'true', false) FROM symbols WHERE symbol = $1 AND exchange = 'NSE'`, ticker).Scan(&raw, &userAdded)
	if err != nil {
		return nil, false, err
	}
	var st OnboardStatus
	if string(raw) == "null" || json.Unmarshal(raw, &st) != nil {
		return nil, userAdded, nil
	}
	return &st, userAdded, nil
}
