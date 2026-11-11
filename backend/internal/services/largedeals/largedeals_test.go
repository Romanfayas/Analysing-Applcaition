package largedeals

import (
	"os"
	"testing"
)

func TestParseBulkAndBlockFiles(t *testing.T) {
	for _, c := range []struct{ file, kind string }{{"testdata/bulk.csv", "BULK"}, {"testdata/block.csv", "BLOCK"}} {
		f, err := os.Open(c.file)
		if err != nil {
			t.Fatal(err)
		}
		deals, skipped, err := ParseCSV(f, c.kind)
		f.Close()
		if err != nil || len(deals) == 0 || skipped != 0 {
			t.Fatalf("%s: deals=%d skipped=%d err=%v", c.file, len(deals), skipped, err)
		}
		d := deals[0]
		if d.Date.Year() != 2026 || d.Date.Month() != 11 || d.Symbol == "" || d.Client == "" || d.Qty <= 0 || d.Price <= 0 || d.Kind != c.kind {
			t.Errorf("%s: bad first row %+v", c.file, d)
		}
	}
}

func TestBlockedPageIsAnErrorNotAnEmptyDay(t *testing.T) {
	f, _ := os.Open("testdata/blocked.html")
	defer f.Close()
	if _, _, err := ParseCSV(f, "BULK"); err == nil {
		t.Fatal("an HTML block page must not parse as a deal file")
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"MOTILAL OSWAL MUTUAL FUND":                      TypeMutualFund,
		"KOTAK MAHINDRA MUTUAL FUND":                     TypeMutualFund,
		"SBI LIFE INSURANCE CO. LTD":                     TypeInsurance,
		"GOLDMAN SACHS INDIA LIMITED":                    TypeOther,
		"ABC EMERGING MARKETS FUND":                      TypeForeign,
		"XYZ GLOBAL INVESTORS PTE LTD":                   TypeForeign,
		"JASTI FAMILY TRUST":                             TypeOther,
		"VAIBHAV RAJENDRA DOSHI":                         TypeOther,
		"BULLPULSE MARKETEDGE PRIVATE LIMITED":           TypeOther,
		"HDFC BANK LIMITED":                              TypeBank,
		"NIPPON LIFE INDIA TRUSTEE LTD-A/C NIPPON INDIA": TypeMutualFund,
	}
	for name, want := range cases {
		if got := Classify(name); got != want {
			t.Errorf("%q: got %s want %s", name, got, want)
		}
	}
	if !IsInstitution(TypeMutualFund) || IsInstitution(TypeOther) || IsInstitution(TypeBank) {
		t.Error("IsInstitution wrong")
	}
}
