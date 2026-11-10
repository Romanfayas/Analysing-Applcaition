package autopaper

import (
	"context"
	"os"
	"testing"

	"github.com/halal-equity/backend/internal/database"
)

// TestAgainstARealDatabase runs the trader end to end on a scratch copy of the real schema (migrations 1-46 applied):
// the real ledger, real fee code and the real reconciliation. Set AUTOPAPER_TEST_DSN to a SUPERUSER DSN of a scratch database; skipped otherwise.
func TestAgainstARealDatabase(t *testing.T) {
	dsn := os.Getenv("AUTOPAPER_TEST_DSN")
	if dsn == "" {
		t.Skip("AUTOPAPER_TEST_DSN not set")
	}
	db, err := database.NewDB(dsn)
	if err != nil {
		t.Skipf("scratch database unavailable: %v", err)
	}
	defer db.Close()
	ctx := context.Background()

	// start clean: this test owns symbol ZZAUTO and every automatic portfolio in its scratch database
	clean := func() {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		// exchange_daily_bars is append-only evidence; this scratch database is disposable, so the superuser bypass is acceptable here.
		if _, err := tx.ExecContext(ctx, `SET LOCAL session_replication_role = replica`); err != nil {
			t.Fatal(err)
		}
		for _, q := range []string{
			`DELETE FROM auto_paper_actions`,
			`DELETE FROM portfolios WHERE kind = 'AUTO_STRATEGY'`,
			`DELETE FROM breakout_tracking WHERE symbol_id IN (SELECT id FROM symbols WHERE symbol = 'ZZAUTO')`,
			`DELETE FROM exchange_daily_bars WHERE ticker = 'ZZAUTO'`,
			`DELETE FROM symbols WHERE symbol = 'ZZAUTO'`,
		} {
			if _, err := tx.ExecContext(ctx, q); err != nil {
				t.Fatalf("cleanup %q: %v", q, err)
			}
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	clean()
	defer clean()

	var sid int
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.QueryRowContext(ctx, `INSERT INTO symbols (symbol, name, exchange, is_active) VALUES ('ZZAUTO', 'Auto test', 'NSE', true) RETURNING id`).Scan(&sid))
	// one source file for provenance is not needed for the bar columns used here
	for i, b := range [][4]float64{{100, 101, 99, 100}, {100, 102, 99, 101}, {101, 112, 100, 111}} {
		d := []string{"2031-03-03", "2031-03-04", "2031-03-05"}[i]
		_, err := db.ExecContext(ctx, `INSERT INTO exchange_daily_bars (symbol_id, trade_date, ticker, isin, series, open, high, low, close, volume, source_file_id)
			VALUES ($1, $2, 'ZZAUTO', 'INE000ZZ0001', 'EQ', $3, $4, $5, $6, 1000000, (SELECT id FROM exchange_source_files LIMIT 1))`, sid, d, b[0], b[1], b[2], b[3])
		must(err)
	}
	var tid int64
	must(db.QueryRowContext(ctx, `INSERT INTO breakout_tracking (symbol_id, signal_date, signal_close, entry_low, entry_high, stop_level, target_1, target_2, atr)
		VALUES ($1, '2031-03-02', 100, 99, 101, 95, 110, 120, 2) RETURNING id`, sid).Scan(&tid))

	w := NewWorker(db, 0)
	specs, err := w.Specs(ctx)
	must(err)
	sp := specs[0]
	// only this test's signal should be traded: the scratch copy may hold older real signals, which are skipped or traded too; assert on ours.
	res, err := Process(ctx, w.store, sp)
	must(err)
	if res.Entries < 1 || res.Exits < 1 {
		t.Fatalf("expected our round trip among the results: %+v", res)
	}
	var buys, sells int
	must(db.QueryRowContext(ctx, `SELECT count(*) FILTER (WHERE trade_type='BUY'), count(*) FILTER (WHERE trade_type='SELL') FROM portfolio_trades WHERE portfolio_id=$1 AND symbol_id=$2`, sp.PortfolioID, sid).Scan(&buys, &sells))
	if buys != 1 || sells != 1 {
		t.Fatalf("one round trip expected, got %d/%d", buys, sells)
	}
	var price float64
	var src string
	must(db.QueryRowContext(ctx, `SELECT price::float8, price_source FROM portfolio_trades WHERE portfolio_id=$1 AND symbol_id=$2 AND trade_type='SELL'`, sp.PortfolioID, sid).Scan(&price, &src))
	if price != 109.95 || src != PriceSourcePlanFill {
		t.Fatalf("sell at %.2f via %s", price, src)
	}

	// idempotent, and the summary reconciles to the ledger
	before, _ := db.AutoActions(ctx, sp.PortfolioID)
	res2, err := Process(ctx, w.store, sp)
	must(err)
	after, _ := db.AutoActions(ctx, sp.PortfolioID)
	if res2.Entries+res2.Exits != 0 || len(before) != len(after) {
		t.Fatalf("second pass must change nothing: %+v", res2)
	}
	sums, err := db.AutoSummaries(ctx)
	must(err)
	found := false
	for _, s := range sums {
		if s.PortfolioID != sp.PortfolioID {
			continue
		}
		found = true
		if !s.Reconciled {
			t.Fatalf("cash %.2f does not reconcile to the ledger (expected %.2f)", s.Cash, s.ExpectedCash)
		}
		var mine *database.AutoClosed
		for i := range s.Closed {
			if s.Closed[i].Symbol == "ZZAUTO" {
				mine = &s.Closed[i]
			}
		}
		if mine == nil || mine.ExitReason != "TARGET" || mine.NetPnl <= 0 || mine.NetPct <= 0 {
			t.Fatalf("closed trade missing or wrong: %+v", mine)
		}
	}
	if !found {
		t.Fatal("portfolio missing from the summary")
	}
}
