package database_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

const fakeSHA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func insertFile(tx *sql.Tx, d time.Time, url string) (int64, error) {
	var id int64
	err := tx.QueryRow(`INSERT INTO exchange_source_files (trade_date, document_type, issuing_authority, source_url, http_status,
		byte_size, sha256, tls_peer_subject, tls_peer_issuer, tls_cert_sha256, retrieved_at, rows_total, rows_loaded, parser_version)
		VALUES ($1,'NSE_CM_BHAVCOPY_UDIFF','NSE_INDIA',$2,200,100,$3,'CN=test','CN=test',$3,now(),1,1,'test') RETURNING id`,
		d, url, fakeSHA).Scan(&id)
	return id, err
}

// The runtime role must never be able to manufacture exchange evidence.
func TestP1924_1_RuntimeRoleCannotWriteExchangeEvidence(t *testing.T) {
	appDB := getAppTestDB(t)
	_, err := appDB.Exec(`INSERT INTO exchange_source_files (trade_date, document_type, issuing_authority, source_url, http_status,
		byte_size, sha256, tls_peer_subject, tls_peer_issuer, tls_cert_sha256, retrieved_at, rows_total, rows_loaded, parser_version)
		VALUES ('1999-01-04','NSE_CM_BHAVCOPY_UDIFF','NSE_INDIA','https://nsearchives.nseindia.com/x',200,1,$1,'s','i',$1,now(),1,1,'t')`, fakeSHA)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")
	for _, tbl := range []string{"exchange_daily_bars", "ca_reconciliation_runs", "ca_reconciliation_findings"} {
		_, err = appDB.Exec(`DELETE FROM ` + tbl + ` WHERE false`)
		assert.Error(t, err, tbl)
	}
	var n int
	require.NoError(t, appDB.QueryRow(`SELECT count(*) FROM evidence.v_daily_bar_reconciliation WHERE false`).Scan(&n), "but it can read the reconciliation")
}

func TestP1924_2_EvidenceIsAppendOnlyAndOriginChecked(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	database.WithIsolatedTx(t, db, func(tx *sql.Tx) {
		d := time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)
		_, err := tx.Exec(`SAVEPOINT s`)
		require.NoError(t, err)
		_, err = insertFile(tx, d, "https://example.com/bhav.zip")
		assert.Error(t, err, "only NSE's official archive host is accepted")
		_, _ = tx.Exec(`ROLLBACK TO SAVEPOINT s`)

		id, err := insertFile(tx, d, "https://nsearchives.nseindia.com/content/cm/test.zip")
		require.NoError(t, err)
		_, _ = tx.Exec(`SAVEPOINT s2`)
		_, err = tx.Exec(`UPDATE exchange_source_files SET rows_loaded = 99 WHERE id = $1`, id)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "append-only")
		_, _ = tx.Exec(`ROLLBACK TO SAVEPOINT s2`)
		_, err = tx.Exec(`DELETE FROM exchange_source_files WHERE id = $1`, id)
		assert.ErrorContains(t, err, "append-only")
	})
}

func TestP1924_3_ReconciliationViewClassifiesVendorBars(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	ctx := context.Background()
	database.WithIsolatedTx(t, db, func(tx *sql.Tx) {
		d := time.Date(1999, 1, 4, 0, 0, 0, 0, time.UTC)
		fileID, err := insertFile(tx, d, "https://nsearchives.nseindia.com/content/cm/test.zip")
		require.NoError(t, err)
		ids := map[string]int{}
		for _, s := range []string{"TCS", "RELIANCE", "INFY", "WIPRO"} {
			var id int
			require.NoError(t, tx.QueryRow(`SELECT id FROM symbols WHERE symbol = $1`, s).Scan(&id))
			ids[s] = id
		}
		bar := func(sym string, o, h, l, c float64) {
			_, err := tx.Exec(`INSERT INTO exchange_daily_bars (symbol_id, trade_date, ticker, isin, series, open, high, low, close, volume, source_file_id)
				VALUES ($1,$2,$3,'INE','EQ',$4,$5,$6,$7,1000,$8)`, ids[sym], d, sym, o, h, l, c, fileID)
			require.NoError(t, err)
		}
		vendor := func(sym string, o, h, l, c float64) {
			_, err := tx.Exec(`INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source)
				VALUES ($1, $2::date + time '03:45' AT TIME ZONE 'UTC', $3,$4,$5,$6,1000,NULL,'yahoo_finance')`, ids[sym], d, o, h, l, c)
			require.NoError(t, err)
		}
		bar("TCS", 100, 110, 95, 105)
		vendor("TCS", 100, 110, 95, 105) // identical: nominal match
		bar("RELIANCE", 200, 220, 190, 210)
		vendor("RELIANCE", 100, 110, 95, 105) // every price halved: vendor bonus adjustment
		bar("INFY", 100, 110, 95, 105)
		vendor("INFY", 100, 125, 95, 120) // valid bar, but prices disagree inconsistently: mismatch
		bar("WIPRO", 100, 110, 95, 105)   // exchange has it, vendor does not

		got := map[string]string{}
		rows, err := tx.QueryContext(ctx, `SELECT symbol, classification FROM evidence.v_daily_bar_reconciliation WHERE trade_date = $1`, d)
		require.NoError(t, err)
		for rows.Next() {
			var s, c string
			require.NoError(t, rows.Scan(&s, &c))
			got[s] = c
		}
		rows.Close()
		assert.Equal(t, "NOMINAL_MATCH", got["TCS"])
		assert.Equal(t, "VENDOR_ADJUSTED", got["RELIANCE"])
		assert.Equal(t, "MISMATCH", got["INFY"])
		assert.Equal(t, "MISSING_VENDOR_BAR", got["WIPRO"])
	})
}

// A bar released under an adjustment whose corporate-action event is (no longer) verified must not count as
// research-eligible, even though its adjusted_close is populated. This is how the 7 TCS bars of 2026-10-05..13
// stood after their dividend event was demoted to ARCHIVE_UNAUTHENTICATED.
func TestP1924_4_StrictEligibilityRejectsAdjustmentsWithoutVerifiedEvidence(t *testing.T) {
	db := database.GetIsolatedTestDB(t)
	database.WithIsolatedTx(t, db, func(tx *sql.Tx) {
		var sym int
		require.NoError(t, tx.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS'`).Scan(&sym))
		// Recreate the historical state the write-time guard would no longer allow (superuser, test DB, rolled back).
		_, err := tx.Exec(`SET LOCAL session_replication_role = replica`)
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source) VALUES
			($1, '1999-02-01 03:45+00', 100, 101, 99, 100, 1, 100, 'yahoo_finance'),
			($1, '1999-02-02 03:45+00', 100, 101, 99, 100, 1, 99.4712, 'NSE_CORPORATE_ACTION')`, sym)
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO corporate_action_events (event_id, symbol_id, symbol, action_type, source_subject, ex_date, verification_status)
			VALUES ('TEST:TCS:1999-02-03:CASH_DIVIDEND:1', $1, 'TCS', 'CASH_DIVIDEND', 'test', '1999-02-03', 'ARCHIVE_UNAUTHENTICATED')`, sym)
		require.NoError(t, err)
		_, err = tx.Exec(`INSERT INTO corporate_action_adjustment_factors (symbol_id, event_id, action_type, ex_date, effective_date,
			adjustment_factor, source_reference, total_return_factor, price_factor_applicable, is_verified, dispute_status)
			VALUES ($1, 'TEST:TCS:1999-02-03:CASH_DIVIDEND:1', 'CASH_DIVIDEND', '1999-02-03', '1999-02-03',
			        0.994712, 'test', 0.994712, true, true, 'UNDISPUTED')`, sym)
		if err != nil {
			t.Skipf("factor table shape differs in this database: %v", err)
		}

		eligible := func(day string) bool {
			var ok bool
			require.NoError(t, tx.QueryRow(`SELECT `+database.StrictResearchEligible+` FROM ohlcv_daily d
				WHERE d.symbol_id = $1 AND d.timestamp = ($2::date + time '03:45') AT TIME ZONE 'UTC'`, sym, day).Scan(&ok))
			return ok
		}
		assert.True(t, eligible("1999-02-01"), "a nominal vendor bar is eligible")
		assert.False(t, eligible("1999-02-02"), "an adjustment backed only by an unverified event is not")

		_, err = tx.Exec(`UPDATE corporate_action_events SET verification_status = 'VERIFIED_EXCHANGE_PRIMARY'
			WHERE event_id = 'TEST:TCS:1999-02-03:CASH_DIVIDEND:1'`)
		require.NoError(t, err)
		assert.True(t, eligible("1999-02-02"), "the same bar becomes eligible only once its event is verified")
	})
}
