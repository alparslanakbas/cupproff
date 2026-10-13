package shopify

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alparslanakbas/cupproof/internal/market"
)

var gb = market.Market{Code: "en-gb", Country: "GB", Currency: "GBP"}

// A market is read when the store ships there in the market's currency,
// checked first when it ships there in another default currency, and
// skipped when it doesn't ship there at all.
func TestMarketsForSortsByShippingAndCurrency(t *testing.T) {
	cases := []struct {
		name        string
		meta        Meta
		read, check string
	}{
		{"US store shipping to the US", Meta{Currency: "USD", ShipsTo: []string{"US", "CA"}}, "[en-us]", "[]"},
		{"US store shipping to the UK too", Meta{Currency: "USD", ShipsTo: []string{"US", "GB"}}, "[en-us]", "[en-gb]"},
		{"EUR store shipping to the US", Meta{Currency: "EUR", ShipsTo: []string{"US", "DE"}}, "[]", "[en-us]"},
		{"ships to neither", Meta{Currency: "USD", ShipsTo: []string{"CA"}}, "[]", "[]"},
		{"meta not read: ships nowhere", Meta{}, "[]", "[]"},
	}
	codes := func(ms []market.Market) string {
		out := []string{}
		for _, m := range ms {
			out = append(out, m.Code)
		}
		return fmt.Sprint(out)
	}
	for _, c := range cases {
		read, check, skipped := MarketsFor(c.meta, []market.Market{market.US, gb})
		if codes(read) != c.read || codes(check) != c.check {
			t.Errorf("%s: read %s check %s, want %s and %s (skipped %v)", c.name, codes(read), codes(check), c.read, c.check, skipped)
		}
		if len(read)+len(check)+len(skipped) != 2 {
			t.Errorf("%s: every market must be read, checked or skipped with a reason", c.name)
		}
	}
}

// The presented currency comes from the home page asked for the market's
// country and currency; a page without Shopify.currency is an error, never
// a guess.
func TestPresentedCurrencyReadsTheHomePage(t *testing.T) {
	page := `<script>Shopify.shop = "x.myshopify.com"; Shopify.currency = {"active":"EUR","rate":"1.207374"};</script>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.URL.Query().Get("country") != "DE" || r.URL.Query().Get("currency") != "EUR" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, page)
	}))
	defer srv.Close()
	cur, err := newTestClient(srv, 1).PresentedCurrency(context.Background(), srv.URL, market.EU)
	if err != nil || cur != "EUR" {
		t.Fatalf("currency %q, err %v; want EUR", cur, err)
	}
	page = `<html>no storefront script</html>`
	if cur, err := newTestClient(srv, 1).PresentedCurrency(context.Background(), srv.URL, market.EU); err == nil {
		t.Fatalf("a page without Shopify.currency gave %q, want an error", cur)
	}
}

func TestFetchMetaReadsShippingAndCurrency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/meta.json" || r.Header.Get("Accept-Encoding") != "" {
			t.Errorf("request %s with Accept-Encoding %q", r.URL.Path, r.Header.Get("Accept-Encoding"))
		}
		fmt.Fprint(w, `{"name":"Onyx Coffee Lab","country":"US","currency":"usd","ships_to_countries":["US","CA"]}`)
	}))
	defer srv.Close()
	m, err := newTestClient(srv, 1).FetchMeta(context.Background(), srv.URL)
	if err != nil || m.Currency != "USD" || len(m.ShipsTo) != 2 || m.Country != "US" {
		t.Fatalf("meta = %+v, err %v", m, err)
	}
}
