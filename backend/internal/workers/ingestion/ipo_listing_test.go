package ingestion

import (
	"reflect"
	"testing"

	"github.com/halal-equity/backend/internal/providers/marketdata/yahoofinance"
)

func TestListingTickerCandidates(t *testing.T) {
	q := func(sym, name string) yahoofinance.ListingQuote { return yahoofinance.ListingQuote{Symbol: sym, Name: name} }
	cases := []struct {
		name    string
		company string
		quotes  []yahoofinance.ListingQuote
		want    []string
	}{
		{"same name", "Johnsoncab Electricals Limited", []yahoofinance.ListingQuote{q("JOHNSONCAB.BO", "Johnsoncab Electricals Limited")}, []string{"JOHNSONCAB.BO"}},
		{"nameless hit, ticker is first word", "Antala Industries Limited", []yahoofinance.ListingQuote{q("ANTALA.BO", "")}, []string{"ANTALA.BO"}},
		{"nameless hit, unrelated ticker", "Antala Industries Limited", []yahoofinance.ListingQuote{q("ANTARIKSH.BO", "")}, nil},
		{"different company with a similar word", "Vectras Enprocon Limited", []yahoofinance.ListingQuote{q("VECTOR.BO", "Vector Industries Limited")}, nil},
		{"NSE and BSE both", "Vectras Enprocon Limited", []yahoofinance.ListingQuote{q("VECTRAS.BO", "Vectras Enprocon Limited"), q("VECTRAS.NS", "Vectras Enprocon Limited")}, []string{"VECTRAS.BO", "VECTRAS.NS"}},
	}
	for _, c := range cases {
		if got := listingTickerCandidates(c.company, c.quotes); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
