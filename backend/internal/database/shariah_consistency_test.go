package database_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

// requireShariahVerdictsMatchTheirNumbers replaces the earlier "exactly 0 PASS / 20 FAIL / 31 DATA_INSUFFICIENT" counts.
// Those pinned a defect (no stock could ever pass because interest income and receivables were never fetched). The
// real invariant is that every stored verdict is explained by its own figures:
//   - PASS needs a permissible business and all four ratios present and within the rule set's limits;
//   - FAIL needs a prohibited business or a breached ratio;
//   - DATA_INSUFFICIENT must not be a hidden PASS: some input has to be genuinely missing, unassessed, or too old;
//   - PASS never claims more certainty than the data supports (source and "unverified" label are recorded).
func requireShariahVerdictsMatchTheirNumbers(t *testing.T, db *database.DB) {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT s.symbol, sc.status::text, sc.business_activity_pass,
		       sc.debt_ratio::float8, sc.cash_deposit_ratio::float8, sc.interest_income_ratio::float8, sc.receivables_ratio::float8,
		       COALESCE(sc.source, ''), COALESCE(sc.details->>'verification_status', ''), COALESCE((sc.details->>'stale')::boolean, false)
		FROM (SELECT DISTINCT ON (symbol_id) * FROM shariah_screenings ORDER BY symbol_id, screening_date DESC, id DESC) sc
		JOIN symbols s ON s.id = sc.symbol_id WHERE s.is_active`)
	require.NoError(t, err)
	defer rows.Close()

	type lim struct{ debt, cash, interest, recv float64 }
	l := lim{0.33, 0.33, 0.05, 0.49}
	n := 0
	for rows.Next() {
		var sym, status, source, verification string
		var stale bool
		var biz *bool
		var debt, cash, interest, recv *float64
		require.NoError(t, rows.Scan(&sym, &status, &biz, &debt, &cash, &interest, &recv, &source, &verification, &stale))
		n++
		complete := debt != nil && cash != nil && interest != nil && recv != nil
		breached := (debt != nil && *debt > l.debt) || (cash != nil && *cash > l.cash) ||
			(interest != nil && *interest > l.interest) || (recv != nil && *recv > l.recv)
		prohibited := biz != nil && !*biz
		switch status {
		case "PASS":
			assert.True(t, biz != nil && *biz, "%s: PASS needs a permissible business", sym)
			assert.True(t, complete, "%s: PASS needs every ratio present", sym)
			assert.False(t, breached, "%s: PASS with a breached ratio", sym)
			assert.Equal(t, "UNVERIFIED_THIRD_PARTY_AGGREGATOR", verification, "%s: a PASS from vendor figures must say so", sym)
		case "FAIL":
			assert.True(t, prohibited || breached, "%s: FAIL needs a prohibited business or a breached ratio", sym)
		case "DATA_INSUFFICIENT":
			assert.False(t, biz != nil && *biz && complete && !breached && !stale, "%s: complete, current, passing figures must not be recorded as DATA_INSUFFICIENT", sym)
		default:
			t.Errorf("%s: unexpected status %q", sym, status)
		}
	}
	require.NoError(t, rows.Err())
	require.Greater(t, n, 0, "no active screenings found")
}
