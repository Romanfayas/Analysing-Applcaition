package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/services/breakouts"
	"github.com/halal-equity/backend/internal/services/market"
	"github.com/halal-equity/backend/internal/services/quant"
)

const (
	scanWindowBars = 40
	scanMinBars    = 10
	scanCacheTTL   = 60 * time.Second
	// A trading day's EOD bars are ingested after ~16:00 IST; treat the session as complete from this time.
	eodReadyHour, eodReadyMinute = 16, 30
)

// ScanExclusion explains why an instrument was left out of the scan instead of being scored on bad data.
type ScanExclusion struct {
	Symbol string `json:"symbol"`
	Reason string `json:"reason"`
}

// BreakoutScan is one run of the breakout engine over the universe, with the facts a reader needs to judge it.
type BreakoutScan struct {
	*quant.BreakoutScanResponse
	DataAsOf      *string         `json:"data_as_of"`     // date of the newest daily bar scored
	ComputedAt    time.Time       `json:"computed_at"`    // when this scan ran
	StaleSessions int             `json:"stale_sessions"` // completed trading sessions missing after the newest bar
	DataQuality   string          `json:"data_quality"`   // FRESH | STALE
	Basis         string          `json:"basis"`          // what the scan reads
	Excluded      []ScanExclusion `json:"excluded"`
	bySymbol      map[string]quant.BreakoutResult
}

// For returns one symbol's result, if it was scanned.
func (s *BreakoutScan) For(symbol string) (quant.BreakoutResult, bool) {
	r, ok := s.bySymbol[symbol]
	return r, ok
}

// sessionsBehind counts completed trading sessions that have no daily bar after lastBar. A session counts as complete
// once the EOD ingestion window has opened (16:30 IST), so the scan is not called stale during the trading day.
func sessionsBehind(lastBar, now time.Time) int {
	nowIST := now.In(market.Location)
	last := time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), 0, 0, 0, 0, market.Location)
	if nowIST.Before(time.Date(nowIST.Year(), nowIST.Month(), nowIST.Day(), eodReadyHour, eodReadyMinute, 0, 0, market.Location)) {
		last = last.AddDate(0, 0, -1)
	}
	n := 0
	for d := time.Date(lastBar.Year(), lastBar.Month(), lastBar.Day(), 0, 0, 0, 0, market.Location).AddDate(0, 0, 1); !d.After(last); d = d.AddDate(0, 0, 1) {
		if d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
			continue
		}
		if holiday, _ := market.IsTradingHoliday(d); holiday {
			continue
		}
		n++
	}
	return n
}

// windowProblem returns a reason when a bar window must not be scored. The exchange record is the arbiter: a bar the
// vendor adjusted for a corporate action (so it is not the traded price) or that disagrees with the exchange would
// fabricate a breakout or hide one. Bars with no exchange file yet are not contradicted and are allowed.
func windowProblem(bars []database.ScanBar) string {
	if len(bars) < scanMinBars {
		return fmt.Sprintf("only %d daily bars stored (need at least %d)", len(bars), scanMinBars)
	}
	for _, b := range bars {
		switch b.Classification {
		case "VENDOR_ADJUSTED":
			return fmt.Sprintf("vendor bar of %s is back-adjusted, not the exchange traded price", b.TradeDate.Format("2006-01-02"))
		case "MISMATCH":
			return fmt.Sprintf("vendor bar of %s disagrees with the exchange record", b.TradeDate.Format("2006-01-02"))
		}
	}
	return ""
}

type scanCache struct {
	mu   sync.Mutex
	scan *BreakoutScan
	at   time.Time
}

var sharedScan scanCache

// getBreakoutScan returns a recent scan, recomputing at most once per TTL (concurrent callers share one run).
func getBreakoutScan(ctx context.Context, db *database.DB, q *quant.Client) (*BreakoutScan, error) {
	sharedScan.mu.Lock()
	defer sharedScan.mu.Unlock()
	if sharedScan.scan != nil && time.Since(sharedScan.at) < scanCacheTTL {
		return sharedScan.scan, nil
	}
	s, err := runBreakoutScan(ctx, db, q, time.Now())
	if err != nil {
		return nil, err
	}
	sharedScan.scan, sharedScan.at = s, time.Now()
	return s, nil
}

// RunBreakoutScan runs a fresh scan for callers outside this package (the tracker).
func RunBreakoutScan(ctx context.Context, db *database.DB, q *quant.Client, now time.Time) (*BreakoutScan, error) {
	return runBreakoutScan(ctx, db, q, now)
}

func runBreakoutScan(ctx context.Context, db *database.DB, q *quant.Client, now time.Time) (*BreakoutScan, error) {
	if q == nil {
		return nil, fmt.Errorf("quant engine is not configured")
	}
	symbols, err := db.GetAllActiveSymbols(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch symbols: %w", err)
	}

	var items []quant.StockScanItem
	var excluded []ScanExclusion
	var newest time.Time
	stale := 0
	for _, s := range symbols {
		bars, err := db.GetScanWindow(ctx, s.ID, scanWindowBars)
		if err != nil {
			return nil, err
		}
		if reason := windowProblem(bars); reason != "" {
			excluded = append(excluded, ScanExclusion{Symbol: s.Symbol, Reason: reason})
			continue
		}
		last := bars[len(bars)-1].TradeDate
		if last.After(newest) {
			newest = last
		}
		behind := sessionsBehind(last, now)
		if behind > stale {
			stale = behind
		}
		quality := "FRESH"
		if behind > 0 {
			quality = "STALE"
		}
		it := quant.StockScanItem{Symbol: s.Symbol, Sector: s.Sector, ShariahStatus: db.GetShariahStatusForSymbol(ctx, s.ID), DataQuality: quality}
		for _, b := range bars {
			it.Open, it.High, it.Low, it.Close = append(it.Open, b.Open), append(it.High, b.High), append(it.Low, b.Low), append(it.Close, b.Close)
			it.Volume = append(it.Volume, b.Volume)
		}
		items = append(items, it)
	}

	resp, err := q.ScanBreakouts(ctx, quant.BreakoutScanRequest{Stocks: items})
	if err != nil {
		return nil, fmt.Errorf("quant scanner failed: %w", err)
	}
	out := &BreakoutScan{
		BreakoutScanResponse: resp, ComputedAt: now.UTC(), StaleSessions: stale, DataQuality: "FRESH",
		Basis:    fmt.Sprintf("Daily bars (last %d sessions). Entry zones, stops and targets are computed from the last completed session; the live row on each card shows where the price is now.", scanWindowBars),
		Excluded: excluded, bySymbol: map[string]quant.BreakoutResult{},
	}
	if stale > 0 {
		out.DataQuality = "STALE"
	}
	if !newest.IsZero() {
		d := newest.Format("2006-01-02")
		out.DataAsOf = &d
	}
	if out.Excluded == nil {
		out.Excluded = []ScanExclusion{}
	}
	// Levels come from completed daily bars. The price has moved since, so each result says where it stands now.
	ids := make(map[string]int, len(symbols))
	for _, s := range symbols {
		ids[s.Symbol] = s.ID
	}
	for i := range resp.Candidates {
		c := &resp.Candidates[i]
		if len(c.EntryZone) != 2 || c.EntryZone[0] <= 0 {
			continue
		}
		px, err := db.GetLatestPrice(ctx, ids[c.Symbol])
		if err != nil || px == nil || px.Close <= 0 {
			continue
		}
		pos := breakouts.Locate(breakouts.Levels{EntryLow: c.EntryZone[0], EntryHigh: c.EntryZone[1], Stop: c.InvalidationLevel, Target1: c.Target1, Target2: c.Target2},
			px.Close, c.CurrentPrice, px.Timestamp)
		c.Live = &pos
	}
	for _, c := range resp.Candidates {
		out.bySymbol[c.Symbol] = c
	}
	if len(excluded) > 0 {
		log.Warn().Int("excluded", len(excluded)).Msg("breakout scan excluded instruments")
	}
	return out, nil
}
