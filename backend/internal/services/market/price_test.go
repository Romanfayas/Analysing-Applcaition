package market

import (
	"math"
	"testing"
)

func floatPtr(v float64) *float64 {
	return &v
}

func TestCalculatePriceChange_Positive(t *testing.T) {
	curr := floatPtr(1200.0)
	prev := floatPtr(1000.0)
	res := CalculatePriceChange(curr, prev)

	if !res.IsValid {
		t.Fatalf("expected valid result, got invalid")
	}
	if res.Change == nil || *res.Change != 200.0 {
		t.Errorf("expected change 200.0, got %v", res.Change)
	}
	if res.ChangePercent == nil || *res.ChangePercent != 20.0 {
		t.Errorf("expected change percent 20.0, got %v", res.ChangePercent)
	}
}

func TestCalculatePriceChange_Negative(t *testing.T) {
	curr := floatPtr(1158.90)
	prev := floatPtr(1170.30)
	res := CalculatePriceChange(curr, prev)

	if !res.IsValid {
		t.Fatalf("expected valid result, got invalid")
	}
	if res.Change == nil || *res.Change != -11.40 {
		t.Errorf("expected change -11.40, got %v", res.Change)
	}
	expectedPct := -0.9741
	if res.ChangePercent == nil || *res.ChangePercent != expectedPct {
		t.Errorf("expected change percent %f, got %v", expectedPct, res.ChangePercent)
	}
}

func TestCalculatePriceChange_Zero(t *testing.T) {
	curr := floatPtr(500.0)
	prev := floatPtr(500.0)
	res := CalculatePriceChange(curr, prev)

	if !res.IsValid {
		t.Fatalf("expected valid result, got invalid")
	}
	if res.Change == nil || *res.Change != 0.0 {
		t.Errorf("expected change 0.0, got %v", res.Change)
	}
	if res.ChangePercent == nil || *res.ChangePercent != 0.0 {
		t.Errorf("expected change percent 0.0, got %v", res.ChangePercent)
	}
}

func TestCalculatePriceChange_MissingPrevClose(t *testing.T) {
	curr := floatPtr(100.0)
	res := CalculatePriceChange(curr, nil)

	if res.IsValid {
		t.Errorf("expected invalid result when prevClose is nil")
	}
	if res.Change != nil || res.ChangePercent != nil {
		t.Errorf("expected nil change and changePercent when prevClose is nil")
	}
}

func TestCalculatePriceChange_MissingCurrentPrice(t *testing.T) {
	prev := floatPtr(100.0)
	res := CalculatePriceChange(nil, prev)

	if res.IsValid {
		t.Errorf("expected invalid result when currentPrice is nil")
	}
	if res.Change != nil || res.ChangePercent != nil {
		t.Errorf("expected nil change and changePercent when currentPrice is nil")
	}
}

func TestCalculatePriceChange_InvalidZeroPrice(t *testing.T) {
	curr := floatPtr(0.0)
	prev := floatPtr(100.0)
	res := CalculatePriceChange(curr, prev)

	if res.IsValid {
		t.Errorf("expected invalid result when currentPrice is 0.0")
	}

	curr2 := floatPtr(100.0)
	prev2 := floatPtr(0.0)
	res2 := CalculatePriceChange(curr2, prev2)
	if res2.IsValid {
		t.Errorf("expected invalid result when prevClose is 0.0")
	}
}

func TestCalculatePriceChange_InvalidNegativePrice(t *testing.T) {
	curr := floatPtr(-50.0)
	prev := floatPtr(100.0)
	res := CalculatePriceChange(curr, prev)

	if res.IsValid {
		t.Errorf("expected invalid result for negative price")
	}
}

func TestCalculatePriceChange_NaNAndInf(t *testing.T) {
	curr := floatPtr(math.NaN())
	prev := floatPtr(100.0)
	res := CalculatePriceChange(curr, prev)
	if res.IsValid {
		t.Errorf("expected invalid result for NaN")
	}

	currInf := floatPtr(math.Inf(1))
	resInf := CalculatePriceChange(currInf, prev)
	if resInf.IsValid {
		t.Errorf("expected invalid result for Inf")
	}
}
