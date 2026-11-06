// Package evidence reconciles the platform's corporate-action records against exchange evidence.
// It only ever reports; it never modifies canonical events, factors or price bars.
package evidence

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Action classes derived from NSE corporate-action subjects.
const (
	ClassDividend = "DIVIDEND"
	ClassBonus    = "BONUS"
	ClassSplit    = "SPLIT"
	ClassDemerger = "DEMERGER"
	ClassBuyback  = "BUYBACK"
	ClassRights   = "RIGHTS"
	ClassOther    = "OTHER"
)

// FeedAction is one entry of NSE's corporates-corporateActions feed.
type FeedAction struct {
	Symbol  string `json:"symbol"`
	Series  string `json:"series"`
	ISIN    string `json:"isin"`
	ExDate  string `json:"exDate"`
	Subject string `json:"subject"`
}

// Classified is a feed action interpreted for reconciliation.
type Classified struct {
	FeedAction
	Ex             time.Time
	Class          string
	DividendPerSh  *float64
	PriceFactor    *float64 // multiply pre-ex-date prices by this to make them comparable post-ex-date
	PriceChanging  bool     // bonus/split/demerger alter the nominal price series
	ParseableExDte bool
}

var (
	reBonus    = regexp.MustCompile(`(?i)bonus\s+(\d+(?:\.\d+)?)\s*:\s*(\d+(?:\.\d+)?)`)
	reSplit    = regexp.MustCompile(`(?i)(?:split|sub-?division).*?from\s+(?:rs\.?|re\.?)\s*(\d+(?:\.\d+)?).*?to\s+(?:rs\.?|re\.?)\s*(\d+(?:\.\d+)?)`)
	reDividend = regexp.MustCompile(`(?i)(?:rs\.?|re\.?)\s*(\d+(?:\.\d+)?)`)
)

// Classify interprets one feed action. Factors are only produced where the subject states the ratio
// explicitly; anything else (e.g. a demerger's cost split) is left nil rather than guessed.
func Classify(a FeedAction) Classified {
	c := Classified{FeedAction: a, Class: ClassOther}
	if t, err := time.Parse("02-Jan-2006", strings.TrimSpace(a.ExDate)); err == nil {
		c.Ex, c.ParseableExDte = t, true
	}
	s := a.Subject
	ls := strings.ToLower(s)
	var factor float64 = 1
	have := false
	if m := reBonus.FindStringSubmatch(s); m != nil {
		bonus, _ := strconv.ParseFloat(m[1], 64)
		held, _ := strconv.ParseFloat(m[2], 64)
		if bonus > 0 && held > 0 {
			factor *= held / (bonus + held)
			have = true
		}
		c.Class = ClassBonus
	}
	if m := reSplit.FindStringSubmatch(s); m != nil {
		from, _ := strconv.ParseFloat(m[1], 64)
		to, _ := strconv.ParseFloat(m[2], 64)
		if from > 0 && to > 0 {
			factor *= to / from
			have = true
		}
		if c.Class == ClassOther {
			c.Class = ClassSplit
		}
	}
	switch {
	case c.Class != ClassOther:
	case strings.Contains(ls, "demerger"):
		c.Class = ClassDemerger
	case strings.Contains(ls, "buy back") || strings.Contains(ls, "buyback"):
		c.Class = ClassBuyback
	case strings.Contains(ls, "rights"):
		c.Class = ClassRights
	case strings.Contains(ls, "dividend"):
		c.Class = ClassDividend
		if m := reDividend.FindStringSubmatch(s); m != nil {
			if v, err := strconv.ParseFloat(m[1], 64); err == nil {
				c.DividendPerSh = &v
			}
		}
	}
	if have {
		c.PriceFactor = &factor
	}
	c.PriceChanging = c.Class == ClassBonus || c.Class == ClassSplit || c.Class == ClassDemerger
	return c
}

// DividendAmounts returns every per-share amount stated in a subject ("Interim Dividend - Rs 10 Per Share And
// Special Dividend - Rs 66 Per Share" -> [10, 66]).
func DividendAmounts(subject string) []float64 {
	var out []float64
	for _, m := range reDividend.FindAllStringSubmatch(subject, -1) {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			out = append(out, v)
		}
	}
	return out
}

// CanonicalClass maps the platform's action_type onto feed classes.
func CanonicalClass(actionType string) string {
	switch strings.ToUpper(actionType) {
	case "CASH_DIVIDEND", "DIVIDEND":
		return ClassDividend
	case "BONUS":
		return ClassBonus
	case "SPLIT", "STOCK_SPLIT":
		return ClassSplit
	case "DEMERGER":
		return ClassDemerger
	case "BUYBACK":
		return ClassBuyback
	case "RIGHTS":
		return ClassRights
	}
	return ClassOther
}

var reFragment = regexp.MustCompile(`^[A-Za-z0-9_()]{1,16}\)?$`)

// IsParserFragment reports whether a canonical "event" is a fragment of another event's text: an
// OTHER event whose subject is a short space-free token, sharing symbol and ex-date with a sibling.
func IsParserFragment(actionType, subject string, hasSibling bool) bool {
	return strings.EqualFold(actionType, "OTHER") && hasSibling && reFragment.MatchString(strings.TrimSpace(subject))
}
