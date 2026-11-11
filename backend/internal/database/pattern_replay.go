package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ReplayTrade is one paper trade of the pattern replay: a past reading of a stock and what the simulated trade that followed did.
type ReplayTrade struct {
	Symbol      string   `json:"symbol"`
	Setup       string   `json:"setup"`
	SignalDate  string   `json:"signal_date"`
	SignalClose float64  `json:"signal_close"`
	DistancePct *float64 `json:"distance_pct"`
	Resistance  *float64 `json:"resistance"`
	Stop        *float64 `json:"stop"`
	Target1     *float64 `json:"target_1"`
	Outcome     string   `json:"outcome"`
	Triggered   bool     `json:"triggered"`
	EntryDate   *string  `json:"entry_date"`
	EntryTime   *string  `json:"entry_time"`
	EntryPrice  *float64 `json:"entry_price"`
	ExitDate    *string  `json:"exit_date"`
	ExitTime    *string  `json:"exit_time"`
	ExitPrice   *float64 `json:"exit_price"`
	BarsHeld    *int     `json:"bars_held"`
	GrossPct    *float64 `json:"gross_pct"`
	NetPct      *float64 `json:"net_pct"`
	ExcessPct   *float64 `json:"excess_vs_nifty_pct"`
	Move10dPct  *float64 `json:"move_10d_pct"`
	Note        *string  `json:"note"`
}

// LatestPatternReplay returns the newest replay summary and when it ran; (nil, zero, nil) before the first nightly run.
func (db *DB) LatestPatternReplay(ctx context.Context) (map[string]any, time.Time, error) {
	var raw []byte
	var at time.Time
	err := db.QueryRowContext(ctx, `SELECT summary, created_at FROM pattern_replay_runs ORDER BY id DESC LIMIT 1`).Scan(&raw, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("query pattern replay summary failed: %w", err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, time.Time{}, fmt.Errorf("decode pattern replay summary failed: %w", err)
	}
	return out, at, nil
}

// PatternReplayTrades lists trades of the newest run, newest signal first. setup and symbol are optional filters.
func (db *DB) PatternReplayTrades(ctx context.Context, setup, symbol string, limit int) ([]ReplayTrade, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.symbol, t.setup, to_char(t.signal_date,'YYYY-MM-DD'), t.signal_close::float8, t.distance_pct::float8, t.resistance::float8, t.stop_level::float8, t.target_1::float8,
		       t.outcome, t.triggered, to_char(t.entry_date,'YYYY-MM-DD'), t.entry_time, t.entry_price::float8, to_char(t.exit_date,'YYYY-MM-DD'), t.exit_time, t.exit_price::float8,
		       t.bars_held, t.gross_pct::float8, t.net_pct::float8, t.excess_vs_nifty_pct::float8, t.move_10d_pct::float8, t.note
		FROM pattern_replay_trades t JOIN symbols s ON s.id = t.symbol_id
		WHERE t.run_id = (SELECT max(id) FROM pattern_replay_runs) AND ($1 = '' OR t.setup = $1) AND ($2 = '' OR s.symbol = $2)
		ORDER BY t.signal_date DESC, s.symbol LIMIT $3`, setup, symbol, limit)
	if err != nil {
		return nil, fmt.Errorf("query pattern replay trades failed: %w", err)
	}
	defer rows.Close()
	out := []ReplayTrade{}
	for rows.Next() {
		var t ReplayTrade
		if err := rows.Scan(&t.Symbol, &t.Setup, &t.SignalDate, &t.SignalClose, &t.DistancePct, &t.Resistance, &t.Stop, &t.Target1, &t.Outcome, &t.Triggered, &t.EntryDate, &t.EntryTime,
			&t.EntryPrice, &t.ExitDate, &t.ExitTime, &t.ExitPrice, &t.BarsHeld, &t.GrossPct, &t.NetPct, &t.ExcessPct, &t.Move10dPct, &t.Note); err != nil {
			return nil, fmt.Errorf("scan pattern replay trade failed: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
