package stocknews

import (
	"os"
	"strings"
	"testing"
	"time"
)

func read(t *testing.T, f string) []byte {
	b, err := os.ReadFile("testdata/" + f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGoogleNewsParsesRealFeedAndStripsPublisherFromTheTitle(t *testing.T) {
	n, err := ParseGoogleNews(read(t, "google.xml"))
	if err != nil || len(n) < 3 {
		t.Fatalf("n=%d err=%v", len(n), err)
	}
	for i, x := range n {
		if x.Title == "" || x.URL == "" || x.Publisher == "" || x.PublishedAt.IsZero() || x.Source != "GOOGLE_NEWS" {
			t.Fatalf("item %d incomplete: %+v", i, x)
		}
		if strings.HasSuffix(x.Title, " - "+x.Publisher) {
			t.Fatalf("publisher not stripped: %q", x.Title)
		}
		if i > 0 && x.PublishedAt.After(n[i-1].PublishedAt) {
			t.Fatal("must be newest first")
		}
	}
}

func TestAnnouncementsAreOfficialFilingsWithLinksAndIndianTime(t *testing.T) {
	f, err := ParseAnnouncements(read(t, "announcements.json"))
	if err != nil || len(f) == 0 {
		t.Fatalf("%v %d", err, len(f))
	}
	a := f[0]
	if a.Subject == "" || !strings.HasPrefix(a.AttachmentURL, "https://nsearchives.nseindia.com/") || a.Category == "" {
		t.Fatalf("%+v", a)
	}
	// 12:18:34 IST on 04-Nov-2026 is 06:48:34 UTC
	for _, x := range f {
		if x.AnnouncedAt.Format("2006-01-02 15:04:05") == "2026-11-04 06:48:34" {
			return
		}
	}
	t.Fatalf("expected the 04-Nov-2026 12:18:34 IST announcement as 06:48:34 UTC, got %v", f[0].AnnouncedAt)
}

func TestEventsFilterByWindowAndKeepTheirMeaning(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	ev, err := ParseEvents(read(t, "events.json"), now.AddDate(0, 0, -60), now.AddDate(0, 0, 120))
	if err != nil {
		t.Fatal(err)
	}
	if len(ev) == 0 {
		t.Fatal("a 2026-11-02 board meeting is inside the window")
	}
	for _, e := range ev {
		if e.Kind != "BOARD_MEETING" || e.Title == "" || e.Date.Before(now.AddDate(0, 0, -60)) || e.Date.After(now.AddDate(0, 0, 120)) {
			t.Fatalf("%+v", e)
		}
	}
	old, _ := ParseEvents(read(t, "events.json"), time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2011, 1, 1, 0, 0, 0, 0, time.UTC))
	if len(old) == 0 {
		t.Fatal("the 2010 entry must be returned when the window includes it")
	}
}

func TestCorporateActionsKeepDividendsWithRecordDates(t *testing.T) {
	now := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	ac, err := ParseActions(read(t, "actions.json"), now.AddDate(-1, 0, 0), now.AddDate(0, 6, 0))
	if err != nil || len(ac) == 0 {
		t.Fatalf("%v %d", err, len(ac))
	}
	found := false
	for _, a := range ac {
		if a.Kind != "CORPORATE_ACTION" || a.Title == "" {
			t.Fatalf("%+v", a)
		}
		if strings.Contains(a.Title, "Dividend") && strings.HasPrefix(a.Detail, "Record date") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a dividend with a record date: %+v", ac)
	}
}

func TestBadDocumentsAreErrorsNotEmptySuccess(t *testing.T) {
	if _, err := ParseGoogleNews([]byte("<html>blocked</html>")); err == nil {
		t.Error("an HTML block page is not RSS")
	}
	if _, err := ParseAnnouncements([]byte(`{"error":"x"}`)); err == nil {
		t.Error("an object is not a list")
	}
	if _, err := ParseEvents([]byte("nope"), time.Time{}, time.Now()); err == nil {
		t.Error("garbage must fail")
	}
}

func TestHashIsStableAndSeparatesFields(t *testing.T) {
	if Hash("a", "bc") == Hash("ab", "c") || Hash("a", "b") != Hash("a", "b") || len(Hash("x")) != 64 {
		t.Fatal("hash must be deterministic and field-aware")
	}
	if !strings.Contains(GoogleNewsURL("Tata Motors Ltd"), "Tata+Motors+Ltd+share") {
		t.Fatal(GoogleNewsURL("Tata Motors Ltd"))
	}
}
