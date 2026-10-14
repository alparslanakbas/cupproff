// Package market defines the storefront markets the site covers.
//
// A market is a URL section (/en-us/) plus the country and currency the
// stores are asked to price for. Prices are never converted with our own
// exchange rate: stores round their local prices, so only the price a store
// shows to that country is the real price (measured 11 Oct 2026).
package market

// Market is one locale section of the site.
type Market struct {
	Code     string // URL segment, e.g. "en-us"
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
}

// US is the first market.
var US = Market{Code: "en-us", Lang: "en-US", Country: "US", Currency: "USD", Name: "United States", Short: "US",
	Hreflangs: []string{"en-US"}}

// UK: a store is read for a market when it ships there and shows that
// market's currency (its default one, or through Shopify Markets: see
// shopify.MarketsFor and PresentedCurrency). Most US stores that ship to the
// UK still show USD there and stay out.
var UK = Market{Code: "en-gb", Lang: "en-GB", Country: "GB", Currency: "GBP", Name: "United Kingdom", Short: "UK",
	Hreflangs: []string{"en-GB"}}

// EU is one market for the euro area, in English (design §3): stores are
// asked for Germany's prices. Prices inside the EU differ by country with
// VAT (Tim Wendelboe: 40.95 EUR to Germany, 42.95 EUR to Sweden), so the
// pages say which country they show.
var EU = Market{Code: "en-eu", Lang: "en", Country: "DE", Currency: "EUR", Name: "European Union", Short: "EU",
	Hreflangs: []string{"en-DE", "en-AT", "en-BE", "en-NL", "en-LU", "en-FR", "en-IE", "en-IT", "en-ES", "en-PT",
		"en-FI", "en-EE", "en-LV", "en-LT", "en-SK", "en-SI", "en-GR", "en-CY", "en-MT", "en-HR", "en-BG"},
	Note: "Prices are the ones each store shows a buyer in Germany; with VAT they can differ in other EU countries."}

// All lists the live markets, in the order the region selector shows them.
var All = []Market{US, UK, EU}

// ByCode finds a live market by its URL segment.
func ByCode(code string) (Market, bool) {
	for _, m := range All {
		if m.Code == code {
			return m, true
		}
	}
	return Market{}, false
}
