package ingestion

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/models"
)

const onboardDailyLookback = 2 * 365 * 24 * time.Hour

// researchFloor is the first session whose daily bars may be research-eligible. Before it, a vendor's "adjusted
// close equals close" proves nothing (vendors back-adjust closes for splits and bonuses), so a user-added stock's older
// history is stored quarantined, exactly like the canonical universe's.
var researchFloor = time.Date(2026, 10, 5, 0, 0, 0, 0, istLoc)

// ScreenFunc runs the Shariah screening for a stored symbol and returns its status.
type ScreenFunc func(ctx context.Context, ticker string) (string, error)

// ErrAlreadyTracked means the symbol is already part of the platform's universe.
var ErrAlreadyTracked = fmt.Errorf("already tracked")

// ValidateNewSymbol checks that a ticker is a real NSE equity before anything is stored for it.
func (p *Pipeline) ValidateNewSymbol(ticker string) (name string, err error) {
	meta, err := p.yahoo.GetSymbolMeta(ticker)
	if err != nil {
		return "", err
	}
	if meta.InstrumentType != "" && meta.InstrumentType != "EQUITY" {
		return "", fmt.Errorf("%s is a %s, not an equity share", ticker, meta.InstrumentType)
	}
	if meta.Currency != "" && meta.Currency != "INR" {
		return "", fmt.Errorf("%s does not trade in rupees", ticker)
	}
	name = meta.Name
	if name == "" {
		name = ticker
	}
	return name, nil
}

// RegisterNewSymbol adds the instrument (a no-op if it is already tracked) and marks onboarding as running.
func (p *Pipeline) RegisterNewSymbol(ctx context.Context, ticker, name string, by uuid.UUID) (id int, created bool, err error) {
	id, created, err = p.db.AddUserSymbol(ctx, ticker, name, by)
	if err != nil || !created {
		return id, created, err
	}
	return id, true, p.db.SetOnboardStatus(ctx, id, database.OnboardStatus{State: database.OnboardRunning})
}

// OnboardSymbol fetches everything the platform needs to analyse and paper-trade a newly added instrument:
// two years of daily history, recent 1-minute bars, up to 60 sessions of 5-minute bars, and a Shariah screening.
// A step that fails is recorded and the rest still run, so a stock is usable as far as its data goes.
func (p *Pipeline) OnboardSymbol(ctx context.Context, id int, ticker, name string, screen ScreenFunc) database.OnboardStatus {
	st := database.OnboardStatus{State: database.OnboardRunning}
	record := func(step string, err error, detail string) {
		s := database.OnboardStep{Name: step, OK: err == nil, Detail: detail}
		if err != nil {
			s.Detail = err.Error()
		}
		st.Steps = append(st.Steps, s)
		_ = p.db.SetOnboardStatus(ctx, id, st)
		log.Printf("[ONBOARD] %s: %s ok=%v %s", ticker, step, s.OK, s.Detail)
	}

	now := time.Now()
	daily, err := p.storeDailyHistory(ctx, id, ticker, now)
	record("daily_history", err, fmt.Sprintf("%d daily bars", daily))

	rep, err := p.IngestSymbol(ctx, ticker, name)
	n1 := 0
	if rep != nil {
		n1 = rep.CandlesStored
	}
	record("intraday_1m", err, fmt.Sprintf("%d one-minute bars (last ~7 sessions)", n1))

	n5, err := p.backfillSymbolFiveMinute(ctx, database.SymbolRecord{ID: id, Symbol: ticker}, time.Now())
	record("intraday_5m", err, fmt.Sprintf("%d five-minute bars (up to 60 sessions)", n5))

	if screen != nil {
		status, err := screen(ctx, ticker)
		record("shariah_screening", err, "status "+status)
	}

	st.State = database.OnboardDone
	for _, s := range st.Steps {
		if !s.OK && (s.Name == "daily_history" || s.Name == "intraday_1m") {
			st.State = database.OnboardFailed
		}
	}
	_ = p.db.SetOnboardStatus(ctx, id, st)
	return st
}

// storeDailyHistory loads two years of completed daily bars through the same quarantine-aware path as the nightly job.
func (p *Pipeline) storeDailyHistory(ctx context.Context, id int, ticker string, now time.Time) (int, error) {
	dp, err := p.marketProvider.GetDailyCandles(ctx, ticker, "NSE", now.Add(-onboardDailyLookback), now)
	if err != nil {
		return 0, err
	}
	if dp == nil || len(dp.Data) == 0 {
		return 0, fmt.Errorf("no daily history returned")
	}
	recs := make([]database.OHLCVRecord, 0, len(dp.Data))
	for _, c := range dp.Data {
		if !sessionComplete(c.Timestamp, now) {
			continue // today's bar is not final until after the close
		}
		rec := dailyRecord(id, c)
		if c.Timestamp.In(istLoc).Before(researchFloor) {
			rec.AdjClose = nil
		}
		recs = append(recs, rec)
	}
	stats, err := p.db.UpsertOHLCVBatchWithStats(ctx, recs)
	if err != nil {
		return 0, err
	}
	return stats.Inserted + stats.Updated + stats.Unchanged + stats.Quarantined, nil
}

func dailyRecord(symbolID int, c models.OHLCV) database.OHLCVRecord {
	return database.OHLCVRecord{
		SymbolID: symbolID, Timestamp: dailyStamp(c.Timestamp), Open: c.Open, High: c.High, Low: c.Low, Close: c.Close,
		Volume: c.Volume, AdjClose: c.AdjustedClose, Source: "yahoo_finance", QualityStatus: "VALID", IntervalMins: 1440,
	}
}
