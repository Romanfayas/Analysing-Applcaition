package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/pkg/response"
)

var istZone = time.FixedZone("IST", 5*3600+1800)

const (
	sessionStartMin = 9*60 + 15  // 09:15
	sessionEndMin   = 15*60 + 30 // 15:30, exclusive
	baselineDays    = 20
)

// ChartCandle is one candle of the stock chart. Intraday candles carry a Unix time, daily candles a date.
type ChartCandle struct {
	Date string  `json:"date,omitempty"`
	TS   int64   `json:"ts,omitempty"`
	O    float64 `json:"o"`
	H    float64 `json:"h"`
	L    float64 `json:"l"`
	C    float64 `json:"c"`
	V    int64   `json:"v"`
}

// LargeMinute is a one-minute candle whose volume is far above what the stock normally trades at that time of day.
type LargeMinute struct {
	TS       int64   `json:"ts"`
	Time     string  `json:"time"`
	Volume   int64   `json:"volume"`
	Notional float64 `json:"notional_inr"`
	Times    float64 `json:"times_normal"`
	Side     string  `json:"candle"` // UP | DOWN | FLAT: the colour of the candle, which is all a price bar can say about direction
}

func minuteOfDay(t time.Time) int {
	l := t.In(istZone)
	return l.Hour()*60 + l.Minute()
}

func inSession(t time.Time) bool {
	m := minuteOfDay(t)
	return m >= sessionStartMin && m < sessionEndMin
}

func istDate(t time.Time) string { return t.In(istZone).Format("2006-01-02") }

// aggregateIntraday merges the session bars into candles of `minutes` minutes.
func aggregateIntraday(bars []database.IntradayBar, minutes int) []ChartCandle {
	var out []ChartCandle
	curKey := ""
	for _, b := range bars {
		if !inSession(b.Timestamp) {
			continue
		}
		slot := (minuteOfDay(b.Timestamp) - sessionStartMin) / minutes
		key := istDate(b.Timestamp) + "#" + strconv.Itoa(slot)
		if key != curKey {
			l := b.Timestamp.In(istZone)
			start := time.Date(l.Year(), l.Month(), l.Day(), 0, sessionStartMin+slot*minutes, 0, 0, istZone)
			out = append(out, ChartCandle{TS: start.Unix(), O: b.Open, H: b.High, L: b.Low, C: b.Close, V: b.Volume})
			curKey = key
			continue
		}
		c := &out[len(out)-1]
		if b.High > c.H {
			c.H = b.High
		}
		if b.Low < c.L {
			c.L = b.Low
		}
		c.C = b.Close
		c.V += b.Volume
	}
	return out
}

func median(v []int64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]int64(nil), v...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	if len(s)%2 == 1 {
		return float64(s[len(s)/2])
	}
	return float64(s[len(s)/2-1]+s[len(s)/2]) / 2
}

// ChartSession is the one-day summary shown above the 1D chart.
type ChartSession struct {
	Date             string  `json:"date"`
	TotalVolume      int64   `json:"total_volume"`
	TotalNotionalINR float64 `json:"total_notional_inr"`
	UpVolumePct      float64 `json:"up_candle_volume_pct"`
	LargeCount       int     `json:"large_minutes"`
	LargeUpINR       float64 `json:"large_up_notional_inr"`
	LargeDownINR     float64 `json:"large_down_notional_inr"`
	LargeSharePct    float64 `json:"large_share_of_day_pct"`
	BaselineSessions int     `json:"baseline_sessions"`
}

// flagLargeMinutes compares each minute of the latest session with the median volume of the same half hour over the previous sessions.
// A minute is flagged when it trades at least `mult` times that and at least `minNotional` rupees. Price bars carry total volume only: this shows
// that something large traded, never who traded or whether it was a purchase.
func flagLargeMinutes(bars []database.IntradayBar, mult, minNotional float64) (*ChartSession, []LargeMinute) {
	byDay := map[string][]database.IntradayBar{}
	var days []string
	for _, b := range bars {
		if !inSession(b.Timestamp) {
			continue
		}
		d := istDate(b.Timestamp)
		if _, ok := byDay[d]; !ok {
			days = append(days, d)
		}
		byDay[d] = append(byDay[d], b)
	}
	if len(days) == 0 {
		return nil, nil
	}
	sort.Strings(days)
	today := days[len(days)-1]
	hist := days[:len(days)-1]
	if len(hist) > baselineDays {
		hist = hist[len(hist)-baselineDays:]
	}
	slotVols := make([][]int64, 13)
	for _, d := range hist {
		for _, b := range byDay[d] {
			s := (minuteOfDay(b.Timestamp) - sessionStartMin) / 30
			if s >= 0 && s < len(slotVols) {
				slotVols[s] = append(slotVols[s], b.Volume)
			}
		}
	}
	slotMed := make([]float64, len(slotVols))
	for i, v := range slotVols {
		slotMed[i] = median(v)
	}
	sum := &ChartSession{Date: today, BaselineSessions: len(hist)}
	var up, down int64
	var largeN float64
	var out []LargeMinute
	for _, b := range byDay[today] {
		notional := float64(b.Volume) * b.Close
		sum.TotalVolume += b.Volume
		sum.TotalNotionalINR += notional
		side := "FLAT"
		switch {
		case b.Close > b.Open:
			side, up = "UP", up+b.Volume
		case b.Close < b.Open:
			side, down = "DOWN", down+b.Volume
		}
		if len(hist) < 3 {
			continue
		}
		med := slotMed[(minuteOfDay(b.Timestamp)-sessionStartMin)/30]
		if med < 1 {
			med = 1
		}
		if float64(b.Volume) >= mult*med && notional >= minNotional {
			out = append(out, LargeMinute{TS: b.Timestamp.Unix(), Time: b.Timestamp.In(istZone).Format("15:04"), Volume: b.Volume, Notional: notional, Times: float64(b.Volume) / med, Side: side})
			sum.LargeCount++
			largeN += notional
			switch side {
			case "UP":
				sum.LargeUpINR += notional
			case "DOWN":
				sum.LargeDownINR += notional
			}
		}
	}
	if sum.TotalNotionalINR > 0 {
		sum.LargeSharePct = largeN / sum.TotalNotionalINR * 100
	}
	if up+down > 0 {
		sum.UpVolumePct = float64(up) / float64(up+down) * 100
	}
	return sum, out
}

// GetChart serves the stock chart for a range: 1D (every 1-minute candle of the latest session), 1W (15-minute), 1M, 6M, 1Y and ALL (daily).
func (h *StocksHandler) GetChart(w http.ResponseWriter, r *http.Request) {
	sym, ok := h.lookupSymbol(w, r)
	if !ok {
		return
	}
	rng := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("range")))
	if rng == "" {
		rng = "1M"
	}
	ctx := r.Context()
	out := map[string]any{"symbol": sym.Symbol, "range": rng}
	switch rng {
	case "1D", "1W":
		sessions, minutes, label := 1, 1, "1 minute"
		if rng == "1W" {
			sessions, minutes, label = 5, 15, "15 minutes"
		}
		need := sessions
		if rng == "1D" {
			need = baselineDays + 1
		}
		bars, err := h.db.IntradayLastSessions(ctx, sym.ID, need)
		if err != nil {
			log.Error().Err(err).Msg("chart: intraday failed")
			response.Error(w, http.StatusInternalServerError, "failed to load the chart")
			return
		}
		show := bars
		if rng == "1D" {
			var lastDay string
			for _, b := range bars {
				if inSession(b.Timestamp) {
					lastDay = istDate(b.Timestamp)
				}
			}
			show = show[:0:0]
			for _, b := range bars {
				if istDate(b.Timestamp) == lastDay {
					show = append(show, b)
				}
			}
		} else if len(bars) > 0 {
			// keep only the newest `sessions` days
			seen, cut := map[string]bool{}, ""
			for i := len(bars) - 1; i >= 0; i-- {
				d := istDate(bars[i].Timestamp)
				if !seen[d] {
					seen[d] = true
					if len(seen) > sessions {
						break
					}
				}
				cut = d
			}
			show = show[:0:0]
			for _, b := range bars {
				if istDate(b.Timestamp) >= cut {
					show = append(show, b)
				}
			}
		}
		out["interval"], out["candles"] = label, aggregateIntraday(show, minutes)
		if rng == "1D" {
			mult, minN := 5.0, 2_000_000.0
			if v, err := strconv.ParseFloat(r.URL.Query().Get("mult"), 64); err == nil && v >= 2 && v <= 20 {
				mult = v
			}
			if v, err := strconv.ParseFloat(r.URL.Query().Get("min_notional"), 64); err == nil && v >= 100_000 && v <= 1_000_000_000 {
				minN = v
			}
			session, large := flagLargeMinutes(bars, mult, minN)
			out["session"], out["large_minutes"] = session, large
			out["params"] = map[string]float64{"times_normal": mult, "min_notional_inr": minN}
		}
	case "1M", "6M", "1Y", "ALL":
		limit := map[string]int{"1M": 22, "6M": 126, "1Y": 252, "ALL": 1000}[rng]
		bars, err := h.db.GetDailyBars(ctx, sym.ID, limit, false)
		if err != nil {
			log.Error().Err(err).Msg("chart: daily failed")
			response.Error(w, http.StatusInternalServerError, "failed to load the chart")
			return
		}
		candles := make([]ChartCandle, 0, len(bars))
		for _, b := range bars {
			candles = append(candles, ChartCandle{Date: istDate(b.Timestamp), O: b.Open, H: b.High, L: b.Low, C: b.Close, V: b.Volume})
		}
		out["interval"], out["candles"] = "1 day", candles
	default:
		response.ValidationError(w, "range must be one of 1D, 1W, 1M, 6M, 1Y, ALL")
		return
	}
	response.JSON(w, http.StatusOK, out)
}
