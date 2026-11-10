package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// StrategyVersionView is a pre-registered strategy version and its current status (the newest status event).
type StrategyVersionView struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Origin       string         `json:"origin"`
	Params       map[string]any `json:"params"`
	Status       string         `json:"status"`
	StatusReason string         `json:"status_reason"`
	Actor        string         `json:"actor"`
	StatusAt     *time.Time     `json:"status_at"`
}

// LessonTag is how often a descriptive tag appeared on resolved breakouts and how those ended. A tag is not a rule.
type LessonTag struct {
	Tag    string `json:"tag"`
	N      int    `json:"n"`
	Worked int    `json:"worked"`
	Failed int    `json:"failed"`
}

// LessonView is one resolved breakout and the plain-language lesson recorded for it.
type LessonView struct {
	Symbol     string   `json:"symbol"`
	SignalDate string   `json:"signal_date"`
	Origin     string   `json:"origin"`
	Verdict    string   `json:"verdict"`
	ReturnPct  *float64 `json:"return_pct"`
	Lesson     string   `json:"lesson"`
	Tags       []string `json:"tags"`
}

// LearningRunView is one learner run.
type LearningRunView struct {
	ID          int64     `json:"id"`
	DataThrough string    `json:"data_through"`
	Decision    string    `json:"decision"`
	CreatedAt   time.Time `json:"created_at"`
}

// LearningSummary is everything the Learning page shows.
type LearningSummary struct {
	Versions  []StrategyVersionView `json:"versions"`
	Active    *StrategyVersionView  `json:"active"`
	Control   *StrategyVersionView  `json:"control"`
	LastRun   map[string]any        `json:"last_run"`
	LastRunID *int64                `json:"last_run_id"`
	LastRunAt *time.Time            `json:"last_run_at"`
	Decision  *string               `json:"last_decision"`
	Runs      []LearningRunView     `json:"runs"`
	Tags      []LessonTag           `json:"lesson_tags"`
	Lessons   []LessonView          `json:"recent_lessons"`
	LessonN   int                   `json:"lessons_total"`
	Events    []map[string]any      `json:"recent_status_events"`
}

// GetLearningSummary reads the append-only learning tables. Nothing here changes any state.
func (db *DB) GetLearningSummary(ctx context.Context) (*LearningSummary, error) {
	out := &LearningSummary{Versions: []StrategyVersionView{}, Runs: []LearningRunView{}, Tags: []LessonTag{}, Lessons: []LessonView{}, Events: []map[string]any{}}

	rows, err := db.QueryContext(ctx, `SELECT id, name, origin, params, COALESCE(status, 'CANDIDATE'), COALESCE(status_reason, ''), COALESCE(actor, ''), status_at
		FROM strategy_current ORDER BY (status = 'ACTIVE_PAPER') DESC, (status = 'CONTROL') DESC, name`)
	if err != nil {
		return nil, fmt.Errorf("versions: %w", err)
	}
	for rows.Next() {
		var v StrategyVersionView
		var raw []byte
		if err := rows.Scan(&v.ID, &v.Name, &v.Origin, &raw, &v.Status, &v.StatusReason, &v.Actor, &v.StatusAt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(raw, &v.Params)
		out.Versions = append(out.Versions, v)
	}
	rows.Close()
	for i := range out.Versions {
		switch out.Versions[i].Status {
		case "ACTIVE_PAPER":
			out.Active = &out.Versions[i]
		case "CONTROL":
			out.Control = &out.Versions[i]
		}
	}

	var raw []byte
	var id int64
	var at time.Time
	var dec string
	err = db.QueryRowContext(ctx, `SELECT id, result, decision, created_at FROM learning_runs ORDER BY id DESC LIMIT 1`).Scan(&id, &raw, &dec, &at)
	if err == nil {
		_ = json.Unmarshal(raw, &out.LastRun)
		out.LastRunID, out.LastRunAt, out.Decision = &id, &at, &dec
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("last run: %w", err)
	}

	rr, err := db.QueryContext(ctx, `SELECT id, data_through::text, decision, created_at FROM learning_runs ORDER BY id DESC LIMIT 15`)
	if err != nil {
		return nil, err
	}
	for rr.Next() {
		var r LearningRunView
		if err := rr.Scan(&r.ID, &r.DataThrough, &r.Decision, &r.CreatedAt); err != nil {
			rr.Close()
			return nil, err
		}
		out.Runs = append(out.Runs, r)
	}
	rr.Close()

	tr, err := db.QueryContext(ctx, `SELECT tag, count(*), count(*) FILTER (WHERE verdict = 'WORKED'), count(*) FILTER (WHERE verdict = 'FAILED')
		FROM breakout_lessons, unnest(tags) AS tag WHERE tag NOT LIKE 'VERDICT\_%' AND tag NOT LIKE 'ORIGIN\_%' GROUP BY tag ORDER BY count(*) DESC, tag LIMIT 30`)
	if err != nil {
		return nil, err
	}
	for tr.Next() {
		var t LessonTag
		if err := tr.Scan(&t.Tag, &t.N, &t.Worked, &t.Failed); err != nil {
			tr.Close()
			return nil, err
		}
		out.Tags = append(out.Tags, t)
	}
	tr.Close()

	lr, err := db.QueryContext(ctx, `SELECT s.symbol, l.signal_date::text, l.origin, l.verdict, l.return_pct::float8, l.lesson, l.tags
		FROM breakout_lessons l JOIN symbols s ON s.id = l.symbol_id ORDER BY l.signal_date DESC, l.tracking_id DESC LIMIT 25`)
	if err != nil {
		return nil, err
	}
	for lr.Next() {
		var l LessonView
		var tags []byte
		if err := lr.Scan(&l.Symbol, &l.SignalDate, &l.Origin, &l.Verdict, &l.ReturnPct, &l.Lesson, &tags); err != nil {
			lr.Close()
			return nil, err
		}
		l.Tags = parsePGTextArray(string(tags))
		out.Lessons = append(out.Lessons, l)
	}
	lr.Close()
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM breakout_lessons`).Scan(&out.LessonN)

	er, err := db.QueryContext(ctx, `SELECT e.id, v.name, e.status, e.reason, e.actor, e.created_at FROM strategy_status_events e
		JOIN strategy_versions v ON v.id = e.version_id WHERE e.actor <> 'SEED' ORDER BY e.id DESC LIMIT 15`)
	if err != nil {
		return nil, err
	}
	for er.Next() {
		var id int64
		var name, status, reason, actor string
		var ts time.Time
		if err := er.Scan(&id, &name, &status, &reason, &actor, &ts); err != nil {
			er.Close()
			return nil, err
		}
		out.Events = append(out.Events, map[string]any{"id": id, "version": name, "status": status, "reason": reason, "actor": actor, "at": ts})
	}
	er.Close()
	return out, nil
}

// parsePGTextArray turns a Postgres text[] literal such as {A,B,"C D"} into a slice.
func parsePGTextArray(s string) []string {
	out := []string{}
	if len(s) < 2 || s[0] != '{' {
		return out
	}
	cur, inQ := "", false
	for _, r := range s[1 : len(s)-1] {
		switch {
		case r == '"':
			inQ = !inQ
		case r == ',' && !inQ:
			out = append(out, cur)
			cur = ""
		default:
			cur += string(r)
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// ErrNotActiveVersion is returned when a rollback targets a version that is not the active paper strategy.
var ErrNotActiveVersion = errors.New("that version is not the active paper strategy")

// RollbackActiveVersion is the owner's one-click rollback: it appends a ROLLED_BACK event, so the platform falls back to the frozen
// control. The version can never be promoted again in this space version. Nothing is deleted or edited.
func (db *DB) RollbackActiveVersion(ctx context.Context, versionID int64, reason string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var status string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(status, '') FROM strategy_current WHERE id = $1`, versionID).Scan(&status); err != nil {
		return err
	}
	if status != "ACTIVE_PAPER" {
		return ErrNotActiveVersion
	}
	if reason == "" {
		reason = "rolled back by the owner"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO strategy_status_events (version_id, status, reason, actor) VALUES ($1, 'ROLLED_BACK', $2, 'OWNER')`, versionID, reason); err != nil {
		return err
	}
	return tx.Commit()
}
