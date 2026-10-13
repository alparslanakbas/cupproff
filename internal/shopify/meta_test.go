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

func TestMarketsForNeedsShippingAndCurrency(t *testing.T) {
	cases := []struct {
		name string
		meta Meta
		want []string
	}{
		{"US store shipping to the US", Meta{Currency: "USD", ShipsTo: []string{"US", "CA"}}, []string{"en-us"}},
		{"ships to the US but sells in EUR", Meta{Currency: "EUR", ShipsTo: []string{"US", "DE"}}, nil},
		{"does not ship to the US", Meta{Currency: "USD", ShipsTo: []string{"CA"}}, nil},
		{"meta not read: ships nowhere", Meta{}, nil},
	}
	for _, c := range cases {
		read, skipped := MarketsFor(c.meta, []market.Market{market.US, gb})
		var got []string
		for _, m := range read {
			got = append(got, m.Code)
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: read %v, want %v (skipped %v)", c.name, got, c.want, skipped)
		}
		if len(read)+len(skipped) != 2 {
			t.Errorf("%s: every market must be read or skipped with a reason", c.name)
		}
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
