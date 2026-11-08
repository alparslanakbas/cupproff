package web

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
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
	// A pending market's texts are written before it opens.
	for _, m := range append(append([]market.Market{}, market.All...), market.Pending...) {
		ids = append(ids, "market."+m.Region()+".name", "market."+m.Region()+".in", "market."+m.Region()+".short")
		if m.Note != "" {
			ids = append(ids, "market."+m.Region()+".note", "market."+m.Region()+".hint")
		}
	}
	for _, c := range []string{"US", "GB", "DE", "ES", "SE", "DK", "FR", "NO", "FI", "IE", "IT", "NL", "AU", "CA", "NZ",
		"BE", "AT", "CH", "PT", "TR"} {
		ids = append(ids, "country."+c)
	}
	for _, c := range []string{"USD", "GBP", "EUR", "DKK", "NOK", "SEK", "TRY"} {
		ids = append(ids, "currency."+c)
	}
	ids = append(ids, "view.drops", "view.sales", "view.all", "lang.name")
	for _, id := range ids {
		if !i18n.Has(i18n.Source, id) {
			t.Errorf("English lacks %q", id)
		}
	}
}

// The tools build their ids from lists and slugs, which the catalog test's
// scan of the code can't see.
func TestToolsAreInTheCatalog(t *testing.T) {
	var ids []string
	for _, c := range calculatorList {
		for _, f := range []string{"title", "lede", "page_title", "desc", "crumb", "intro"} {
			ids = append(ids, "calc."+c.slug+"."+f)
		}
	}
	for _, d := range drinks {
		ids = append(ids, "drink."+d.Key+".name", "drink."+d.Key+".serving")
	}
	for _, level := range []string{"green", "accent", "amber"} {
		ids = append(ids, "caffeine."+level)
	}
	for _, r := range brewRatios {
		if r.named {
			ids = append(ids, "brew.ratio."+strconv.FormatFloat(r.value, 'f', -1, 64))
		}
	}
	for _, c := range []string{"decaf", "espresso", "single-origin", "blends", "herbal", "black", "green", "oolong", "pu-erh", "matcha", "chai"} {
		ids = append(ids, "quiz."+c+".title", "quiz."+c+".why", "quiz."+c+".deals")
	}
	for _, v := range []string{"brew.espresso", "brew.filter", "brew.press", "brew.unsure", "taste.strong", "taste.fresh",
		"taste.between", "taste.earthy", "taste.whisked", "taste.spiced"} {
		ids = append(ids, "quiz."+v)
	}
	for _, k := range kinds {
		ids = append(ids, "bf.drops."+k.name, "bf.sales."+k.name)
	}
	for _, s := range []string{"green", "amber", "raised"} {
		ids = append(ids, "bf.how."+s)
	}
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

// Every page offers every language the site has: the market's own languages
// lead to the same page, another to the first market that has it, at the
// same page when the page exists there, else at that market's list.
func TestLanguageLinks(t *testing.T) {
	s := &Server{SiteURL: "https://example.org"}
	links := func(cur market.Market, paths map[string]string) string {
		var out []string
		for _, l := range s.inMarkets(views.Page{}, cur, coffeeKind, paths, "").Languages {
			mark := ""
			if l.Current {
				mark = "*"
			}
			out = append(out, l.Lang+mark+" "+l.URL)
		}
		return strings.Join(out, ", ")
	}
	de, _ := market.ByCode("de-eu")
	const others = ", fr /fr-eu/coffee/x, es /es-eu/coffee/x, it /it-eu/coffee/x, pt /pt-eu/coffee/x, nl /nl-eu/coffee/x"
	for _, c := range []struct{ name, got, want string }{
		{"US page not sold in the EU", links(market.US, map[string]string{"en-us": "/en-us/coffee/x"}),
			"en* /en-us/coffee/x, de /de-eu/, fr /fr-eu/, es /es-eu/, it /it-eu/, pt /pt-eu/, nl /nl-eu/"},
		{"US page also in the EU", links(market.US, map[string]string{"en-us": "/en-us/coffee/x", "en-eu": "/en-eu/coffee/x"}),
			"en* /en-us/coffee/x, de /de-eu/coffee/x" + others},
		{"German page", links(de, map[string]string{"en-eu": "/de-eu/coffee/x"}), "en /en-eu/coffee/x, de* /de-eu/coffee/x" + others},
	} {
		if c.got != c.want {
			t.Errorf("%s: %s\n want %s", c.name, c.got, c.want)
		}
	}
}
