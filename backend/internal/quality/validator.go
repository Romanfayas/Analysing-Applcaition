package quality

import (
	"fmt"
	"math"
	"time"
)

// Status represents the quality status of a data point.
type Status string

const (
	StatusValid     Status = "VALID"
	StatusMissing   Status = "MISSING"
	StatusStale     Status = "STALE"
	StatusDuplicate Status = "DUPLICATE"
	StatusSuspect   Status = "SUSPECT"
	StatusInvalid   Status = "INVALID"
	StatusCorrected Status = "CORRECTED"
)

// ValidationResult represents the outcome of a quality check.
type ValidationResult struct {
	Status  Status `json:"status"`
	Issues  []string `json:"issues,omitempty"`
}

// OHLCVCandle is a minimal OHLCV structure for validation.
type OHLCVCandle struct {
	Timestamp time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    int64
}

// Validator runs data quality checks on OHLCV candles.
type Validator struct {
	maxPriceChangePercent float64
	maxGapDays            int
	minVolume             int64
}

// NewValidator creates a quality validator with default thresholds.
func NewValidator() *Validator {
	return &Validator{
		maxPriceChangePercent: 20.0, // >20% daily move is suspicious for most stocks
		maxGapDays:            5,     // More than 5 trading days gap (accounting for holidays)
		minVolume:             0,     // Allow 0 volume (some days have no trading)
	}
}

// ValidateCandle checks a single candle for basic integrity.
func (v *Validator) ValidateCandle(candle OHLCVCandle) ValidationResult {
	var issues []string

	// 1. Non-positive prices
	if candle.Open <= 0 || candle.High <= 0 || candle.Low <= 0 || candle.Close <= 0 {
		issues = append(issues, "non-positive price detected")
		return ValidationResult{Status: StatusSuspect, Issues: issues}
	}

	// 2. High must be >= Open, Close, Low
	if candle.High < candle.Open || candle.High < candle.Close || candle.High < candle.Low {
		issues = append(issues, fmt.Sprintf("high (%.2f) is less than other prices", candle.High))
	}

	// 3. Low must be <= Open, Close, High
	if candle.Low > candle.Open || candle.Low > candle.Close || candle.Low > candle.High {
		issues = append(issues, fmt.Sprintf("low (%.2f) is greater than other prices", candle.Low))
	}

	// 4. Negative volume
	if candle.Volume < 0 {
		issues = append(issues, "negative volume")
	}

	if len(issues) > 0 {
		return ValidationResult{Status: StatusSuspect, Issues: issues}
	}

	return ValidationResult{Status: StatusValid}
}

// ValidateSeries checks a sequence of candles for series-level issues.
func (v *Validator) ValidateSeries(candles []OHLCVCandle) []ValidationResult {
	results := make([]ValidationResult, len(candles))
	var flatDaysCount int

	for i, candle := range candles {
		// Basic candle validation
		results[i] = v.ValidateCandle(candle)

		if i == 0 {
			continue
		}

		prev := candles[i-1]
		var issues []string

		// 1. Extreme price change (>20% price jumps/crashes)
		if prev.Close > 0 {
			change := (candle.Close - prev.Close) / prev.Close * 100
			if change > 20.0 || change < -20.0 {
				issues = append(issues, fmt.Sprintf("extreme daily price change: %.1f%%", change))
			}
		}

		// 2. Trading day gaps (accounting for weekends and holidays)
		gap := int(candle.Timestamp.Sub(prev.Timestamp).Hours() / 24)
		if gap > v.maxGapDays {
			issues = append(issues, fmt.Sprintf("trading gap of %d days", gap))
		}

		// 3. Duplicate timestamps
		if candle.Timestamp.Equal(prev.Timestamp) {
			results[i] = ValidationResult{Status: StatusDuplicate, Issues: []string{"duplicate timestamp"}}
			continue
		}

		// 4. Stale data (Flat close for multiple days)
		if candle.Close == prev.Close {
			flatDaysCount++
			if flatDaysCount >= 3 {
				issues = append(issues, fmt.Sprintf("stale data: Close price unchanged for %d days", flatDaysCount+1))
			}
		} else {
			flatDaysCount = 0
		}

		// 5. Flat price (open=high=low=close with 0 volume is suspicious)
		if candle.Open == candle.High && candle.High == candle.Low &&
			candle.Low == candle.Close && candle.Volume == 0 {
			issues = append(issues, "flat candle with zero volume (possible holiday or data error)")
		}

		if len(issues) > 0 {
			if results[i].Status == StatusValid {
				results[i] = ValidationResult{Status: StatusSuspect, Issues: issues}
			} else {
				results[i].Issues = append(results[i].Issues, issues...)
			}
		}
	}

	return results
}

// DetectMissingCandles finds gaps in the trading calendar.
// Returns expected dates that are missing from the data.
func (v *Validator) DetectMissingCandles(candles []OHLCVCandle, from, to time.Time) []time.Time {
	if len(candles) == 0 {
		return nil
	}

	existingDates := make(map[string]bool)
	for _, c := range candles {
		existingDates[c.Timestamp.Format("2006-01-02")] = true
	}

	var missing []time.Time
	current := from
	for current.Before(to) || current.Equal(to) {
		weekday := current.Weekday()
		// Skip weekends
		if weekday != time.Saturday && weekday != time.Sunday {
			dateStr := current.Format("2006-01-02")
			if !existingDates[dateStr] {
				missing = append(missing, current)
			}
		}
		current = current.AddDate(0, 0, 1)
	}

	return missing
}

// QualityScore calculates an overall quality score (0-100) for a data series.
func (v *Validator) QualityScore(results []ValidationResult) float64 {
	if len(results) == 0 {
		return 0
	}

	valid := 0
	for _, r := range results {
		if r.Status == StatusValid || r.Status == StatusCorrected {
			valid++
		}
	}

	return float64(valid) / float64(len(results)) * 100
}

// FundamentalStatement represents a simplified version for validation purposes.
type FundamentalStatement struct {
	Revenue           *float64
	PAT               *float64
	TotalAssets       *float64
	TotalLiabilities  *float64
	TotalEquity       *float64
	OperatingCashFlow *float64
}

// ValidateFundamentals checks financial constraints.
func (v *Validator) ValidateFundamentals(stmt FundamentalStatement) ValidationResult {
	var issues []string

	if stmt.TotalAssets != nil && *stmt.TotalAssets < 0 {
		issues = append(issues, "total assets cannot be negative")
	}
	if stmt.TotalLiabilities != nil && *stmt.TotalLiabilities < 0 {
		issues = append(issues, "total liabilities cannot be negative")
	}
	if stmt.Revenue != nil && *stmt.Revenue < 0 {
		issues = append(issues, "revenue cannot be negative")
	}

	// Basic accounting equation: Assets = Liabilities + Equity
	if stmt.TotalAssets != nil && stmt.TotalLiabilities != nil && stmt.TotalEquity != nil {
		expectedAssets := *stmt.TotalLiabilities + *stmt.TotalEquity
		// Allow small rounding tolerance for millions/billions truncation
		if math.Abs(*stmt.TotalAssets-expectedAssets) > 2.0 {
			issues = append(issues, fmt.Sprintf("Assets (%.2f) != Liabilities (%.2f) + Equity (%.2f)", *stmt.TotalAssets, *stmt.TotalLiabilities, *stmt.TotalEquity))
		}
	}

	if len(issues) > 0 {
		return ValidationResult{Status: StatusInvalid, Issues: issues}
	}

	return ValidationResult{Status: StatusValid}
}
