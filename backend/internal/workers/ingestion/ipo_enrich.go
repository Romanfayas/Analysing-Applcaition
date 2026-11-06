package ingestion

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/halal-equity/backend/internal/database"
	"github.com/halal-equity/backend/internal/providers/ipowatch"
	"github.com/halal-equity/backend/internal/providers/marketdata/nsedata"
)

// IPORecordDetail is the merged record shown behind "View details". Official and unofficial facts stay separate.
type IPORecordDetail struct {
	Official         *nsedata.IPODetail     `json:"official"`
	Unofficial       *ipowatch.Detail       `json:"unofficial"`
	Categories       []nsedata.CategoryBook `json:"categories"`
	FreshIssueCr     *float64               `json:"fresh_issue_cr"`
	OFSCr            *float64               `json:"ofs_cr"`
	IssueSizeCr      *float64               `json:"issue_size_cr"`
	LotSize          *int                   `json:"lot_size"`
	MinInvestment    *float64               `json:"min_investment"`
	ListingAt        string                 `json:"listing_at,omitempty"`
	ScheduledListing *time.Time             `json:"scheduled_listing_date,omitempty"`
	GMPSource        string                 `json:"gmp_source,omitempty"`
	GMPSourceURL     string                 `json:"gmp_source_url,omitempty"`
}

// minCategoryShare is the smallest share of an issue a category needs before a multiple derived from it means anything.
const minCategoryShare = 0.005

func cr(shares, price *float64) *float64 {
	if shares == nil || price == nil {
		return nil
	}
	v := *shares * *price / 1e7
	return &v
}

func firstOf(vals ...*float64) *float64 {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

// MergeIPODetail combines NSE's record with the secondary one. NSE wins wherever it speaks. For SME issues NSE publishes
// shares bid but not shares offered, so those category multiples are derived from the secondary page and marked DERIVED;
// the Rs 10 lakh split of the non-institutional portion is 2:1, as the SEBI rule sets it.
func MergeIPODetail(o *nsedata.IPODetail, u *ipowatch.Detail, band *float64) IPORecordDetail {
	m := IPORecordDetail{Official: o, Unofficial: u}
	var uFresh, uOFS, uSize *float64
	if u != nil {
		uFresh, uOFS, uSize = u.FreshCr, u.OFSCr, u.IssueSizeCr
		m.ListingAt, m.ScheduledListing = u.Listing, u.ListingDate
	}
	var fresh, ofs *float64
	if o != nil {
		fresh = firstOf(o.Structure.FreshAmountCr, cr(o.Structure.FreshShares, band))
		ofs = firstOf(o.Structure.OFSAmountCr, cr(o.Structure.OFSShares, band))
		m.LotSize = o.LotSize
		m.Categories = append(m.Categories, o.Categories...)
	}
	m.FreshIssueCr, m.OFSCr = firstOf(fresh, uFresh), firstOf(ofs, uOFS)
	if fresh != nil || ofs != nil {
		t := 0.0
		for _, v := range []*float64{fresh, ofs} {
			if v != nil {
				t += *v
			}
		}
		m.IssueSizeCr = &t
	} else {
		m.IssueSizeCr = uSize
	}
	if m.LotSize != nil && band != nil {
		v := float64(*m.LotSize) * *band
		m.MinInvestment = &v
	}
	if u != nil {
		offered := map[string]*float64{}
		for _, r := range u.Reservations {
			c := strings.ToUpper(r.Category)
			switch {
			case strings.HasPrefix(c, "QIB"):
				offered[nsedata.CatQIB] = r.Shares
			case strings.HasPrefix(c, "NII"):
				offered[nsedata.CatNII] = r.Shares
			case strings.HasPrefix(c, "RETAIL"):
				offered[nsedata.CatRetail] = r.Shares
			}
		}
		if n := offered[nsedata.CatNII]; n != nil {
			big, small := *n*2/3, *n/3
			offered[nsedata.CatBHNI], offered[nsedata.CatSHNI] = &big, &small
		}
		var sumOff float64
		for _, k := range []string{nsedata.CatQIB, nsedata.CatNII, nsedata.CatRetail} {
			if v := offered[k]; v != nil {
				sumOff += *v
			}
		}
		for i := range m.Categories {
			c := &m.Categories[i]
			if c.Offered != nil {
				continue
			}
			off := offered[c.Category]
			if c.Category == nsedata.CatTotal && sumOff > 0 {
				off = &sumOff
			}
			// A category barely present in the source (one issue lists "209 shares" for QIBs) gives a meaningless multiple,
			// so the shares are shown but no multiple is claimed.
			if off != nil && *off > 0 {
				c.Offered = off
				if c.Bid != nil && *off >= sumOff*minCategoryShare {
					t := *c.Bid / *off
					c.Times, c.Basis = &t, "DERIVED"
				}
			}
		}
	}
	return m
}

func (w *IPOWorker) enrich(ctx context.Context) {
	if n, err := w.db.SweepIPOLifecycle(ctx, time.Now().In(istLoc)); err != nil {
		log.Printf("[IPO-WORKER] lifecycle sweep failed: %v", err)
	} else if n > 0 {
		log.Printf("[IPO-WORKER] lifecycle sweep moved %d issues on", n)
	}
	list, err := w.watch.FetchGMP()
	if err != nil {
		log.Printf("[IPO-WORKER] GMP list unavailable: %v", err)
	}
	time.Sleep(ipowatch.RequestGap)
	ipos, err := w.db.IPOsToRefresh(ctx, 10*24*time.Hour)
	if err != nil {
		log.Printf("[IPO-WORKER] cannot list issues to refresh: %v", err)
		return
	}
	matched := make(map[int]bool, len(list))
	for _, ipo := range ipos {
		var row *ipowatch.GMPRow
		for i := range list {
			if ipowatch.SameCompany(list[i].Name, ipo.CompanyName) {
				row, matched[i] = &list[i], true
				break
			}
		}
		w.enrichOne(ctx, ipo, row)
	}
	for i, r := range list {
		if !matched[i] {
			w.addFromList(ctx, r)
		}
	}
}

func (w *IPOWorker) enrichOne(ctx context.Context, ipo database.ActiveIPO, row *ipowatch.GMPRow) {
	band := ipo.PriceBandHigh
	var u *ipowatch.Detail
	if row != nil {
		if row.GMP != nil {
			var pct *float64
			if row.Price != nil && *row.Price > 0 {
				v := *row.GMP / *row.Price * 100
				pct = &v
			}
			if err := w.db.RecordIPOGMP(ctx, ipo.ID, *row.GMP, pct, ipowatch.SourceName, row.GMPPageURL); err != nil {
				log.Printf("[IPO-WORKER] %s: GMP not stored: %v", ipo.CompanyName, err)
			}
		}
		if band == nil {
			band = row.Price
		}
		if row.DetailURL != "" {
			d, err := w.watch.FetchDetail(row.DetailURL)
			if err != nil {
				log.Printf("[IPO-WORKER] %s: secondary detail unavailable: %v", ipo.CompanyName, err)
			} else {
				u = d
			}
			time.Sleep(ipowatch.RequestGap)
		}
	}
	var o *nsedata.IPODetail
	if ipo.Symbol != "" {
		series := "EQ"
		if ipo.Board == "SME" {
			series = "SME"
		}
		d, err := w.nse.GetIPOFullDetail(ipo.Symbol, series)
		if err != nil {
			log.Printf("[IPO-WORKER] %s: NSE detail unavailable: %v", ipo.Symbol, err)
		} else {
			o = d
		}
		time.Sleep(600 * time.Millisecond)
	}
	if o == nil && u == nil {
		return
	}
	merged := MergeIPODetail(o, u, band)
	if row != nil {
		merged.GMPSource, merged.GMPSourceURL = ipowatch.SourceName, row.GMPPageURL
	}
	if err := w.db.UpsertIPODetails(ctx, ipo.ID, merged); err != nil {
		log.Printf("[IPO-WORKER] %s: details not stored: %v", ipo.CompanyName, err)
		return
	}
	facts := database.IPOFacts{IssueSizeCr: merged.IssueSizeCr, FreshCr: merged.FreshIssueCr, OFSCr: merged.OFSCr, MinInvestment: merged.MinInvestment, LotSize: merged.LotSize}
	if u != nil {
		facts.BandLow, facts.BandHigh = ipowatch.ParseBand(u.PriceBand)
	}
	if o != nil && o.Industry != "" {
		facts.Sector = &o.Industry
	}
	if err := w.db.ApplyIPOFacts(ctx, ipo.ID, facts); err != nil {
		log.Printf("[IPO-WORKER] %s: facts not stored: %v", ipo.CompanyName, err)
	}
	if o != nil || len(merged.Categories) > 0 {
		snap := database.IPOSubscriptionSnapshot{Source: "NSE_IPO_DETAIL"}
		for _, c := range merged.Categories {
			switch c.Category {
			case nsedata.CatQIB:
				snap.QIB = c.Times
			case nsedata.CatNII:
				snap.NII = c.Times
			case nsedata.CatRetail:
				snap.Retail = c.Times
			case nsedata.CatTotal:
				snap.Total = c.Times
			}
		}
		if ipo.Status != "LISTED" {
			if _, err := w.db.RecordIPOSubscription(ctx, ipo.ID, nil, time.Now(), snap); err != nil {
				log.Printf("[IPO-WORKER] %s: subscription not stored: %v", ipo.CompanyName, err)
			}
		}
	}
	w.detectListing(ctx, ipo, merged)
}

// detectListing records the first day's prices of a listed issue. NSE's past-issues feed can lag by a day, so an issue past
// its scheduled listing date also counts as listed once the exchange shows real trades in it. A date alone is never enough.
func (w *IPOWorker) detectListing(ctx context.Context, ipo database.ActiveIPO, m IPORecordDetail) {
	// Until the listing day has ended its close is still moving, so keep refreshing it.
	today := time.Now().In(istLoc).Format("2006-01-02")
	listedEarlier := ipo.ListingDate != nil && ipo.ListingDate.Format("2006-01-02") < today
	if ipo.Symbol == "" || (ipo.ListingPrice != nil && listedEarlier) {
		return
	}
	when := ipo.ListingDate
	if when == nil && ipo.Status == "CLOSED_AWAITING_LISTING" {
		when = m.ScheduledListing
	}
	if when == nil || when.After(time.Now()) {
		return
	}
	ticker := ipo.Symbol
	if ipo.Board == "SME" {
		ticker += "-SM" // NSE SME series on the price feed
	}
	bars, err := w.yahoo.GetHistoricalData(ticker, when.AddDate(0, 0, -1), time.Now().AddDate(0, 0, 1), "1d")
	if err != nil || len(bars) == 0 || bars[0].Open <= 0 {
		return
	}
	first := bars[0]
	if err := w.db.SetIPOListed(ctx, ipo.ID, first.Timestamp, first.Open, first.Close); err != nil {
		log.Printf("[IPO-WORKER] %s: listing not stored: %v", ipo.Symbol, err)
		return
	}
	log.Printf("[IPO-WORKER] %s listed on %s at %.2f", ipo.Symbol, first.Timestamp.Format("2006-01-02"), first.Open)
}

// addFromList stores an issue the NSE feeds do not carry (for example one listing only on BSE SME), so it is not missing.
func (w *IPOWorker) addFromList(ctx context.Context, r ipowatch.GMPRow) {
	if r.DetailURL == "" {
		return
	}
	d, err := w.watch.FetchDetail(r.DetailURL)
	time.Sleep(ipowatch.RequestGap)
	if err != nil || d.Timeline == nil {
		return
	}
	open, err1 := time.Parse("January 2, 2006", d.Timeline["IPO Open Date"])
	closeD, err2 := time.Parse("January 2, 2006", d.Timeline["IPO Close Date"])
	if err1 != nil || err2 != nil {
		return
	}
	board, exchange, source := "MAINBOARD", "NSE", "IPOWATCH_UNOFFICIAL"
	listing := strings.ToUpper(d.Listing)
	if strings.Contains(listing, "SME") {
		board = "SME"
	}
	if strings.Contains(listing, "BSE") && !strings.Contains(listing, "NSE") {
		exchange = "BSE"
	}
	status := "UPCOMING"
	switch r.Status {
	case "Open":
		status = "OPEN"
	case "Closed":
		status = "CLOSED_AWAITING_LISTING"
	case "Listed":
		return // old news; the list keeps listed issues for a while
	}
	name := r.Name
	if !strings.HasSuffix(strings.ToLower(name), "limited") && !strings.HasSuffix(strings.ToLower(name), "ltd") {
		name += " Limited"
	}
	rec := database.IPORecord{CompanyName: name, Exchange: exchange, Board: &board, Source: &source, Status: status, OpenDate: &open, CloseDate: &closeD, PriceBandHigh: r.Price, IssueSize: d.IssueSizeCr}
	id, isNew, err := w.db.UpsertIPOReportingInsert(ctx, rec)
	if err != nil {
		log.Printf("[IPO-WORKER] %s: not stored: %v", name, err)
		return
	}
	if isNew {
		log.Printf("[IPO-WORKER] added %s from the secondary list (%s)", name, fmt.Sprint(d.Listing))
	}
	w.enrichOne(ctx, database.ActiveIPO{ID: id, CompanyName: name, Board: board, Status: status, PriceBandHigh: r.Price}, &r)
}
