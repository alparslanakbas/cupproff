// Package market defines the storefront markets the site covers.
//
// A market is a URL section (/en-us/) plus the country and currency the
// stores are asked to price for. Prices are never converted with our own
// exchange rate: stores round their local prices, so only the price a store
// shows to that country is the real price (measured 11 Oct 2026).
package market

import (
	"slices"
	"strings"

	"github.com/alparslanakbas/cupproof/internal/i18n"
)

// Market is one locale section of the site.
type Market struct {
	// Key names the market in the data (offers.market, roaster_markets.market,
	// …). It is the market's English URL segment, so the rows written before
	// the site had other languages need no migration.
	Key      string
	Code     string // URL segment of this language version, e.g. "en-us"
	Lang     string // BCP 47 tag for <html lang>
	Country  string // ISO 3166-1 alpha-2, sent to Shopify as country=
	Currency string // ISO 4217, sent to Shopify as currency=
	Name     string // shown in headings and on the region chooser
	Short    string // shown in the header's region links
	// Hreflangs are the language-region values whose searchers this market's
	// pages serve. Google wants a region code from ISO 3166-1, and "EU" is
	// not one, so the euro market lists the euro-area countries itself.
	Hreflangs []string
	// Note says what a buyer must know about these prices, on every page of
	// the market; "" when nothing beyond the site-wide notes.
	Note string
	// Hint is the short form of Note for the region chooser.
	Hint string
	// Variants are the market's pages in other languages than English.
	Variants []Variant
}

// US is the first market.
var US = Market{Key: "en-us", Code: "en-us", Lang: "en-US", Country: "US", Currency: "USD", Name: "United States", Short: "US",
	Hreflangs: []string{"en-US"}}

// UK: a store is read for a market when it ships there and shows that
// market's currency (its default one, or through Shopify Markets: see
// shopify.MarketsFor and PresentedCurrency). Most US stores that ship to the
// UK still show USD there and stay out.
var UK = Market{Key: "en-gb", Code: "en-gb", Lang: "en-GB", Country: "GB", Currency: "GBP", Name: "United Kingdom", Short: "UK",
	Hreflangs: []string{"en-GB"}}

// EU is one market for the euro area, in English (design §3): stores are
// asked for Germany's prices. Prices inside the EU differ by country with
// VAT (Tim Wendelboe: 40.95 EUR to Germany, 42.95 EUR to Sweden), so the
// pages say which country they show.
var EU = Market{Key: "en-eu", Code: "en-eu", Lang: "en", Country: "DE", Currency: "EUR", Name: "European Union", Short: "EU",
	Hreflangs: []string{"en-DE", "en-AT", "en-BE", "en-NL", "en-LU", "en-FR", "en-IE", "en-IT", "en-ES", "en-PT",
		"en-FI", "en-EE", "en-LV", "en-LT", "en-SK", "en-SI", "en-GR", "en-CY", "en-MT", "en-HR", "en-BG"},
	Note: "Prices are the ones each store shows a buyer in Germany; with VAT they can differ in other EU countries.",
	Hint: "Prices as shown in Germany",
	// German first: 9 of the 27 EU stores are German (6 Nov 2026). French,
	// Spanish, Italian, Portuguese and Dutch followed on 7 Nov 2026. Each
	// lists only the euro-area countries where it is spoken.
	Variants: []Variant{
		{Lang: "de", Hreflangs: []string{"de-DE", "de-AT", "de-BE", "de-LU"}},
		{Lang: "fr", Hreflangs: []string{"fr-FR", "fr-BE", "fr-LU"}},
		{Lang: "es", Hreflangs: []string{"es-ES"}},
		{Lang: "it", Hreflangs: []string{"it-IT"}},
		{Lang: "pt", Hreflangs: []string{"pt-PT"}},
		{Lang: "nl", Hreflangs: []string{"nl-NL", "nl-BE"}},
	}}

// All lists the live markets, in the order the region selector shows them.
var All = []Market{US, UK, EU}

// Languages are the site's languages besides English, in the order the
// markets list them: a language is on the site once a market speaks it.
func Languages() []string {
	var langs []string
	for _, m := range All {
		for _, v := range m.Variants {
			if !slices.Contains(langs, v.Lang) {
				langs = append(langs, v.Lang)
			}
		}
	}
	return langs
}

// LangMarket is the first market that speaks lang, in lang: the market a
// page outside the markets (a guide, the watchlist) leads to. English is
// the US, as it always was.
func LangMarket(lang string) Market {
	for _, m := range All {
		for _, v := range m.Versions() {
			if v.Language() == lang {
				return v
			}
		}
	}
	return US
}

// ByCode finds a live market by its URL segment, in any of its languages.
func ByCode(code string) (Market, bool) {
	for _, m := range All {
		if m.Code == code {
			return m, true
		}
		for _, v := range m.Variants {
			if v.Lang+"-"+m.Region() == code {
				return m.In(v), true
			}
		}
	}
	return Market{}, false
}

// Language is the language the market's pages are written in, as the
// catalog names it: "en" for "en-US".
func (m Market) Language() string {
	lang, _, _ := strings.Cut(m.Lang, "-")
	return strings.ToLower(lang)
}

// ByKey finds a live market by its data key.
func ByKey(key string) (Market, bool) {
	for _, m := range All {
		if m.Key == key {
			return m, true
		}
	}
	return Market{}, false
}

// Variant is a market's pages in another language: the same stores and
// prices at their own address, the language before the market's region
// ("de-eu").
type Variant struct {
	Lang string // BCP 47 tag for <html lang>, and the catalog's language
	// Hreflangs as for Market: only the market's countries where the
	// language is spoken, so that a searcher elsewhere isn't sent to prices
	// for Germany.
	Hreflangs []string
}

// Region is the market's part of the address after the language: "eu".
func (m Market) Region() string {
	_, region, _ := strings.Cut(m.Key, "-")
	return region
}

// In is the market's pages in v's language: their own address, language
// and hreflangs, and the market's name, short name and notes from the
// catalog. The short name goes into sentences ("Café · UE"), where the
// English "EU" would read as a foreign word in French.
func (m Market) In(v Variant) Market {
	region := m.Region()
	m.Code, m.Lang, m.Hreflangs = v.Lang+"-"+region, v.Lang, v.Hreflangs
	m.Name = i18n.T(v.Lang, "market."+region+".name")
	m.Short = i18n.T(v.Lang, "market."+region+".short")
	if m.Note != "" {
		m.Note = i18n.T(v.Lang, "market."+region+".note")
		m.Hint = i18n.T(v.Lang, "market."+region+".hint")
	}
	return m
}

// Versions are the market's pages in each of its languages, English first.
func (m Market) Versions() []Market {
	base, ok := ByKey(m.Key)
	if !ok {
		return []Market{m}
	}
	versions := []Market{base}
	for _, v := range base.Variants {
		versions = append(versions, base.In(v))
	}
	return versions
}

// Localize moves a path of the market's English pages to this version's:
// "/en-eu/coffee/x" becomes "/de-eu/coffee/x". Other paths stay as they are.
func (m Market) Localize(path string) string {
	if rest, ok := strings.CutPrefix(path, "/"+m.Key+"/"); ok {
		return "/" + m.Code + "/" + rest
	}
	return path
}
