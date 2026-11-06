package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/halal-equity/backend/internal/auth"
	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/middleware"
)

type paperFixture struct {
	db     *database.DB
	router chi.Router
}

func newPaperFixture(t *testing.T) *paperFixture {
	if os.Getenv("DB_HOST") == "" && os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("set DB_HOST or TEST_DATABASE_URL to run isolated-database handler tests")
	}
	db := database.GetIsolatedTestDB(t)
	return &paperFixture{db: db, router: NewPaperTradingHandler(db, nil).Routes()}
}

func (f *paperFixture) newUser(t *testing.T) uuid.UUID {
	u, err := f.db.CreateUser(context.Background(), fmt.Sprintf("paper-%s@example.test", uuid.NewString()), "P", "x")
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM users WHERE id = $1`, u.ID) }) // cascades the portfolio
	return u.ID
}

func (f *paperFixture) call(user uuid.UUID, method, path string, body any) (*httptest.ResponseRecorder, envelope) {
	var rdr *bytes.Reader
	if body != nil {
		raw, _ := json.Marshal(body)
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &auth.Claims{UserID: user}))
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	var env envelope
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	return rec, env
}

func TestPaper_EachAccountGetsItsOwnPortfolio(t *testing.T) {
	f := newPaperFixture(t)
	a, b := f.newUser(t), f.newUser(t)

	var pa, pb database.PortfolioRecord
	_, env := f.call(a, http.MethodGet, "/portfolios", nil)
	var listA []database.PortfolioRecord
	require.NoError(t, json.Unmarshal(env.Data, &listA))
	require.Len(t, listA, 1)
	pa = listA[0]
	_, env = f.call(b, http.MethodGet, "/portfolios", nil)
	var listB []database.PortfolioRecord
	require.NoError(t, json.Unmarshal(env.Data, &listB))
	pb = listB[0]

	assert.NotEqual(t, pa.ID, pb.ID, "accounts must not share a ledger")
	assert.Equal(t, a, pa.OwnerID)
	assert.EqualValues(t, database.DefaultPaperCapital, pa.CashBalance)

	// Idempotent: asking again returns the same portfolio rather than minting another.
	_, env = f.call(a, http.MethodPost, "/portfolios", nil)
	var again database.PortfolioRecord
	require.NoError(t, json.Unmarshal(env.Data, &again))
	assert.Equal(t, pa.ID, again.ID)
}

func TestPaper_CannotReachAnotherAccountsPortfolioById(t *testing.T) {
	f := newPaperFixture(t)
	a, b := f.newUser(t), f.newUser(t)
	_, env := f.call(a, http.MethodGet, "/portfolios", nil)
	var listA []database.PortfolioRecord
	require.NoError(t, json.Unmarshal(env.Data, &listA))
	aID := listA[0].ID
	f.call(b, http.MethodGet, "/portfolios", nil) // materialise b's own

	for _, path := range []string{"", "/orders", "/positions", "/analytics"} {
		rec, _ := f.call(b, http.MethodGet, fmt.Sprintf("/portfolios/%d%s", aID, path), nil)
		assert.Equal(t, http.StatusNotFound, rec.Code, "GET %s", path)
	}
	rec, _ := f.call(b, http.MethodPost, fmt.Sprintf("/portfolios/%d/orders", aID),
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 1})
	assert.Equal(t, http.StatusNotFound, rec.Code, "placing an order against another account's portfolio")

	rec, _ = f.call(a, http.MethodGet, fmt.Sprintf("/portfolios/%d", aID), nil)
	assert.Equal(t, http.StatusOK, rec.Code, "the owner can still reach it")
}

func TestPaper_ClientCannotChooseFillPrice(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	rec, env := f.call(u, http.MethodPost, "/orders",
		map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 10, "price": 1.0})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "price is set by the server")
}

func TestPaper_OrderInputValidation(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	for name, body := range map[string]map[string]any{
		"zero shares":     {"symbol": "TCS", "trade_type": "BUY", "shares": 0},
		"negative shares": {"symbol": "TCS", "trade_type": "SELL", "shares": -5},
		"absurd shares":   {"symbol": "TCS", "trade_type": "BUY", "shares": 5_000_000},
		"unknown side":    {"symbol": "TCS", "trade_type": "SHORT", "shares": 1},
		"unknown symbol":  {"symbol": "NOPE", "trade_type": "BUY", "shares": 1},
		"unknown field":   {"symbol": "TCS", "trade_type": "BUY", "shares": 1, "leverage": 5},
	} {
		rec, _ := f.call(u, http.MethodPost, "/orders", body)
		assert.GreaterOrEqual(t, rec.Code, 400, name)
		assert.Less(t, rec.Code, 500, name)
	}
}

// seedFreshPrice gives TCS a current intraday bar so an order can reach the ledger.
func seedFreshPrice(t *testing.T, f *paperFixture) {
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
		VALUES ($1, now(), 5, 100, 100, 100, 100, 1, 'p1920_test')`, tcs)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1920_test'`) })
}

// P1.9.24: the owner removed the Shariah restriction for PAPER trading. A BUY of an instrument without a
// certified PASS now executes, and the status at execution is recorded on the trade.
func TestPaper_BuyAllowedForPaperAndShariahStatusRecorded(t *testing.T) {
	t.Setenv("PAPER_SHARIAH_GATE_ENABLED", "")
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedFreshPrice(t, f)

	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 1})
	require.Equal(t, http.StatusCreated, rec.Code, env.Error)
	var body struct {
		Trade database.PortfolioTrade `json:"trade"`
	}
	require.NoError(t, json.Unmarshal(env.Data, &body))
	require.NotNil(t, body.Trade.ShariahStatusAtTrade, "the status is recorded, never hidden")
	assert.NotEqual(t, "PASS", *body.Trade.ShariahStatusAtTrade)

	_, env = f.call(u, http.MethodGet, "/orders", nil)
	var trades []database.PortfolioTrade
	require.NoError(t, json.Unmarshal(env.Data, &trades))
	require.Len(t, trades, 1)
	require.NotNil(t, trades[0].ShariahStatusAtTrade, "persisted, not just returned")
}

// The restriction can still be switched back on by configuration.
func TestPaper_ShariahGateCanBeReEnabled(t *testing.T) {
	t.Setenv("PAPER_SHARIAH_GATE_ENABLED", "true")
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedFreshPrice(t, f)
	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 1})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "Shariah compliance gate blocked")
}

func TestPaper_ShortSellingIsImpossible(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	// A current price is needed so the order reaches the ledger rule rather than the price guard.
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
		VALUES ($1, now(), 5, 100, 100, 100, 100, 1, 'p1920_test')`, tcs)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1920_test'`) })
	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "SELL", "shares": 1})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "Short selling is strictly prohibited")
}

func TestPaper_UnauthenticatedContextIsRejected(t *testing.T) {
	f := newPaperFixture(t)
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/portfolios", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestPaper_StalePriceIsNotUsedForFills(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	var tcs int
	require.NoError(t, f.db.QueryRow(`SELECT id FROM symbols WHERE symbol = 'TCS' AND exchange = 'NSE'`).Scan(&tcs))
	_, err := f.db.Exec(`INSERT INTO ohlcv_intraday (symbol_id, timestamp, interval_minutes, open, high, low, close, volume, source)
		VALUES ($1, now() - interval '9 days', 5, 100, 100, 100, 100, 1, 'p1920_test')`, tcs)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = f.db.Exec(`DELETE FROM ohlcv_intraday WHERE source = 'p1920_test'`) })

	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "SELL", "shares": 1})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "STALE")
}

func TestNormalizeTickerAndPattern(t *testing.T) {
	for in, want := range map[string]string{"pcjeweller": "PCJEWELLER", " PCJEWELLER.NS ": "PCJEWELLER", "nse:tcs": "TCS", "M&M": "M&M", "BAJAJ-AUTO": "BAJAJ-AUTO"} {
		assert.Equal(t, want, normalizeTicker(in), in)
		assert.True(t, tickerPattern.MatchString(normalizeTicker(in)), in)
	}
	for _, bad := range []string{"", "A B", "DROP;TABLE", "../x", "WAYTOOLONGTICKERSYMBOLNAME1", "-X"} {
		assert.False(t, tickerPattern.MatchString(normalizeTicker(bad)), bad)
	}
}

func TestAddStock_RejectsBadInputAndRecognisesCanonicalSymbols(t *testing.T) {
	f := newPaperFixture(t)
	u := f.newUser(t)
	h := NewStocksHandler(f.db, nil)
	post := func(body string) (int, string) {
		req := httptest.NewRequest(http.MethodPost, "/custom", bytes.NewBufferString(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &auth.Claims{UserID: u}))
		rec := httptest.NewRecorder()
		h.Routes().ServeHTTP(rec, req)
		return rec.Code, rec.Body.String()
	}
	code, _ := post(`{"symbol":"DROP TABLE"}`)
	assert.Equal(t, http.StatusUnprocessableEntity, code)
	code, _ = post(`{"symbol":"x","extra":1}`)
	assert.GreaterOrEqual(t, code, 400, "unknown fields are refused")
	code, body := post(`{"symbol":"tcs"}`)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, body, "already_tracked")
}

func TestPaper_RiskRuleStopsAnOversizedPositionButNeverASell(t *testing.T) {
	t.Setenv("PAPER_SHARIAH_GATE_ENABLED", "")
	t.Setenv("PAPER_RISK_MAX_POSITION_PCT", "25")
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedFreshPrice(t, f) // TCS at 100; the account starts with Rs 10,00,000

	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 2600})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, env.Error, "risk rule")
	assert.Contains(t, env.Error, "one stock")

	rec, env = f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 2400})
	require.Equal(t, http.StatusCreated, rec.Code, env.Error)

	// Adding more would breach the cap; selling is always allowed.
	rec, _ = f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 200})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	rec, env = f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "SELL", "shares": 100})
	assert.Equal(t, http.StatusCreated, rec.Code, env.Error)

	rec, env = f.call(u, http.MethodGet, "/risk-rules", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, string(env.Data), "max_position_pct")
}

func TestPaper_RiskRulesCanBeSwitchedOff(t *testing.T) {
	t.Setenv("PAPER_SHARIAH_GATE_ENABLED", "")
	t.Setenv("PAPER_RISK_MAX_POSITION_PCT", "0")
	f := newPaperFixture(t)
	u := f.newUser(t)
	seedFreshPrice(t, f)
	rec, env := f.call(u, http.MethodPost, "/orders", map[string]any{"symbol": "TCS", "trade_type": "BUY", "shares": 9000})
	assert.Equal(t, http.StatusCreated, rec.Code, env.Error)
}
