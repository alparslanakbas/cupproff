package shopify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alparslanakbas/cupproof/internal/market"
)

func TestParseWeight(t *testing.T) {
	cases := []struct {
		in    string
		grams int
	}{
		{"12 oz", 340},
		{"12oz / Whole Bean", 340},
		{"Whole Bean / 2 lb", 907},
		{"5 lbs", 2268},
		{"340g", 340},
		{"1 kg bag", 1000},
		{"250 Grams", 250},
		{"2 x 12 oz", 680},
		{"0,5 kg", 500},
		{"32 fl oz cold brew", 0}, // a volume, not a weight
		{"Whole Bean", 0},
		{"Hologram", 0},
	}
	for _, c := range cases {
		if got, _ := ParseWeight(c.in); got != c.grams {
			t.Errorf("ParseWeight(%q) = %d, want %d", c.in, got, c.grams)
		}
	}
}

func ptr(s string) *string { return &s }

func TestParseWeightLabelsAreUniform(t *testing.T) {
	cases := map[string]string{"12oz": "12 oz", "2lbs": "2 lb", "340 Grams": "340 g", "0,5 kg": "0.5 kg", "2 x 12 oz": "2 x 12 oz"}
	for in, want := range cases {
		if _, got := ParseWeight(in); got != want {
			t.Errorf("ParseWeight(%q) label = %q, want %q", in, got, want)
		}
	}
}

// Real leaks from the first live run (13 Oct 2026): a size in the text made
// cups, gifts and unroasted beans look like coffee bags.
func TestToOffersSkipsNonCoffeeProductTypes(t *testing.T) {
	products := []Product{
		{Title: "Black Cat Project Latte Cup Large 12oz", ProductType: "Drinkware", Variants: []Variant{{Price: "20.00"}}},
		{Title: "Coffee Blossom Honey 12oz", ProductType: "Gifts", Variants: []Variant{{Price: "15.00"}}},
		{Title: "Unroasted Colombia Popayán Cauca", ProductType: "Green", Variants: []Variant{{Title: "5 lb", Price: "40.00"}}},
		{Title: "Colombia El Vergel Tag Team", ProductType: "", Variants: []Variant{{Title: "12 oz", Price: "24.00", Available: true}}},
	}
	offers, skipped := ToOffers(products)
	if len(offers) != 1 || offers[0].Title != "Colombia El Vergel Tag Team" || len(skipped) != 3 {
		t.Fatalf("offers=%+v skipped=%+v; want only the coffee without a product type", offers, skipped)
	}
}

func TestToOffersFoldsGrindIntoSizeAndKeepsCheapestAvailable(t *testing.T) {
	p := Product{
		ID: 1, Title: "Ethiopia Guji", Handle: "ethiopia-guji",
		Variants: []Variant{
			{ID: 10, Option1: ptr("12 oz"), Option2: ptr("Whole Bean"), Price: "19.50", Available: true},
			{ID: 11, Option1: ptr("12 oz"), Option2: ptr("Ground"), Price: "19.00", Available: false},
			{ID: 12, Option1: ptr("12 oz"), Option2: ptr("Espresso Grind"), Price: "19.25", Available: true, CompareAtPrice: ptr("22.00")},
			{ID: 13, Option1: ptr("5 lb"), Option2: ptr("Whole Bean"), Price: "98.00", Available: true},
		},
	}
	offers, skipped := ToOffers([]Product{p})
	if len(skipped) != 0 || len(offers) != 2 {
		t.Fatalf("offers=%d skipped=%v, want 2 offers", len(offers), skipped)
	}
	small := offers[0]
	if small.WeightGrams != 340 || small.VariantID != 12 || small.PriceMinor != 1925 || small.CompareAtMinor != 2200 {
		t.Errorf("12 oz offer = %+v; want the cheapest AVAILABLE variant (12) with its compare-at", small)
	}
	if offers[1].WeightGrams != 2268 || offers[1].PriceMinor != 9800 {
		t.Errorf("5 lb offer = %+v", offers[1])
	}
}

func TestToOffersSkipsNonCoffeeAndMissingSizeWithoutGuessing(t *testing.T) {
	products := []Product{
		{Title: "Logo Mug 12 oz", Handle: "mug", Variants: []Variant{{Title: "Default", Price: "18.00", Available: true}}},
		{Title: "Cup of Excellence Colombia 8 oz", Handle: "coe", Variants: []Variant{{Title: "Default Title", Price: "30.00", Available: true}}},
		{Title: "House Blend", Handle: "house", Variants: []Variant{{Title: "Default Title", Price: "16.00", Available: true}}},
	}
	offers, skipped := ToOffers(products)
	if len(offers) != 1 || offers[0].Handle != "coe" || offers[0].WeightGrams != 227 {
		t.Fatalf("offers = %+v; want only the Cup of Excellence lot (size from its title)", offers)
	}
	if len(skipped) != 2 {
		t.Fatalf("skipped = %+v; want the mug and the size-less blend", skipped)
	}
}

func TestParseMinorNeverUsesFloat(t *testing.T) {
	cases := map[string]int64{"19.99": 1999, "20": 2000, "0.5": 50, "1234.10": 123410}
	for in, want := range cases {
		if got, err := parseMinor(in); err != nil || got != want {
			t.Errorf("parseMinor(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := parseMinor("1.999"); err == nil {
		t.Error("three decimals must be rejected, not rounded")
	}
}

// Store JSON is not always clean: one sloppy store must not stop an ingest.
func TestDecodeToleratesDuplicateNamesAndInvalidUTF8(t *testing.T) {
	body := []byte("{\"products\":[{\"id\":7,\"title\":\"Café\",\"title\":\"Cafe Blend\"}]}")
	products, err := DecodeProducts(body)
	if err != nil || len(products) != 1 || products[0].ID != 7 || products[0].Title != "Cafe Blend" {
		t.Fatalf("products=%+v err=%v", products, err)
	}
}

func newTestClient(srv *httptest.Server, pages int) *Client {
	tr := NewTransport()
	tr.TLSClientConfig = srv.Client().Transport.(*http.Transport).TLSClientConfig
	return &Client{HTTP: &http.Client{Transport: tr}, UserAgent: "test", MaxPages: pages}
}

func TestFetchProductsSendsMarketAndReadsPastAShortPage(t *testing.T) {
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.RawQuery)
		switch r.URL.Query().Get("page") {
		case "1":
			fmt.Fprint(w, `{"products":[{"id":1,"title":"A"}]}`) // short page, but not the end
		case "2":
			fmt.Fprint(w, `{"products":[{"id":2,"title":"B"}]}`)
		default:
			fmt.Fprint(w, `{"products":[]}`)
		}
	}))
	defer srv.Close()
	products, err := newTestClient(srv, 10).FetchProducts(context.Background(), srv.URL, market.US)
	if err != nil || len(products) != 2 {
		t.Fatalf("products=%d err=%v; want 2 (the short first page must not end the catalog)", len(products), err)
	}
	if want := "country=US&currency=USD&limit=250&page=1"; queries[0] != want {
		t.Errorf("first query = %q, want %q (market must be explicit)", queries[0], want)
	}
}

// With an Accept-Encoding header Cloudflare challenged our requests to
// Shopify storefronts (403); without it they passed (13 Oct 2026). Turning
// compression back on "for efficiency" must fail here, not in production.
func TestStoreRequestsCarryNoAcceptEncoding(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Header.Get("Accept-Encoding"))
		fmt.Fprint(w, `{"products":[]}`)
	}))
	defer srv.Close()
	if _, err := newTestClient(srv, 1).FetchProducts(context.Background(), srv.URL, market.US); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "" {
		t.Fatalf("Accept-Encoding = %q, want none", got)
	}
}

func TestFetchProductsErrors(t *testing.T) {
	cases := []struct {
		status int
		header string
		want   error
	}{
		{404, "", ErrNotShopify},
		{429, "", ErrRateLimited},
		{403, "challenge", ErrChallenged},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c.header != "" {
				w.Header().Set("Cf-Mitigated", c.header)
			}
			w.WriteHeader(c.status)
		}))
		_, err := newTestClient(srv, 3).FetchProducts(context.Background(), srv.URL, market.US)
		if !errors.Is(err, c.want) {
			t.Errorf("HTTP %d: err = %v, want %v", c.status, err, c.want)
		}
		srv.Close()
	}
}
