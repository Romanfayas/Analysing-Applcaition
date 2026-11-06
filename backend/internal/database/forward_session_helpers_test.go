package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forwardSessionBarStep is the number of daily bars appended per trading session
// (one per active instrument in the canonical universe).
const forwardSessionBarStep = 47

// assertForwardSessionGrowth asserts that got == floor + step*N for some integer N >= 0.
//
// Daily EOD ingestion legitimately appends one bar per instrument every trading session,
// so absolute bar counts grow over time. Asserting membership in a hand-maintained
// whitelist breaks on every new session; this asserts the structural invariant instead
// (the historical floor is preserved and growth only happens in whole-session increments).
func assertForwardSessionGrowth(t *testing.T, floor, step, got int, msgAndArgs ...interface{}) bool {
	t.Helper()
	if got >= floor && (got-floor)%step == 0 {
		return true
	}
	return assert.Fail(t,
		fmt.Sprintf("count %d is not %d + N*%d whole sessions", got, floor, step),
		msgAndArgs...)
}

// p1924EvidenceFilter excludes the P1.9.24 operator evidence (NSE feed snapshots and company filings), leaving the
// baseline artifacts that earlier phases locked at exactly 23.
const p1924EvidenceFilter = `parser_version NOT LIKE 'p1.9.24%'`

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// assertArtifactBaselineAndNewEvidence keeps the intent of the "exactly 23 artifacts" invariants (no test fixtures or
// forged evidence leaking into the application database) while allowing legitimate P1.9.24 evidence: the baseline is
// still exactly 23, and every additional artifact is exchange-hosted, checksummed and NOT promoted.
func assertArtifactBaselineAndNewEvidence(t *testing.T, q rowQuerier) {
	t.Helper()
	ctx := context.Background()
	var baseline, baselineDocIDs int
	require.NoError(t, q.QueryRowContext(ctx, `SELECT count(*), count(DISTINCT document_identifier)
		FROM corporate_action_source_artifacts WHERE `+p1924EvidenceFilter).Scan(&baseline, &baselineDocIDs))
	assert.Equal(t, 23, baseline, "baseline corporate-action artifacts must remain exactly 23")
	assert.Equal(t, 23, baselineDocIDs)

	var bad int
	require.NoError(t, q.QueryRowContext(ctx, `SELECT count(*) FROM corporate_action_source_artifacts
		WHERE NOT (`+p1924EvidenceFilter+`) AND NOT (
			validation_status = 'AUTHENTICITY_PENDING'
			AND document_type IN ('EXCHANGE_API_JSON', 'STATUTORY_FILING')
			AND issuing_authority = 'NSE_INDIA'
			AND source_url ~ '^https://(www|nsearchives)\.nseindia\.com/'
			AND sha256_checksum ~ '^[0-9a-f]{64}$')`).Scan(&bad))
	assert.Equal(t, 0, bad, "every non-baseline artifact must be unpromoted, checksummed, exchange-hosted P1.9.24 evidence")
}
