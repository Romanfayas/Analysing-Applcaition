package evidence

import (
	"net/url"
	"sort"
	"strings"
	"time"
)

// Announcement is one entry of NSE's corporate-announcements feed: a company filing hosted by the exchange.
type Announcement struct {
	Symbol string `json:"symbol"`
	Desc   string `json:"desc"`
	Date   string `json:"an_dt"`
	Text   string `json:"attchmntText"`
	File   string `json:"attchmntFile"`
}

// AnnouncementsURL lists a company's filings around an action's ex-date.
func AnnouncementsURL(symbol string, ex time.Time) string {
	q := url.Values{}
	q.Set("index", "equities")
	q.Set("symbol", symbol)
	q.Set("from_date", ex.AddDate(0, 0, -120).Format("02-01-2006"))
	q.Set("to_date", ex.AddDate(0, 0, 3).Format("02-01-2006"))
	return "https://www.nseindia.com/api/corporate-announcements?" + q.Encode()
}

var classKeywords = map[string][]string{
	ClassBonus:    {"bonus"},
	ClassSplit:    {"sub-division", "subdivision", "sub division", "split"},
	ClassDemerger: {"demerger", "scheme of arrangement"},
}

// SelectFilings picks the filings most likely to state an action's terms: record-date intimations first, then
// board-meeting outcomes, newest first, at most `limit`. Only PDFs on the exchange's archive are considered.
func SelectFilings(anns []Announcement, class string, limit int) []Announcement {
	kws := classKeywords[class]
	type scored struct {
		a     Announcement
		score int
		t     time.Time
	}
	var c []scored
	for _, a := range anns {
		text := strings.ToLower(a.Desc + " " + a.Text)
		if !strings.HasPrefix(a.File, "https://nsearchives.nseindia.com/") || !strings.HasSuffix(strings.ToLower(a.File), ".pdf") {
			continue
		}
		hit := false
		for _, k := range kws {
			hit = hit || strings.Contains(text, k)
		}
		if !hit {
			continue
		}
		s := 0
		if strings.Contains(text, "record date") {
			s += 2
		}
		if strings.Contains(text, "outcome") || strings.Contains(text, "board meeting") || strings.Contains(text, "regulation 30") ||
			strings.Contains(text, "board of directors") || strings.Contains(text, "meeting held") {
			s++
		}
		t, _ := time.Parse("02-Jan-2006 15:04:05", a.Date)
		c = append(c, scored{a, s, t})
	}
	sort.SliceStable(c, func(i, j int) bool {
		if c[i].score != c[j].score {
			return c[i].score > c[j].score
		}
		return c[i].t.After(c[j].t)
	})
	var out []Announcement
	for i := 0; i < len(c) && i < limit; i++ {
		out = append(out, c[i].a)
	}
	return out
}
