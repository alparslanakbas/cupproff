package market

import (
	"slices"
	"testing"
)

// A pending market has no address and no language on the site until it
// opens; Preview gives it both, after the live markets and without changing
// them.
func TestPendingMarketsStayOffUntilPreview(t *testing.T) {
	for _, code := range []string{"en-tr", "tr-tr"} {
		if _, ok := ByCode(code); ok {
			t.Errorf("%s is on the site before its market opened", code)
		}
	}
	if slices.Contains(Languages(), "tr") {
		t.Error("Turkish is on the site before a market speaks it")
	}

	live := All
	t.Cleanup(func() { All = live })
	Preview()

	m, ok := ByCode("tr-tr")
	if !ok || m.Key != "en-tr" || m.Currency != "TRY" || m.Language() != "tr" || m.Name != "Türkiye" {
		t.Fatalf("tr-tr in preview = %+v, %v", m, ok)
	}
	if got := LangMarket("tr").Code; got != "tr-tr" {
		t.Errorf("Turkish pages lead to %q, want tr-tr", got)
	}
	if !slices.Contains(Languages(), "tr") {
		t.Error("Turkish isn't on the site in preview")
	}
	if len(All) != len(live)+len(Pending) {
		t.Fatalf("preview has %d markets, want %d", len(All), len(live)+len(Pending))
	}
	for i, m := range live {
		if All[i].Code != m.Code {
			t.Errorf("preview moved the live markets: %s is at %d", m.Code, i)
		}
	}
}
