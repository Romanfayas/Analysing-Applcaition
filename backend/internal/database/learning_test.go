package database

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestParsePGTextArray(t *testing.T) {
	cases := map[string][]string{
		"{}":                           {},
		"{A}":                          {"A"},
		"{ORIGIN_LIVE,VERDICT_WORKED}": {"ORIGIN_LIVE", "VERDICT_WORKED"},
		`{"A B",C}`:                    {"A B", "C"},
	}
	for in, want := range cases {
		if got := parsePGTextArray(in); !reflect.DeepEqual(got, want) {
			t.Errorf("parsePGTextArray(%q)=%v want %v", in, got, want)
		}
	}
}

// TestRollbackActiveVersion needs a scratch database that already has migrations 44 and 45 applied (LEARNING_TEST_DSN); it is skipped otherwise.
// It never runs against the live database: it writes status events.
func TestRollbackActiveVersion(t *testing.T) {
	dsn := os.Getenv("LEARNING_TEST_DSN")
	if dsn == "" {
		t.Skip("LEARNING_TEST_DSN not set")
	}
	db, err := NewDB(dsn)
	if err != nil {
		t.Skipf("scratch database unavailable: %v", err)
	}
	defer db.Close()
	ctx := context.Background()
	var id int64
	if err := db.QueryRowContext(ctx, `INSERT INTO strategy_versions (name, space_version, params, params_sha256, origin, rationale)
		VALUES ('t::active', 'T', '{"plan":"x"}', repeat('c', 64), 'LEARNER_SPACE', 'test') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	// not active yet -> refused
	if err := db.RollbackActiveVersion(ctx, id, ""); !errors.Is(err, ErrNotActiveVersion) {
		t.Fatalf("want ErrNotActiveVersion, got %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO strategy_status_events (version_id, status, reason, actor) VALUES ($1, 'ACTIVE_PAPER', 'test', 'LEARNER')`, id); err != nil {
		t.Fatal(err)
	}
	s, err := db.GetLearningSummary(ctx)
	if err != nil || s.Active == nil || s.Active.ID != id {
		t.Fatalf("summary should show the active version: %v %+v", err, s)
	}
	if err := db.RollbackActiveVersion(ctx, id, "because"); err != nil {
		t.Fatal(err)
	}
	var status, actor string
	_ = db.QueryRowContext(ctx, `SELECT status, actor FROM strategy_current WHERE id = $1`, id).Scan(&status, &actor)
	if status != "ROLLED_BACK" || actor != "OWNER" {
		t.Fatalf("want ROLLED_BACK by OWNER, got %s by %s", status, actor)
	}
	if err := db.RollbackActiveVersion(ctx, id, ""); !errors.Is(err, ErrNotActiveVersion) {
		t.Fatalf("a second rollback must be refused, got %v", err)
	}
}
