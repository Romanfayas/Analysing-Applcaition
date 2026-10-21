package yahoofinance

import (
	"strings"
	"testing"
	"time"
)

func TestResolveProviderSymbol(t *testing.T) {
	// 1. Standard active symbols
	standardTests := []struct {
		symbol   string
		expected string
	}{
		{"RELIANCE", "RELIANCE.NS"},
		{"TCS", "TCS.NS"},
		{"INFY", "INFY.NS"},
		{"TMPV", "TMPV.NS"}, // Canonical primary successor for Tata Motors PV
		{"TMCV", "TMCV.NS"}, // Canonical successor for Tata Motors CV
		{"LTM", "LTM.NS"},   // Canonical current NSE ticker for LTIMindtree
	}

	for _, tc := range standardTests {
		res, err := ResolveProviderSymbol(tc.symbol)
		if err != nil {
			t.Errorf("ResolveProviderSymbol(%s) returned unexpected error: %v", tc.symbol, err)
		}
		if res != tc.expected {
			t.Errorf("ResolveProviderSymbol(%s) = %s; want %s", tc.symbol, res, tc.expected)
		}
	}

	// 2. Renamed 1:1 alias
	res, err := ResolveProviderSymbol("LTIM")
	if err != nil {
		t.Errorf("ResolveProviderSymbol(LTIM) returned unexpected error: %v", err)
	}
	if res != "LTM.NS" {
		t.Errorf("ResolveProviderSymbol(LTIM) = %s; want LTM.NS", res)
	}

	// 3. Predecessor demerged security MUST fail resolution for live market data
	// and NEVER silently map to an arbitrary successor
	res, err = ResolveProviderSymbol("TATAMOTORS")
	if err == nil {
		t.Fatalf("expected error resolving demerged predecessor TATAMOTORS, got %s", res)
	}
	if !strings.Contains(err.Error(), "SYMBOL_DEMERGED") {
		t.Errorf("expected error to contain SYMBOL_DEMERGED, got: %v", err)
	}
	if !strings.Contains(err.Error(), "TMPV") || !strings.Contains(err.Error(), "TMCV") {
		t.Errorf("expected error message to cite both successors TMPV and TMCV, got: %v", err)
	}
}

func TestCorporateActionRegistryMetadata(t *testing.T) {
	// Verify Tata Motors Demerger Metadata
	tm, exists := CorporateActionsRegistry["TATAMOTORS"]
	if !exists {
		t.Fatalf("TATAMOTORS missing from CorporateActionsRegistry")
	}
	if tm.ActionType != "DEMERGER" {
		t.Errorf("expected DEMERGER, got %s", tm.ActionType)
	}
	expectedDate := time.Date(2025, 10, 24, 0, 0, 0, 0, time.UTC)
	if !tm.EffectiveDate.Equal(expectedDate) {
		t.Errorf("expected effective date %v, got %v", expectedDate, tm.EffectiveDate)
	}
	if len(tm.Successors) != 2 {
		t.Fatalf("expected 2 successors for TATAMOTORS demerger, got %d", len(tm.Successors))
	}

	// Primary successor: TMPV (Passenger Vehicles)
	s1 := tm.Successors[0]
	if s1.Symbol != "TMPV" || s1.ProviderSym != "TMPV.NS" || !s1.IsPrimary {
		t.Errorf("successor 1 mismatch: %+v", s1)
	}
	if s1.CostSplitPct != 68.85 {
		t.Errorf("expected 68.85%% cost split for TMPV, got %.2f", s1.CostSplitPct)
	}

	// Secondary successor: TMCV (Commercial Vehicles)
	s2 := tm.Successors[1]
	if s2.Symbol != "TMCV" || s2.ProviderSym != "TMCV.NS" || s2.IsPrimary {
		t.Errorf("successor 2 mismatch: %+v", s2)
	}
	if s2.CostSplitPct != 31.15 {
		t.Errorf("expected 31.15%% cost split for TMCV, got %.2f", s2.CostSplitPct)
	}

	// Verify LTIM Rename Metadata
	ltim, exists := CorporateActionsRegistry["LTIM"]
	if !exists {
		t.Fatalf("LTIM missing from CorporateActionsRegistry")
	}
	if ltim.ActionType != "RENAME" {
		t.Errorf("expected RENAME, got %s", ltim.ActionType)
	}
	expectedRenameDate := time.Date(2026, 2, 27, 0, 0, 0, 0, time.UTC)
	if !ltim.EffectiveDate.Equal(expectedRenameDate) {
		t.Errorf("expected rename effective date %v, got %v", expectedRenameDate, ltim.EffectiveDate)
	}
	if len(ltim.Successors) != 1 || ltim.Successors[0].Symbol != "LTM" {
		t.Errorf("expected successor LTM for LTIM rename, got %+v", ltim.Successors)
	}
}
