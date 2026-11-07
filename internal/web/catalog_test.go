package web

import (
	"testing"

	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
)

// The catalog has every text the code names by building its id, which the
// i18n package's scan of the code can't see: kinds, categories, sorts,
// markets, countries and currencies.
func TestCatalogHasTheTablesTexts(t *testing.T) {
	var ids []string
	for _, k := range kinds {
		for _, f := range []string{"nav", "title", "heading", "lede", "desc", "back", "maker", "size", "unit", "buy",
			"store_note", "all_stores", "sort_unit", "section", "search", "best_unit", "brand_title", "about", "all_link"} {
			ids = append(ids, "kind."+k.name+"."+f)
		}
		for _, f := range []string{"tracked", "count", "card_count", "in_stock"} {
			ids = append(ids, "kind."+k.name+"."+f+".one", "kind."+k.name+"."+f+".other")
		}
		for _, c := range k.categories {
			ids = append(ids, "cat."+c.slug+".label", "cat."+c.slug+".noun", "cat."+c.slug+".about")
		}
	}
	for _, o := range sorts {
		if o.label != "" {
			ids = append(ids, o.label)
		}
	}
	for _, m := range market.All {
		ids = append(ids, "market."+m.Region()+".name", "market."+m.Region()+".in")
		if m.Note != "" {
			ids = append(ids, "market."+m.Region()+".note", "market."+m.Region()+".hint")
		}
	}
	for _, c := range []string{"US", "GB", "DE", "ES", "SE", "DK", "FR", "NO", "FI", "IE", "IT", "NL", "AU", "CA", "NZ",
		"BE", "AT", "CH", "PT"} {
		ids = append(ids, "country."+c)
	}
	for _, c := range []string{"USD", "GBP", "EUR", "DKK", "NOK", "SEK"} {
		ids = append(ids, "currency."+c)
	}
	ids = append(ids, "view.drops", "view.sales", "view.all", "lang.name")
	for _, id := range ids {
		if !i18n.Has(i18n.Source, id) {
			t.Errorf("English lacks %q", id)
		}
	}
}

// A market's other languages answer at their own address with the market's
// data, and only those: /de-us/ is no page.
func TestMarketVersions(t *testing.T) {
	for _, m := range market.All {
		for _, v := range m.Versions() {
			got, ok := market.ByCode(v.Code)
			if !ok || got.Key != m.Key || got.Language() != v.Language() {
				t.Errorf("%s: ByCode = %+v, %v", v.Code, got, ok)
			}
			if got := v.Localize("/" + m.Key + "/coffee/x"); got != "/"+v.Code+"/coffee/x" {
				t.Errorf("%s: Localize = %q", v.Code, got)
			}
		}
	}
	if _, ok := market.ByCode("de-us"); ok {
		t.Error("de-us answers, but the US market has no German pages")
	}
}
