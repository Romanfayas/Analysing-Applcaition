package market

import (
	"math"
)

// PriceChangeResult represents the calculation of daily price change.
type PriceChangeResult struct {
	CurrentPrice  *float64 `json:"current_price"`
	PreviousClose *float64 `json:"previous_close"`
	Change        *float64 `json:"change"`
	ChangePercent *float64 `json:"change_percent"`
	IsValid       bool     `json:"is_valid"`
}

// CalculatePriceChange calculates the price change and percentage change.
// Returns an invalid result with nil change fields if either price is missing,
// non-positive, NaN, or infinite.
func CalculatePriceChange(currentPrice *float64, prevClose *float64) PriceChangeResult {
	if currentPrice == nil || prevClose == nil {
		return PriceChangeResult{
			CurrentPrice:  currentPrice,
			PreviousClose: prevClose,
			Change:        nil,
			ChangePercent: nil,
			IsValid:       false,
		}
	}

	curr := *currentPrice
	prev := *prevClose

	if curr <= 0 || prev <= 0 || math.IsNaN(curr) || math.IsNaN(prev) || math.IsInf(curr, 0) || math.IsInf(prev, 0) {
		return PriceChangeResult{
			CurrentPrice:  currentPrice,
			PreviousClose: prevClose,
			Change:        nil,
			ChangePercent: nil,
			IsValid:       false,
		}
	}

	change := curr - prev
	changePct := (change / prev) * 100.0

	// Round to 4 decimal places for precision
	change = math.Round(change*10000) / 10000
	changePct = math.Round(changePct*10000) / 10000

	return PriceChangeResult{
		CurrentPrice:  &curr,
		PreviousClose: &prev,
		Change:        &change,
		ChangePercent: &changePct,
		IsValid:       true,
	}
}
