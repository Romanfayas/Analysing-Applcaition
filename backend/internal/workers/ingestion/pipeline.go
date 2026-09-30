package ingestion

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"halal-equity/internal/database"
	"halal-equity/internal/providers/marketdata"
	"halal-equity/internal/providers/marketdata/nsedata"
	"halal-equity/internal/providers/marketdata/yahoofinance"
	"halal-equity/internal/quality"
	"halal-equity/internal/notifications"
	"halal-equity/internal/metrics"
)

// Pipeline orchestrates the full data ingestion flow:
// Provider → Validation → Database
type Pipeline struct {
	db               *database.DB
	marketProvider   marketdata.MarketDataProvider
	fundProvider     marketdata.FundamentalDataProvider
	corpActProvider  marketdata.CorporateActionsProvider
	shareProvider    marketdata.ShareholdingProvider
	ipoProvider      marketdata.IPODataProvider
	validator        *quality.Validator
	notifier         *notifications.TelegramNotifier
}

// NewPipeline creates a new ingestion pipeline.
func NewPipeline(db *database.DB, notifier *notifications.TelegramNotifier) *Pipeline {
	yf := yahoofinance.NewProvider()
	nse := nsedata.NewProvider()
	return &Pipeline{
		db:               db,
		marketProvider:   yf,
		fundProvider:     yf,
		corpActProvider:  nse,
		shareProvider:    nse,
		ipoProvider:      nse,
		validator:        quality.NewValidator(),
		notifier:         notifier,
	}
}

// IngestReport summarizes an ingestion run.
type IngestReport struct {
	Symbol       string        `json:"symbol"`
	SymbolID     int           `json:"symbol_id"`
	CandlesFetched int         `json:"candles_fetched"`
	CandlesStored  int         `json:"candles_stored"`
	QualityScore float64       `json:"quality_score"`
	Issues       []string      `json:"issues,omitempty"`
	Duration     time.Duration `json:"duration"`
	Error        string        `json:"error,omitempty"`
}

// IngestSymbol fetches and stores OHLCV data for a single symbol.
// It performs incremental ingestion — only fetches data newer than what's in the DB.
func (p *Pipeline) IngestSymbol(ctx context.Context, symbol, name string) (*IngestReport, error) {
	start := time.Now()
	report := &IngestReport{Symbol: symbol}

	// 1. Ensure symbol exists in DB
	symbolID, err := p.db.UpsertSymbol(ctx, symbol, name, "NSE")
	if err != nil {
		report.Error = err.Error()
		report.Duration = time.Since(start)
		return report, err
	}
	report.SymbolID = symbolID

	// 2. Determine fetch range (incremental)
	var from time.Time
	latestTS, err := p.db.GetLatestOHLCVTimestamp(ctx, symbolID)
	if err != nil || latestTS == nil {
		// No existing data — fetch 2 years
		from = time.Now().AddDate(-2, 0, 0)
	} else {
		// Fetch from the day after the latest candle
		from = latestTS.AddDate(0, 0, 1)
	}
	to := time.Now()

	if from.After(to) {
		report.Duration = time.Since(start)
		return report, nil // Up to date
	}

	// 3. Fetch from Provider with Exponential Backoff
	var dataPoint *marketdata.DataPoint[[]models.OHLCV]
	var err error
	maxRetries := 3
	for attempt := 1; attempt <= maxRetries; attempt++ {
		dataPoint, err = p.marketProvider.GetDailyCandles(ctx, symbol, "NSE", from, to)
		if err == nil {
			break
		}
		
		if err == marketdata.ErrRateLimited || err == marketdata.ErrProviderFail {
			if attempt == maxRetries {
				report.Error = fmt.Sprintf("failed after %d attempts: %v", maxRetries, err)
				report.Duration = time.Since(start)
				return report, err
			}
			backoff := time.Duration(math.Pow(2, float64(attempt))) * time.Second
			log.Printf("[PIPELINE] %s: Rate limited/Failed, retrying in %v (Attempt %d/%d)", symbol, backoff, attempt, maxRetries)
			time.Sleep(backoff)
			continue
		}
		
		// Unrecoverable error
		report.Error = err.Error()
		report.Duration = time.Since(start)
		return report, err
	}

	if dataPoint == nil {
		report.Duration = time.Since(start)
		return report, nil
	}
	candles := dataPoint.Data
	report.CandlesFetched = len(candles)

	if len(candles) == 0 {
		report.Duration = time.Since(start)
		return report, nil
	}

	// 4. Validate data quality
	qualityCandles := make([]quality.OHLCVCandle, len(candles))
	for i, c := range candles {
		qualityCandles[i] = quality.OHLCVCandle{
			Timestamp: c.Timestamp,
			Open:      c.Open,
			High:      c.High,
			Low:       c.Low,
			Close:     c.Close,
			Volume:    c.Volume,
		}
	}
	validationResults := p.validator.ValidateSeries(qualityCandles)
	report.QualityScore = p.validator.QualityScore(validationResults)

	// 5. Convert to DB records (only store VALID and SUSPECT with flags)
	var records []database.OHLCVRecord
	for i, candle := range candles {
		qResult := validationResults[i]

		// Skip duplicates entirely
		if qResult.Status == quality.StatusDuplicate {
			report.Issues = append(report.Issues, fmt.Sprintf("Skipped duplicate at %v", candle.Timestamp))
			continue
		}

		record := database.OHLCVRecord{
			SymbolID:      symbolID,
			Timestamp:     candle.Timestamp,
			Open:          candle.Open,
			High:          candle.High,
			Low:           candle.Low,
			Close:         candle.Close,
			Volume:        candle.Volume,
			AdjClose:      candle.AdjClose,
			Source:        "yahoo_finance",
			QualityStatus: string(qResult.Status),
		}
		records = append(records, record)

		// Track issues
		for _, issue := range qResult.Issues {
			report.Issues = append(report.Issues,
				fmt.Sprintf("%s at %v: %s", symbol, candle.Timestamp.Format("2006-01-02"), issue))
		}
	}

	// 6. Batch upsert to database
	stored, err := p.db.UpsertOHLCVBatch(ctx, records)
	if err != nil {
		report.Error = err.Error()
		report.Duration = time.Since(start)
		return report, err
	}
	report.CandlesStored = stored

	report.Duration = time.Since(start)
	log.Printf("[PIPELINE] %s: fetched=%d stored=%d quality=%.1f%% in %v",
		symbol, report.CandlesFetched, report.CandlesStored, report.QualityScore, report.Duration)

	// Send Telegram alert if quality is very poor
	if report.QualityScore < 90.0 && p.notifier != nil {
		p.notifier.SendAlert(notifications.Alert{
			Type:      notifications.AlertDataQuality,
			Title:     "Data Quality Warning",
			Message:   fmt.Sprintf("Symbol: %s\nQuality Score: %.1f%%\nIssues detected: %d", symbol, report.QualityScore, len(report.Issues)),
			Symbol:    symbol,
			Timestamp: time.Now(),
		})
		metrics.IngestionJobsTotal.WithLabelValues(symbol, "warning").Inc()
	} else {
		metrics.IngestionJobsTotal.WithLabelValues(symbol, "success").Inc()
	}

	metrics.DataQualityScore.WithLabelValues(symbol).Set(report.QualityScore)

	return report, nil
}

// IngestBatch processes multiple symbols with a bounded worker pool.
func (p *Pipeline) IngestBatch(ctx context.Context, symbols map[string]string) []IngestReport {
	var reports []IngestReport
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Bounded concurrency using a semaphore (max 5 concurrent workers)
	concurrencyLimit := 5
	sem := make(chan struct{}, concurrencyLimit)

	for symbol, name := range symbols {
		select {
		case <-ctx.Done():
			return reports
		default:
		}

		wg.Add(1)
		sem <- struct{}{} // Acquire token

		go func(sym, n string) {
			defer wg.Done()
			defer func() { <-sem }() // Release token

			report, _ := p.IngestSymbol(ctx, sym, n)
			if report != nil {
				mu.Lock()
				reports = append(reports, *report)
				mu.Unlock()
			}
		}(symbol, name)
	}

	wg.Wait()
	return reports
}

// IngestNIFTY50 ingests all NIFTY 50 constituent stocks.
func (p *Pipeline) IngestNIFTY50(ctx context.Context) []IngestReport {
	nifty50 := map[string]string{
		"RELIANCE":    "Reliance Industries Ltd",
		"TCS":         "Tata Consultancy Services",
		"HDFCBANK":    "HDFC Bank Ltd",
		"INFY":        "Infosys Ltd",
		"ICICIBANK":   "ICICI Bank Ltd",
		"HINDUNILVR":  "Hindustan Unilever Ltd",
		"ITC":         "ITC Ltd",
		"SBIN":        "State Bank of India",
		"BHARTIARTL":  "Bharti Airtel Ltd",
		"KOTAKBANK":   "Kotak Mahindra Bank",
		"LT":          "Larsen & Toubro Ltd",
		"AXISBANK":    "Axis Bank Ltd",
		"WIPRO":       "Wipro Ltd",
		"ASIANPAINT":  "Asian Paints Ltd",
		"MARUTI":      "Maruti Suzuki India Ltd",
		"TITAN":       "Titan Company Ltd",
		"ULTRACEMCO":  "UltraTech Cement Ltd",
		"SUNPHARMA":   "Sun Pharmaceutical Ind",
		"BAJFINANCE":  "Bajaj Finance Ltd",
		"TATAMOTORS":  "Tata Motors Ltd",
		"HCLTECH":     "HCL Technologies Ltd",
		"NTPC":        "NTPC Ltd",
		"POWERGRID":   "Power Grid Corp",
		"TATASTEEL":   "Tata Steel Ltd",
		"ONGC":        "Oil & Natural Gas Corp",
		"M&M":         "Mahindra & Mahindra Ltd",
		"JSWSTEEL":    "JSW Steel Ltd",
		"ADANIENT":    "Adani Enterprises Ltd",
		"ADANIPORTS":  "Adani Ports & SEZ Ltd",
		"TECHM":       "Tech Mahindra Ltd",
		"DRREDDY":     "Dr Reddys Laboratories",
		"INDUSINDBK":  "IndusInd Bank Ltd",
		"CIPLA":       "Cipla Ltd",
		"NESTLEIND":   "Nestle India Ltd",
		"GRASIM":      "Grasim Industries Ltd",
		"DIVISLAB":    "Divis Laboratories Ltd",
		"BPCL":        "Bharat Petroleum Corp",
		"BRITANNIA":   "Britannia Industries",
		"HEROMOTOCO":  "Hero MotoCorp Ltd",
		"COALINDIA":   "Coal India Ltd",
		"EICHERMOT":   "Eicher Motors Ltd",
		"BAJAJ-AUTO":  "Bajaj Auto Ltd",
		"BAJAJFINSV":  "Bajaj Finserv Ltd",
		"TATACONSUM":  "Tata Consumer Products",
		"APOLLOHOSP":  "Apollo Hospitals Enterprise",
		"LTIM":        "LTIMindtree Ltd",
		"SBILIFE":     "SBI Life Insurance Co",
		"HINDALCO":    "Hindalco Industries Ltd",
		"HDFCLIFE":    "HDFC Life Insurance Co",
		"WIPRO":       "Wipro Ltd",
	}

	log.Printf("[PIPELINE] Starting NIFTY 50 ingestion (%d symbols)", len(nifty50))
	return p.IngestBatch(ctx, nifty50)
}
