package plans

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

type parityCase struct {
	Plan        string `json:"plan"`
	EntryFilter string `json:"entry_filter"`
	Signal      struct {
		Stop, T1, T2, EntryHigh, ATR float64
	} `json:"-"`
	RawSignal map[string]float64 `json:"signal"`
	Bars      []struct {
		Date                   string
		Open, High, Low, Close float64
	} `json:"bars"`
	Expected *struct {
		Entry float64 `json:"entry"`
		Exit  float64 `json:"exit"`
		Why   string  `json:"why"`
		Bars  int     `json:"bars"`
	} `json:"expected"`
}

// TestParityWithThePythonSimulator feeds the same 4,800 generated situations (every plan x entry filter, random gaps, stops and targets) to
// the Go port and compares with what the learner's Python simulator produced: entry, exit price, reason and holding time must all match.
func TestParityWithThePythonSimulator(t *testing.T) {
	raw, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []parityCase
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) < 1000 {
		t.Fatalf("expected thousands of cases, got %d", len(cases))
	}
	stats := map[string]int{}
	for i, c := range cases {
		sig := Signal{Stop: c.RawSignal["stop"], T1: c.RawSignal["t1"], T2: c.RawSignal["t2"], EntryHigh: c.RawSignal["entry_high"], ATR: c.RawSignal["atr"]}
		bars := make([]Bar, len(c.Bars))
		for j, b := range c.Bars {
			bars[j] = Bar{b.Date, b.Open, b.High, b.Low, b.Close}
		}
		got := Simulate(sig, bars, Menu[c.Plan], EntryFilters[c.EntryFilter])
		if c.Expected == nil {
			if got.Entered {
				t.Fatalf("case %d (%s/%s): python skipped, go entered at %.2f", i, c.Plan, c.EntryFilter, got.EntryPx)
			}
			stats["skipped"]++
			continue
		}
		if !got.Entered || !got.Exited {
			t.Fatalf("case %d (%s/%s): python traded, go entered=%v exited=%v skip=%s", i, c.Plan, c.EntryFilter, got.Entered, got.Exited, got.SkipReason)
		}
		if math.Abs(got.EntryPx-c.Expected.Entry) > 1e-9 || math.Abs(got.ExitPx-c.Expected.Exit) > 1e-9 || got.Reason != c.Expected.Why || got.ExitIdx != c.Expected.Bars {
			t.Fatalf("case %d (%s/%s): python %+v, go entry %.4f exit %.4f %s after %d", i, c.Plan, c.EntryFilter, *c.Expected, got.EntryPx, got.ExitPx, got.Reason, got.ExitIdx)
		}
		stats[got.Reason]++
	}
	t.Logf("%d cases identical: %v", len(cases), stats)
}

func TestExtendingTheBarsNeverChangesAnEarlierResult(t *testing.T) {
	sig := Signal{Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 2}
	all := []Bar{{"d1", 100, 101, 99, 100}, {"d2", 100, 102, 99, 101}, {"d3", 101, 112, 100, 111}, {"d4", 111, 130, 90, 95}, {"d5", 95, 96, 80, 82}}
	first := Simulate(sig, all[:3], Menu["T1_10D"], nil)
	later := Simulate(sig, all, Menu["T1_10D"], nil)
	if !first.Exited || first.Reason != "TARGET" || first != later {
		t.Fatalf("a decided outcome must not change when later bars arrive: %+v vs %+v", first, later)
	}
}

func TestOpenPositionStaysOpenUntilTheLimitSessionExists(t *testing.T) {
	sig := Signal{Stop: 90, T1: 130, T2: 140, EntryHigh: 101, ATR: 2}
	var bars []Bar
	for i := 0; i < 10; i++ {
		bars = append(bars, Bar{"d", 100, 102, 99, 100})
	}
	o := Simulate(sig, bars, Menu["T1_10D"], nil) // bars[0] entry + 9 more: session 10 does not exist yet
	if !o.Entered || o.Exited {
		t.Fatalf("must still be open: %+v", o)
	}
	o = Simulate(sig, append(bars, Bar{"d", 100, 102, 99, 100.5}), Menu["T1_10D"], nil)
	if !o.Exited || o.Reason != "TIME_LIMIT" || o.ExitPx != 100.5 {
		t.Fatalf("time exit at the limit session's close expected: %+v", o)
	}
}

func TestGapThroughStopFillsAtTheOpenAndBothTouchedMeansStop(t *testing.T) {
	sig := Signal{Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 2}
	gap := Simulate(sig, []Bar{{"d1", 100, 101, 99, 100}, {"d2", 92, 93, 90, 91}}, Menu["T1_10D"], nil)
	if gap.Reason != "STOP_LOSS" || gap.ExitPx != 92 {
		t.Fatalf("gap through the stop must fill at the open: %+v", gap)
	}
	both := Simulate(sig, []Bar{{"d1", 100, 101, 99, 100}, {"d2", 100, 115, 94, 100}}, Menu["T1_10D"], nil)
	if both.Reason != "STOP_LOSS" || both.ExitPx != 95 {
		t.Fatalf("a bar touching both stop and target is a stop: %+v", both)
	}
}

func TestSkipsAreExplainedNotSilent(t *testing.T) {
	sig := Signal{Stop: 95, T1: 110, T2: 120, EntryHigh: 101, ATR: 0}
	cases := map[string]Outcome{
		"OPENED_AT_OR_BELOW_STOP":         Simulate(sig, []Bar{{"d", 94, 96, 93, 95}}, Menu["T1_10D"], nil),
		"OPENED_AT_OR_ABOVE_TARGET":       Simulate(sig, []Bar{{"d", 111, 112, 110, 111}}, Menu["T1_10D"], nil),
		"OPENED_TOO_FAR_ABOVE_ENTRY_ZONE": Simulate(sig, []Bar{{"d", 104, 105, 103, 104}}, Menu["T1_10D"], EntryFilters["WITHIN_1PCT"]),
		"NO_ATR_FOR_TRAILING_STOP":        Simulate(sig, []Bar{{"d", 100, 101, 99, 100}}, Menu["TRAIL_2ATR_20D"], nil),
		"NO_ENTRY_SESSION_YET":            Simulate(sig, nil, Menu["T1_10D"], nil),
	}
	for want, got := range cases {
		if got.Entered || got.SkipReason != want {
			t.Errorf("want skip %s, got %+v", want, got)
		}
	}
}
