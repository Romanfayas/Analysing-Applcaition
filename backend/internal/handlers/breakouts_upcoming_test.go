package handlers

import (
	"testing"

	"github.com/halal-equity/backend/internal/services/quant"
)

func TestClosestToBreakoutRanksWatchlistByDistance(t *testing.T) {
	sc := func(v float64) *float64 { return &v }
	scan := &BreakoutScan{BreakoutScanResponse: &quant.BreakoutScanResponse{Candidates: []quant.BreakoutResult{
		{Symbol: "FAR", Stage: "WATCHLIST", ResistanceLevel: 100, DistanceToResistancePct: 5.5, StructureScore: sc(70)},
		{Symbol: "NEAR", Stage: "WATCHLIST", ResistanceLevel: 100, DistanceToResistancePct: 0.8, StructureScore: sc(40)},
		{Symbol: "NEARBETTER", Stage: "WATCHLIST", ResistanceLevel: 100, DistanceToResistancePct: 0.8, StructureScore: sc(60)},
		{Symbol: "ABOVE", Stage: "WATCHLIST", ResistanceLevel: 100, DistanceToResistancePct: -3},
		{Symbol: "TOOFAR", Stage: "WATCHLIST", ResistanceLevel: 100, DistanceToResistancePct: 12},
		{Symbol: "CAND", Stage: "BREAKOUT_CANDIDATE", ResistanceLevel: 100, DistanceToResistancePct: 0.5},
	}}}
	got := closestToBreakout(scan, 10)
	want := []string{"NEARBETTER", "NEAR", "FAR"}
	if len(got) != len(want) {
		t.Fatalf("got %d items, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Symbol != w {
			t.Errorf("position %d: got %s want %s", i, got[i].Symbol, w)
		}
	}
	if len(closestToBreakout(scan, 2)) != 2 {
		t.Error("limit not applied")
	}
}
