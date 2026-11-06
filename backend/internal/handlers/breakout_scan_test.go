package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/quant"
)

func ist(y int, m time.Month, d, h, min int) time.Time {
	return time.Date(y, m, d, h, min, 0, 0, market.Location)
}

func TestSessionsBehind(t *testing.T) {
	friClose := ist(2026, 10, 30, 0, 0)
	// Monday morning: Friday's bar is the latest completed session, so nothing is missing.
	assert.Equal(t, 0, sessionsBehind(friClose, ist(2026, 11, 2, 11, 0)))
	// Monday after the EOD window: Monday's session is complete and has no bar yet -> 1 behind.
	assert.Equal(t, 1, sessionsBehind(friClose, ist(2026, 11, 2, 17, 0)))
	// Monday 16:00 is still inside the ingestion window: not yet called stale.
	assert.Equal(t, 0, sessionsBehind(friClose, ist(2026, 11, 2, 16, 0)))
	// Wednesday evening with Friday's bar: Mon, Tue and Wed are missing (Tue 10 Nov is Diwali but 3 Nov is a normal day).
	assert.Equal(t, 3, sessionsBehind(friClose, ist(2026, 11, 4, 18, 0)))
	// Weekends add nothing.
	assert.Equal(t, 0, sessionsBehind(friClose, ist(2026, 10, 31, 20, 0)))
	assert.Equal(t, 0, sessionsBehind(friClose, ist(2026, 11, 1, 20, 0)))
	// A trading holiday (Diwali Laxmi Pujan 2026-11-10) is not a missing session.
	assert.Equal(t, 0, sessionsBehind(ist(2026, 11, 9, 0, 0), ist(2026, 11, 10, 19, 0)))
}

func bars(n int, class func(i int) string) []database.ScanBar {
	out := make([]database.ScanBar, n)
	for i := range out {
		out[i] = database.ScanBar{TradeDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i), Close: 100, Classification: class(i)}
	}
	return out
}

func TestWindowProblemUsesTheExchangeRecordAsArbiter(t *testing.T) {
	ok := func(int) string { return "NOMINAL_MATCH" }
	assert.Empty(t, windowProblem(bars(40, ok)))
	assert.Empty(t, windowProblem(bars(40, func(int) string { return "UNVERIFIED" })), "no exchange file yet is not a contradiction")
	assert.Empty(t, windowProblem(bars(40, func(i int) string {
		if i > 35 {
			return "NO_EXCHANGE_FILE"
		}
		return "NOMINAL_MATCH"
	})))
	assert.Contains(t, windowProblem(bars(40, func(i int) string {
		if i == 7 {
			return "VENDOR_ADJUSTED"
		}
		return "NOMINAL_MATCH"
	})), "back-adjusted")
	assert.Contains(t, windowProblem(bars(40, func(i int) string {
		if i == 20 {
			return "MISMATCH"
		}
		return "NOMINAL_MATCH"
	})), "disagrees")
	assert.Contains(t, windowProblem(bars(5, ok)), "only 5 daily bars")
}

func TestRunBreakoutScanEndToEndAgainstTheIsolatedDatabase(t *testing.T) {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	db := database.GetIsolatedTestDB(t)

	// Fake quant engine: records what it was asked to score and echoes a candidate per stock.
	var gotItems []quant.StockScanItem
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req quant.BreakoutScanRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotItems = req.Stocks
		resp := quant.BreakoutScanResponse{TotalScreened: len(req.Stocks), StageCounts: map[string]int{}}
		for _, s := range req.Stocks {
			sc := 55.0
			resp.Candidates = append(resp.Candidates, quant.BreakoutResult{Symbol: s.Symbol, Stage: "WATCHLIST", StructureScore: &sc})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	var good, adjusted int
	require.NoError(t, db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&good))
	require.NoError(t, db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'INFY' AND exchange = 'NSE'`).Scan(&adjusted))

	const fileDay = "1999-03-01"
	cleanup := func() {
		// Evidence tables are append-only by trigger; the isolated test database bypasses it for its own cleanup.
		_, _ = db.Exec(`BEGIN; SET LOCAL session_replication_role = replica;
			DELETE FROM exchange_daily_bars WHERE source_file_id IN (SELECT id FROM exchange_source_files WHERE trade_date = '` + fileDay + `');
			DELETE FROM exchange_source_files WHERE trade_date = '` + fileDay + `';
			DELETE FROM ohlcv_daily WHERE symbol_id IN (` + itoa(good) + `, ` + itoa(adjusted) + `) AND source = 'yahoo_finance'; COMMIT;`)
	}
	cleanup()
	t.Cleanup(cleanup)

	var fileID int64
	require.NoError(t, db.QueryRow(`INSERT INTO exchange_source_files (trade_date, document_type, issuing_authority, source_url, http_status, byte_size,
		sha256, tls_peer_subject, tls_peer_issuer, tls_cert_sha256, retrieved_at, rows_total, rows_loaded, parser_version)
		VALUES ($1,'NSE_CM_BHAVCOPY_UDIFF','NSE_INDIA','https://nsearchives.nseindia.com/t',200,1,$2,'s','i',$2,now(),1,1,'t') RETURNING id`,
		fileDay, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").Scan(&fileID))

	// 30 weekday bars ending Fri 2001-06-29 (a window no real data can overlap); INFY's vendor bars are exactly half
	// the exchange's traded price, as a split-adjusted vendor series would be.
	end := time.Date(2001, 6, 29, 0, 0, 0, 0, market.Location)
	n := 0
	for d := end; n < 30; d = d.AddDate(0, 0, -1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		day := d.Format("2006-01-02")
		for sym, vendor := range map[int]float64{good: 100, adjusted: 50} {
			_, err := db.Exec(`INSERT INTO ohlcv_daily (symbol_id, timestamp, open, high, low, close, volume, adjusted_close, source)
				VALUES ($1, ($2::date + time '03:45') AT TIME ZONE 'UTC', $3, $3 * 1.01, $3 * 0.99, $3, 1000, NULL, 'yahoo_finance')
				ON CONFLICT DO NOTHING`, sym, day, vendor)
			require.NoError(t, err)
			_, err = db.Exec(`INSERT INTO exchange_daily_bars (symbol_id, trade_date, ticker, isin, series, open, high, low, close, volume, source_file_id)
				VALUES ($1, $2::date, 'T', 'I', 'EQ', 100, 101, 99, 100, 1000, $3) ON CONFLICT DO NOTHING`, sym, day, fileID)
			require.NoError(t, err)
		}
		n++
	}

	scan, err := runBreakoutScan(context.Background(), db, quant.NewClient(srv.URL), end.Add(17*time.Hour))
	require.NoError(t, err)

	scored := map[string]quant.StockScanItem{}
	for _, it := range gotItems {
		scored[it.Symbol] = it
	}
	require.Contains(t, scored, "TCS")
	assert.Len(t, scored["TCS"].Close, 30)
	assert.NotContains(t, scored, "INFY", "a back-adjusted window must never be scored")
	var reason string
	for _, e := range scan.Excluded {
		if e.Symbol == "INFY" {
			reason = e.Reason
		}
	}
	assert.Contains(t, reason, "back-adjusted")
	res, ok := scan.For("TCS")
	require.True(t, ok)
	require.NotNil(t, res.StructureScore)
	assert.Equal(t, 55.0, *res.StructureScore)
	require.NotNil(t, scan.DataAsOf)
	assert.Equal(t, "2001-06-29", *scan.DataAsOf)
	assert.Equal(t, "FRESH", scan.DataQuality)
	assert.Equal(t, 0, scan.StaleSessions)

	// The same data judged a week later is stale, and that is reported rather than hidden.
	stale, err := runBreakoutScan(context.Background(), db, quant.NewClient(srv.URL), end.AddDate(0, 0, 7).Add(17*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, "STALE", stale.DataQuality)
	assert.Equal(t, 5, stale.StaleSessions)
	for _, it := range gotItems {
		assert.Equal(t, "STALE", it.DataQuality, "the engine is told the inputs are stale")
	}
}

func itoa(i int) string { return strconv.Itoa(i) }
