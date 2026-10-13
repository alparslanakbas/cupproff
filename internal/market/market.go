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
	Lang     string // BCP 47 tag for <html lang> and hreflang
	Country  string // ISO 3166-1 alpha-2, sent to Shopify as country=
	Currency string // ISO 4217, sent to Shopify as currency=
	Name     string // shown in the region selector
}

// US is the first market.
var US = Market{Code: "en-us", Lang: "en-US", Country: "US", Currency: "USD", Name: "United States"}

// All lists the live markets, in the order the region selector shows them.
var All = []Market{US}

// ByCode finds a live market by its URL segment.
func ByCode(code string) (Market, bool) {
	for _, m := range All {
		if m.Code == code {
			return m, true
		}
	}
	return Market{}, false
}
