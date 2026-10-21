package handlers

import (
	"testing"
	"time"

	"github.com/halal-equity/backend/internal/database"
)

func okObs() database.PaperObservation {
	return database.PaperObservation{Symbol: "INFY", SetupType: "CONFIRMED_BREAKOUT", EntryTriggerPrice: 1000, InvalidationPrice: 950,
		TargetPrice: 1100, ShariahStatus: "PASS"}
}

func TestValidateCandidate(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		mut    func(o *database.PaperObservation)
		last   float64
		wantOK bool
	}{
		{"valid", func(o *database.PaperObservation) {}, 995, true},
		{"entry 76% above last close (the invalidated P1.6 INFY row)", func(o *database.PaperObservation) { o.EntryTriggerPrice, o.InvalidationPrice, o.TargetPrice = 1920.5, 1860, 2041.5 }, 1090.9, false},
		{"level ordering", func(o *database.PaperObservation) { o.InvalidationPrice = 1000 }, 995, false},
		{"shariah not PASS", func(o *database.PaperObservation) { o.ShariahStatus = "FAIL" }, 995, false},
		{"foreign rule version", func(o *database.PaperObservation) { o.RuleVersion = "abc" }, 995, false},
		{"pinned rule version", func(o *database.PaperObservation) { o.RuleVersion = pinnedStrategyHash }, 995, true},
		{"no history", func(o *database.PaperObservation) {}, 0, false},
		{"future cutoff", func(o *database.PaperObservation) { o.DataCutoffAt = now.Add(time.Hour) }, 995, false},
	}
	for _, c := range cases {
		o := okObs()
		c.mut(&o)
		err := ValidateCandidate(&o, c.last, now)
		if (err == nil) != c.wantOK {
			t.Errorf("%s: err=%v wantOK=%v", c.name, err, c.wantOK)
		}
	}
}
