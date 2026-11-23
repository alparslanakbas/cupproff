package web

import (
	"strings"
	"testing"

	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// Only English product pages are indexed (24 Nov): a translated product page
// is noindex, leaves the sitemap and isn't named in hreflang sets. The
// market's home, lists, categories and brands stay indexed in every language.
func TestOnlyEnglishProductPagesAreIndexed(t *testing.T) {
	for _, m := range []market.Market{market.US, market.UK, market.EU} {
		if !indexedVersion(m) {
			t.Errorf("%s: the English version must stay indexed", m.Code)
		}
	}
	for _, lang := range []string{"de", "fr", "es", "it", "pt", "nl"} {
		v := market.EU.VersionIn(lang)
		if v.Language() != lang {
			t.Fatalf("the EU market has no %s version (got %s)", lang, v.Code)
		}
		if indexedVersion(v) {
			t.Errorf("%s: a translated product page must not be indexed", v.Code)
		}
	}
	in := []views.Alternate{{Lang: "en-US"}, {Lang: "en-GB"}, {Lang: "en-DE"}, {Lang: "de-DE"},
		{Lang: "fr-FR"}, {Lang: "nl-NL"}, {Lang: "x-default"}}
	var langs []string
	for _, a := range indexedAlternates(in) {
		langs = append(langs, a.Lang)
	}
	if got, want := strings.Join(langs, " "), "en-US en-GB en-DE x-default"; got != want {
		t.Errorf("alternates = %s, want %s", got, want)
	}
}
