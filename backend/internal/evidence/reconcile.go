package evidence

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"time"
)

// FeedURL is NSE's corporate-actions feed for one symbol over a window.
func FeedURL(symbol string, from, to time.Time) string {
	q := url.Values{}
	q.Set("index", "equities")
	q.Set("symbol", symbol)
	q.Set("from_date", from.Format("02-01-2006"))
	q.Set("to_date", to.Format("02-01-2006"))
	return "https://www.nseindia.com/api/corporates-corporateActions?" + q.Encode()
}

// Finding is one reconciliation result row.
type Finding struct {
	SymbolID       int
	Symbol         string
	Type           string
	Class          string
	ExDate         *time.Time
	FeedSubject    string
	CanonicalEvent string
	ArtifactID     *int
	ImpliedFactor  *float64
	ObservedRatio  *float64
	Detail         map[string]any
}

type canonicalEvent struct {
	eventID, actionType, subject, status string
	ex                                   time.Time
	dividend                             *float64
}

// Reconcile compares canonical events with the newest NSE feed artifact of each symbol and with the
// vendor/exchange price ratios, then records the findings as one append-only run. It writes nothing else.
func Reconcile(ctx context.Context, db *sql.DB, from, to time.Time) (runID int64, summary map[string]int, err error) {
	syms, err := symbolsWithHistory(ctx, db)
	if err != nil {
		return 0, nil, err
	}
	var findings []Finding
	for _, s := range syms {
		f, err := reconcileSymbol(ctx, db, s.id, s.symbol, from, to)
		if err != nil {
			return 0, nil, fmt.Errorf("%s: %w", s.symbol, err)
		}
		findings = append(findings, f...)
	}

	summary = map[string]int{"symbols": len(syms)}
	for _, f := range findings {
		summary[f.Type]++
		if f.Type == "FEED_ACTION_MISSING_FROM_CANONICAL" && f.Class != ClassDividend {
			summary["MISSING_PRICE_CHANGING_ACTIONS"]++
		}
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, err
	}
	defer tx.Rollback()
	sj, _ := json.Marshal(summary)
	if err := tx.QueryRowContext(ctx, `INSERT INTO ca_reconciliation_runs (window_from, window_to, summary) VALUES ($1, $2, $3) RETURNING id`,
		from, to, sj).Scan(&runID); err != nil {
		return 0, nil, err
	}
	for _, f := range findings {
		dj, _ := json.Marshal(f.Detail)
		if _, err := tx.ExecContext(ctx, `INSERT INTO ca_reconciliation_findings (run_id, symbol_id, finding_type, action_class, ex_date,
			feed_subject, canonical_event_id, artifact_id, implied_price_factor, observed_vendor_ratio, detail)
			VALUES ($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11)`,
			runID, f.SymbolID, f.Type, f.Class, f.ExDate, f.FeedSubject, f.CanonicalEvent, f.ArtifactID,
			f.ImpliedFactor, f.ObservedRatio, dj); err != nil {
			return 0, nil, fmt.Errorf("insert finding: %w", err)
		}
	}
	return runID, summary, tx.Commit()
}

type symRow struct {
	id     int
	symbol string
}

func symbolsWithHistory(ctx context.Context, db *sql.DB) ([]symRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.symbol FROM symbols s
		WHERE COALESCE(s.metadata->>'user_added', 'false') <> 'true'
		  AND (EXISTS (SELECT 1 FROM ohlcv_daily d WHERE d.symbol_id = s.id)
		       OR EXISTS (SELECT 1 FROM corporate_action_events e WHERE e.symbol_id = s.id))
		ORDER BY s.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []symRow
	for rows.Next() {
		var r symRow
		if err := rows.Scan(&r.id, &r.symbol); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func reconcileSymbol(ctx context.Context, db *sql.DB, symID int, symbol string, from, to time.Time) ([]Finding, error) {
	var out []Finding
	day := func(t time.Time) *time.Time {
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &d
	}

	// newest NSE feed artifact for this symbol
	var artID int
	var payload string
	err := db.QueryRowContext(ctx, `
		SELECT id, raw_payload FROM corporate_action_source_artifacts
		WHERE source_url LIKE 'https://www.nseindia.com/api/corporates-corporateActions?%'
		  AND source_url LIKE '%symbol=' || replace(replace($1, '&', '%26'), ' ', '+') || '&%'
		ORDER BY id DESC LIMIT 1`, symbol).Scan(&artID, &payload)
	feedOK := err == nil
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	var feed []Classified
	if feedOK {
		var raw []FeedAction
		if err := json.Unmarshal([]byte(payload), &raw); err != nil {
			feedOK = false
		} else {
			for _, a := range raw {
				c := Classify(a)
				if c.ParseableExDte && !c.Ex.Before(from) && !c.Ex.After(to) {
					feed = append(feed, c)
				}
			}
		}
	}
	if !feedOK {
		out = append(out, Finding{SymbolID: symID, Symbol: symbol, Type: "FEED_UNAVAILABLE",
			Detail: map[string]any{"reason": "no parseable NSE corporate-actions feed artifact for this symbol"}})
	}

	events, err := canonicalEvents(ctx, db, symID)
	if err != nil {
		return nil, err
	}
	ratios, err := dailyRatios(ctx, db, symID)
	if err != nil {
		return nil, err
	}

	used := map[int]bool{}
	sibling := map[string]int{}
	for _, e := range events {
		sibling[e.ex.Format("2006-01-02")]++
	}
	var artPtr *int
	if feedOK {
		artPtr = &artID
	}
	for _, e := range events {
		if IsParserFragment(e.actionType, e.subject, sibling[e.ex.Format("2006-01-02")] > 1) {
			out = append(out, Finding{SymbolID: symID, Symbol: symbol, Type: "PARSER_FRAGMENT", Class: ClassOther,
				ExDate: day(e.ex), CanonicalEvent: e.eventID,
				Detail: map[string]any{"subject": e.subject, "verification_status": e.status,
					"reason": "short token split from another event's text on the same ex-date; not a corporate action"}})
			continue
		}
		if !feedOK {
			continue
		}
		cls := CanonicalClass(e.actionType)
		// A single exchange entry can state several components (interim + special dividend on one ex-date),
		// so a feed entry may confirm more than one canonical event.
		match := -1
		for i, c := range feed {
			if c.Class == cls && c.Ex.Equal(*day(e.ex)) {
				match = i
				break
			}
		}
		if match < 0 {
			detail := map[string]any{"subject": e.subject, "verification_status": e.status}
			if cls == ClassOther {
				detail["note"] = "non-price notice (e.g. ticker change); NSE's corporate-actions feed does not list these"
			}
			if near := nearestSameClass(feed, cls, e.ex); near != nil {
				detail["nearest_feed_ex_date"] = near.Ex.Format("2006-01-02")
				detail["nearest_feed_days_off"] = int(near.Ex.Sub(*day(e.ex)).Hours() / 24)
				detail["nearest_feed_subject"] = near.Subject
				if e.dividend != nil {
					detail["canonical_amount"] = *e.dividend
					detail["nearest_feed_amounts"] = DividendAmounts(near.Subject)
				}
			}
			out = append(out, Finding{SymbolID: symID, Symbol: symbol, Type: "CANONICAL_NOT_IN_FEED", Class: cls,
				ExDate: day(e.ex), CanonicalEvent: e.eventID, ArtifactID: artPtr, Detail: detail})
			continue
		}
		used[match] = true
		c := feed[match]
		detail := map[string]any{"verification_status": e.status, "operator_review_required": true}
		if e.dividend != nil {
			amounts := DividendAmounts(c.Subject)
			ok := false
			for _, a := range amounts {
				ok = ok || abs(a-*e.dividend) < 0.005
			}
			detail["dividend_canonical"], detail["dividend_feed_amounts"], detail["dividend_amount_matches"] = *e.dividend, amounts, ok
		}
		out = append(out, Finding{SymbolID: symID, Symbol: symbol, Type: "CANONICAL_CONFIRMED_BY_FEED", Class: cls,
			ExDate: day(c.Ex), FeedSubject: c.Subject, CanonicalEvent: e.eventID, ArtifactID: artPtr, Detail: detail})
	}

	// Feed actions the platform never recorded. For price-changing ones, compare the factor the
	// subject implies with the step in vendor/exchange close ratios across the ex-date.
	for i, c := range feed {
		if used[i] {
			continue
		}
		f := Finding{SymbolID: symID, Symbol: symbol, Type: "FEED_ACTION_MISSING_FROM_CANONICAL", Class: c.Class,
			ExDate: day(c.Ex), FeedSubject: c.Subject, ArtifactID: artPtr, ImpliedFactor: c.PriceFactor,
			Detail: map[string]any{"price_changing": c.PriceChanging}}
		if c.PriceChanging {
			// What the exchange's own traded prices show across the ex-date (close on ex-date / prior close).
			// Consistency within normal daily movement is supporting evidence only, never verification.
			if step := exchangePriceStep(ctx, db, symID, c.Ex); step != nil {
				f.Detail["exchange_price_step"] = *step
				if comb := combinedFactor(feed, c.Ex); comb != nil {
					f.Detail["combined_same_day_factor"] = *comb
					f.Detail["exchange_step_consistent_with_factor"] = abs(*step / *comb - 1) < 0.10
				}
			}
		}
		if c.PriceChanging {
			if step, n := ratioStep(ratios, c.Ex); step != nil {
				f.ObservedRatio = step
				f.Detail["ratio_observations"] = n
				if comb := combinedFactor(feed, c.Ex); comb != nil {
					f.Detail["factor_matches_observed_vendor_adjustment"] = abs(*step-*comb) < 0.0005
				}
			} else {
				f.Detail["note"] = "no exchange bars on both sides of the ex-date yet; load bhavcopies to cross-check"
			}
		}
		out = append(out, f)
	}
	return out, nil
}

func canonicalEvents(ctx context.Context, db *sql.DB, symID int) ([]canonicalEvent, error) {
	rows, err := db.QueryContext(ctx, `SELECT event_id, action_type, source_subject, verification_status, ex_date, dividend_amount::float8
		FROM corporate_action_events WHERE symbol_id = $1 ORDER BY ex_date, id`, symID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []canonicalEvent
	for rows.Next() {
		var e canonicalEvent
		var div sql.NullFloat64
		if err := rows.Scan(&e.eventID, &e.actionType, &e.subject, &e.status, &e.ex, &div); err != nil {
			return nil, err
		}
		if div.Valid {
			e.dividend = &div.Float64
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

type ratioObs struct {
	d     time.Time
	ratio float64
}

func dailyRatios(ctx context.Context, db *sql.DB, symID int) ([]ratioObs, error) {
	rows, err := db.QueryContext(ctx, `SELECT trade_date, close_ratio::float8 FROM evidence.v_daily_bar_reconciliation
		WHERE symbol_id = $1 AND close_ratio IS NOT NULL ORDER BY trade_date`, symID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ratioObs
	for rows.Next() {
		var o ratioObs
		if err := rows.Scan(&o.d, &o.ratio); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// ratioStep is median(vendor/exchange ratio over the 10 sessions before ex) divided by the same median over
// the 10 sessions from ex onwards: the adjustment the vendor applied at that event.
func ratioStep(obs []ratioObs, ex time.Time) (*float64, int) {
	var before, after []float64
	for _, o := range obs {
		if o.d.Before(ex) {
			before = append(before, o.ratio)
		} else {
			after = append(after, o.ratio)
		}
	}
	if len(before) == 0 || len(after) == 0 {
		return nil, 0
	}
	if len(before) > 10 {
		before = before[len(before)-10:]
	}
	if len(after) > 10 {
		after = after[:10]
	}
	a := median(after)
	if a == 0 {
		return nil, 0
	}
	v := median(before) / a
	return &v, len(before) + len(after)
}

func median(x []float64) float64 {
	c := append([]float64(nil), x...)
	sort.Float64s(c)
	n := len(c)
	if n%2 == 1 {
		return c[n/2]
	}
	return (c[n/2-1] + c[n/2]) / 2
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// exchangePriceStep is the exchange's own close on the ex-date divided by its close for the prior session.
func exchangePriceStep(ctx context.Context, db *sql.DB, symID int, ex time.Time) *float64 {
	var exClose, priorClose sql.NullFloat64
	err := db.QueryRowContext(ctx, `SELECT
		(SELECT close::float8 FROM exchange_daily_bars WHERE symbol_id = $1 AND trade_date = $2),
		(SELECT close::float8 FROM exchange_daily_bars WHERE symbol_id = $1 AND trade_date < $2 ORDER BY trade_date DESC LIMIT 1)`,
		symID, ex).Scan(&exClose, &priorClose)
	if err != nil || !exClose.Valid || !priorClose.Valid || priorClose.Float64 == 0 {
		return nil
	}
	v := exClose.Float64 / priorClose.Float64
	return &v
}

// combinedFactor multiplies every stated price factor on one ex-date (e.g. a bonus and a split together).
// nil when any price-changing action that day has no stated factor (a demerger's ratio is never guessed).
func combinedFactor(feed []Classified, ex time.Time) *float64 {
	f, any := 1.0, false
	for _, c := range feed {
		if !c.PriceChanging || !c.Ex.Equal(ex) {
			continue
		}
		if c.PriceFactor == nil {
			return nil
		}
		f *= *c.PriceFactor
		any = true
	}
	if !any {
		return nil
	}
	return &f
}

func nearestSameClass(feed []Classified, cls string, ex time.Time) *Classified {
	var best *Classified
	var bestGap float64
	for i := range feed {
		if feed[i].Class != cls {
			continue
		}
		gap := abs(feed[i].Ex.Sub(ex).Hours())
		if best == nil || gap < bestGap {
			best, bestGap = &feed[i], gap
		}
	}
	return best
}
