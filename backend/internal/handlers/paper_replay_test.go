package handlers

import (
	"encoding/json"
	"github.com/google/uuid"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/database"
)

// seedPastSession gives TCS two finished 1-minute bars on a fixed past Wednesday (2001-03-07, IST), far from any
// real stored data, so replay results can be asserted exactly.
func seedPastSession(t *testing.T, f *paperFixture) {
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	for ts, px := range map[string]float64{"2001-03-07 10:00:00+05:30": 111.11, "2001-03-07 10:30:00+05:30": 122.22} {
		_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
			VALUES ($1, $2, 1, $3, $3, $3, $3, 1, 'p1925_test')`, tcs, ts, px)
		require.NoError(t, err)
	}
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1925_test'`) })
}

func decodeTrade(t *testing.T, raw json.RawMessage) database.PortfolioTrade {
	var body struct {
		Trade database.PortfolioTrade `json:"trade"`
	}
	require.NoError(t, json.Unmarshal(raw, &body))
	return body.Trade
}

func TestPaper_PriceAtUsesOnlyFinishedBars(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedPastSession(t, f)
	get := func(at string) (int, envelope) {
		rec, env := f.call(u, http.MethodGet, "/price-at?symbol=TCS&at="+url.QueryEscape(at), nil)
		return rec.Code, env
	}

	code, env := get("2001-03-07T10:05")
	require.Equal(t, http.StatusOK, code, env.Error)
	var out struct {
		Price database.PricePoint `json:"price"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &out))
	assert.InDelta(t, 111.11, out.Price.Price, 1e-9)
	assert.Equal(t, database.PriceSourceIntraday1M, out.Price.Source)

	// The 10:30 bar has not finished at 10:30:00, so it must not be used (no lookahead). Across a feed gap the answer is the
	// last valid bar, and the note says how old it is.
	code, env = get("2001-03-07T10:30")
	require.Equal(t, http.StatusOK, code, env.Error)
	require.NoError(t, json.Unmarshal(env.Data, &out))
	assert.InDelta(t, 111.11, out.Price.Price, 1e-9, "never the bar still forming")
	assert.Contains(t, out.Price.Note, "feed gap")

	code, _ = get("2001-03-07T11:45")
	assert.Equal(t, http.StatusUnprocessableEntity, code, "past the gap limit there is no honest price")

	code, _ = get("2001-03-07T10:31")
	assert.Equal(t, http.StatusOK, code, "the 10:30 bar finished at 10:31")

	for name, at := range map[string]string{"weekend": "2001-03-10T11:00", "before open": "2001-03-07T09:00", "future": "2999-01-01"} {
		code, _ = get(at)
		assert.GreaterOrEqual(t, code, 400, name)
	}
}

func TestPaper_BackdatedOrderRecordsPriceBasisAndKeepsLedgerChronological(t *testing.T) {
	t.Setenv("PAPER_SHARIAH_GATE_ENABLED", "")
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedPastSession(t, f)

	rec, env := f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 10, "execute_at": "2001-03-07T10:05"})
	require.Equal(t, http.StatusCreated, rec.Code, env.Error)
	tr := decodeTrade(t, env.Data)
	assert.InDelta(t, 111.11, tr.Price, 1e-9)
	assert.True(t, tr.IsBackdated)
	require.NotNil(t, tr.PriceSource)
	assert.Equal(t, database.PriceSourceIntraday1M, *tr.PriceSource)
	assert.Equal(t, 2001, tr.TradeDate.Year(), "the ledger carries the replay time, not today")

	// A later replay time is fine.
	rec, env = f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "SELL", "shares": 10, "execute_at": "2001-03-07T10:31"})
	require.Equal(t, http.StatusCreated, rec.Code, env.Error)
	assert.InDelta(t, 122.22, decodeTrade(t, env.Data).Price, 1e-9)

	// An order dated before existing trades goes into its place and the ledger is replayed around it.
	rec, env = f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 5, "execute_at": "2001-03-07T10:05"})
	require.Equal(t, http.StatusCreated, rec.Code, env.Error)

	// But it cannot make history impossible: selling 5 before anything was bought would have been a short sale.
	rec, env = f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "SELL", "shares": 5, "execute_at": "2001-03-07T10:04"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "short selling")

	rec, env = f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 1, "execute_at": "2999-01-01"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "future")

	rec, env = f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 1, "execute_at": "not a time"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "could not read time")
}

func TestPaper_SimulateIsReadOnlyAndChargesCosts(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedPastSession(t, f)

	rec, env := f.call(u, http.MethodPost, "/simulate",
		map[string]any{"symbol": "TCS", "shares": 100, "from": "2001-03-07T10:05", "to": "2001-03-07T10:31"})
	require.Equal(t, http.StatusOK, rec.Code, env.Error)
	var out struct {
		Invested   float64  `json:"invested"`
		Proceeds   float64  `json:"proceeds"`
		NetPnL     float64  `json:"net_pnl"`
		NetPct     float64  `json:"net_return_pct"`
		Warnings   []string `json:"warnings"`
		Disclaimer string   `json:"disclaimer"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &out))
	gross := (122.22 - 111.11) * 100
	assert.Less(t, out.NetPnL, gross, "fees must reduce the result")
	assert.Greater(t, out.NetPnL, gross-100, "but only by plausible delivery costs")
	assert.InDelta(t, out.NetPnL/out.Invested*100, out.NetPct, 1e-9)
	assert.NotEmpty(t, out.Warnings)

	_, env = f.call(u, http.MethodGet, "/orders", nil)
	var trades []database.PortfolioTrade
	require.NoError(t, json.Unmarshal(env.Data, &trades))
	assert.Empty(t, trades, "simulation must not touch the ledger")

	rec, _ = f.call(u, http.MethodPost, "/simulate",
		map[string]any{"symbol": "TCS", "shares": 1, "from": "2001-03-07T10:31", "to": "2001-03-07T10:05"})
	assert.Equal(t, http.StatusBadRequest, rec.Code, "to must be after from")
}

// After the close, before the official daily close is loaded, the last finished intraday bar is the honest close.
func TestPaper_AfterCloseFallsBackToLastTradeOfTheSession(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
		VALUES ($1, '2001-03-07 15:29:00+05:30', 1, 133.33, 133.33, 133.33, 133.33, 1, 'p1925_test')`, tcs)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1925_test'`) })

	rec, env := f.call(u, http.MethodGet, "/price-at?symbol=TCS&at="+url.QueryEscape("2001-03-07T21:22"), nil)
	require.Equal(t, http.StatusOK, rec.Code, env.Error)
	var out struct {
		Price database.PricePoint `json:"price"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &out))
	assert.InDelta(t, 133.33, out.Price.Price, 1e-9)
	assert.Equal(t, database.PriceSourceIntraday1M, out.Price.Source)
	assert.Contains(t, out.Price.Note, "official daily close is not loaded")
}

// seedDropSession: a 111.11 entry bar, a bar that trades down to 100, and a recovery bar to 122.22.
func seedDropSession(t *testing.T, f *paperFixture) {
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	for _, b := range []struct {
		ts         string
		o, h, l, c float64
	}{
		{"2001-03-07 10:00:00+05:30", 111.11, 111.11, 111.11, 111.11},
		{"2001-03-07 10:10:00+05:30", 110, 110, 100, 101},
		{"2001-03-07 10:30:00+05:30", 122.22, 122.22, 122.22, 122.22},
	} {
		_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
			VALUES ($1, $2, 1, $3, $4, $5, $6, 1, 'p1925_test')`, tcs, b.ts, b.o, b.h, b.l, b.c)
		require.NoError(t, err)
	}
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1925_test'`) })
}

type simOut struct {
	ExitReason string  `json:"exit_reason"`
	NetPnL     float64 `json:"net_pnl"`
	Exit       struct {
		Price float64 `json:"price"`
	} `json:"exit"`
	Protective struct {
		Without *struct {
			NetPnL float64 `json:"net_pnl"`
		} `json:"without_protection"`
	} `json:"protective_orders"`
	SellCosts struct {
		TotalFees float64 `json:"total_fees"`
	} `json:"sell_costs"`
	Slippage struct {
		Total float64 `json:"total_bps_each_side"`
	} `json:"slippage"`
	Benchmark map[string]any `json:"benchmark"`
}

func simulate(t *testing.T, f *paperFixture, u uuid.UUID, body map[string]any) (int, simOut, string) {
	rec, env := f.call(u, http.MethodPost, "/simulate", body)
	var out simOut
	if rec.Code == http.StatusOK {
		require.NoError(t, json.Unmarshal(env.Data, &out))
	}
	return rec.Code, out, env.Error
}

func TestPaper_SimulateStopLossIsTestedOnTheBarsThatFollowed(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedDropSession(t, f)
	base := map[string]any{"symbol": "TCS", "shares": 100, "from": "2001-03-07T10:05", "to": "2001-03-07T10:31"}

	code, plain, msg := simulate(t, f, u, base)
	require.Equal(t, http.StatusOK, code, msg)
	assert.Equal(t, "END_OF_WINDOW", plain.ExitReason)
	assert.InDelta(t, 122.22, plain.Exit.Price, 1e-9)

	stopped := map[string]any{"stop_loss_pct": 5}
	for k, v := range base {
		stopped[k] = v
	}
	code, st, msg := simulate(t, f, u, stopped)
	require.Equal(t, http.StatusOK, code, msg)
	assert.Equal(t, "STOP_LOSS", st.ExitReason)
	assert.InDelta(t, 111.11*0.95, st.Exit.Price, 1e-6, "filled at the stop level, not at the later recovery")
	assert.Less(t, st.NetPnL, 0.0)
	require.NotNil(t, st.Protective.Without, "the unprotected outcome is shown for comparison")
	assert.Greater(t, st.Protective.Without.NetPnL, 0.0)

	// A target the price never reaches before the end changes nothing.
	far := map[string]any{"target_pct": 90}
	for k, v := range base {
		far[k] = v
	}
	_, same, _ := simulate(t, f, u, far)
	assert.Equal(t, "END_OF_WINDOW", same.ExitReason)
}

func TestPaper_SimulateIntradayProductAndValidation(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedPastSession(t, f)
	body := func(over map[string]any) map[string]any {
		m := map[string]any{"symbol": "TCS", "shares": 100, "from": "2001-03-07T10:05", "to": "2001-03-07T10:31"}
		for k, v := range over {
			m[k] = v
		}
		return m
	}
	_, delivery, _ := simulate(t, f, u, body(nil))
	code, intraday, msg := simulate(t, f, u, body(map[string]any{"product": "INTRADAY"}))
	require.Equal(t, http.StatusOK, code, msg)
	assert.Less(t, intraday.SellCosts.TotalFees, delivery.SellCosts.TotalFees, "same-day STT (0.025%) and no DP charge cost less than delivery")
	assert.Greater(t, delivery.Slippage.Total, 4.99, "base slippage is always applied")
	assert.Contains(t, intraday.Benchmark["note"], "same-day", "index levels are daily, so no comparison is invented for a same-day trade")

	for name, over := range map[string]map[string]any{
		"overnight intraday": {"product": "INTRADAY", "to": "2001-03-08T11:00"},
		"unknown product":    {"product": "MARGIN"},
		"negative stop":      {"stop_loss_pct": -1},
		"stop of 100%":       {"stop_loss_pct": 100},
	} {
		code, _, _ := simulate(t, f, u, body(over))
		assert.Equal(t, http.StatusBadRequest, code, name)
	}
}
