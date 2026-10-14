package shopify

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
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
		{"1/4 Pound", 113}, // Rishi; once read as "4 Pound"
		{"600/750 g", 750}, // two sizes, not the fraction 600/750 (0.8 g)
		{"113g (4 oz.)", 113},
		{"1 Cake (357 Grams)", 357},
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

// Tea houses' catalogs as read on 13 Oct 2026: teaware, candles, samplers and
// wholesale tea are not offers; one-cup samples and boxes of bags are not
// comparable sizes, while the same tea's loose sizes are.
func TestToOffersReadsTeaHouses(t *testing.T) {
	products := []Product{
		{Title: "Shibo Teapot", ProductType: "Teaware", Variants: []Variant{{Title: "150g", Price: "120.00", Available: true}}},
		{Title: "Atelier Candle", ProductType: "Candles", Variants: []Variant{{Title: "8oz", Price: "68.00", Available: true}}},
		{Title: "Tea Sampler", Variants: []Variant{{Title: "Loose / 4 oz. Tin", Price: "30.00", Available: true}}},
		{Title: "Assam Breakfast", ProductType: "Wholesale Tea", Variants: []Variant{{Title: "1 lb", Price: "40.00", Available: true}}},
		{Title: "Dong Ding Oolong", ProductType: "Teas", Variants: []Variant{
			{Title: "3g Sample (1 Serving)", Price: "2.00", Available: true},
			{Title: "10 3g Teabags", Price: "12.00", Available: true},
			{Title: "113g (4 oz.)", Price: "28.00", Available: true},
		}},
		{Title: "Jade Cloud", ProductType: "LOOSE LEAF", Variants: []Variant{
			{Title: "Teaser", Option1: ptr("Teaser"), Price: "5.00", Available: true},
			{Title: "1/4 Pound", Option1: ptr("1/4 Pound"), Price: "24.00", Available: true},
		}},
		{Title: "2019 Bulang Raw Pu-erh", ProductType: "Raw Pu-erh Tea", Variants: []Variant{
			{Title: "25 Grams Sample", Price: "9.00", Available: true},
			{Title: "1 Cake (357 Grams)", Price: "89.00", Available: true},
		}},
		// UK round: a tea estate's honey, sold by weight.
		{Title: "Cornish Honey (Estate Floral) -113g", ProductType: "Honey", Variants: []Variant{{Title: "Default Title", Price: "12.00", Available: true}}},
	}
	offers, skipped := ToOffers(products, Store{Kind: Tea})
	var got []string
	for _, o := range offers {
		got = append(got, fmt.Sprintf("%s %dg", o.Title, o.WeightGrams))
	}
	want := []string{"Dong Ding Oolong 113g", "Jade Cloud 113g", "2019 Bulang Raw Pu-erh 357g"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("offers = %v, want %v (skipped %v)", got, want, skipped)
	}
	// The same rules must not leak into coffee: a roaster's tea stays out.
	if offers, _ := ToOffers(products[4:5], Store{Kind: Coffee}); len(offers) != 0 {
		t.Errorf("a tea read as coffee: %+v", offers)
	}
}

func TestParseWeightLabelsAreUniform(t *testing.T) {
	cases := map[string]string{"12oz": "12 oz", "2lbs": "2 lb", "340 Grams": "340 g", "0,5 kg": "0.5 kg", "2 x 12 oz": "2 x 12 oz", "1/4 Pound": "1/4 lb"}
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
	offers, skipped := ToOffers(products, Store{Kind: Coffee})
	if len(offers) != 1 || offers[0].Title != "Colombia El Vergel Tag Team" || len(skipped) != 3 {
		t.Fatalf("offers=%+v skipped=%+v; want only the coffee without a product type", offers, skipped)
	}
}

// From the second roaster round (13 Oct 2026): a $0 sample, a wholesale
// "(WS)" listing and a white-label product must not become offers; tea and
// "Goods" are not coffee.
func TestToOffersSkipsZeroPricesWholesaleAndTea(t *testing.T) {
	products := []Product{
		{Title: "Spark Sample", Handle: "spark-sample-ws", Variants: []Variant{{Title: "4 oz", Price: "0.00", Available: true}}},
		{Title: "Elefante Extended Fermentation (WS)", Variants: []Variant{{Title: "5 lb", Price: "90.00", Available: true}}},
		{Title: "Red Jasmine 2026", ProductType: "Tea", Variants: []Variant{{Title: "2 oz", Price: "14.00", Available: true}}},
		{Title: "Finca Mercado Bag", ProductType: "Goods", Variants: []Variant{{Title: "12 oz", Price: "20.00", Available: true}}},
		{Title: "IO-E-05", Tags: []string{"bigface", "white-label"}, Variants: []Variant{{Title: "8.8oz", Price: "4.58", Available: true}}},
		// Seen in the coffee listing after the tea round.
		{Title: "12oz Bionics Hot Cup", ProductType: "Warehouse", Variants: []Variant{{Title: "12 oz", Price: "100.00", Available: true}}},
		{Title: "Roasted Coffee - Recurring - 12 Installments", Variants: []Variant{{Title: "250 g", Price: "342.00", Available: true}}},
		{Title: "Corsica - 3oz Filter Packs", ProductType: "Coffee", Variants: []Variant{{Title: "3 oz", Price: "288.00", Available: true}}},
		{Title: "Chai Latte Powder", ProductType: "Barista Provisions", Variants: []Variant{{Title: "3 lb", Price: "64.80", Available: true}}},
		{Title: "Around the World Gift Box", ProductType: "Coffee", Variants: []Variant{{Title: "12 oz", Price: "60.00", Available: true}}},
		// UK round: a roaster's hot chocolate and ceramics by type, matcha typed "Coffee".
		{Title: "Islands 45% Hot Chocolate Powder", ProductType: "Hot Chocolate", Variants: []Variant{{Title: "1kg", Price: "22.00", Available: true}}},
		{Title: "Sam Marks - 8oz Cup", ProductType: "Ceramics", Variants: []Variant{{Title: "8oz", Price: "28.00", Available: true}}},
		{Title: "Matcha Powder Refill Pouch 30g", ProductType: "Coffee", Variants: []Variant{{Title: "30g", Price: "18.00", Available: true}}},
		// EU round: a German subscription, a Spanish one by type, a filter basket ("Sieb").
		{Title: "Copacabana - Classic Espresso Abo 12 Monate", ProductType: "Espresso", Variants: []Variant{{Title: "250g", Price: "150.00", Available: true}}},
		{Title: "Suscripción 'Essentials' Espresso", ProductType: "Suscripción", Variants: []Variant{{Title: "250g", Price: "13.00", Available: true}}},
		{Title: "VST SIEB RIDGED", Variants: []Variant{{Title: "18g", Price: "35.00", Available: true}}},
		{Title: "Lelit Bianca v3 + Formation 2h + 1kg de café", Variants: []Variant{{Title: "Default Title", Price: "2399.00", Available: true}}},
		{Title: "Seltene Kaffees Probierset", Variants: []Variant{{Title: "600/750 g", Price: "79.90", Available: true}}},
		{Title: "14. Barahona AA - Dominikanische Republik Filter", Tags: []string{"archiv"}, Variants: []Variant{{Title: "50g / ganze Bohne", Price: "100.00"}}},
		// Community Coffee: one unit's weight on a multi-unit pack, and creamer.
		{Title: "Signature Blend Ground Coffee Packets 40 Count", ProductType: "Ground", Variants: []Variant{{Title: "40 Count - 2.5 oz. Packet", Price: "84.57", Available: true}}},
		{Title: "Military Match Breakfast Blend Ground Coffee", ProductType: "Military Match", Variants: []Variant{{Title: "12 oz. | 8-pack", Price: "42.96", Available: true}}},
		{Title: "Lactose Free Coffee Creamer", Variants: []Variant{{Title: "11 oz.", Price: "3.29", Available: true}}},
		{Title: "Fraction (Half-Caff - WS)", Tags: []string{"Blends", "wholesale", "wholesale-only"}, Variants: []Variant{{Title: "2lb", Price: "273.00", Available: true}}},
		{Title: "Elefante Extended Fermentation", Variants: []Variant{
			{Title: "12 oz", Price: "0.00", Available: true},
			{Title: "2 lb", Price: "52.00", Available: true},
		}},
		// Heart's retail bags carry a bare "wholesale" tag; they stay.
		{Title: "Ethiopia Halo", Tags: []string{"Single Origin", "wholesale"}, Variants: []Variant{{Title: "8oz", Price: "23.00", Available: true}}},
	}
	offers, _ := ToOffers(products, Store{Kind: Coffee})
	if len(offers) != 2 || offers[0].WeightGrams != 907 || offers[0].PriceMinor != 5200 || offers[1].Title != "Ethiopia Halo" {
		t.Fatalf("offers = %+v; want the consumer 2 lb bag (the $0 size dropped) and Heart's retail bag", offers)
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
	offers, skipped := ToOffers([]Product{p}, Store{Kind: Coffee})
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

// Real coffees whose names look like filtered words (13 Oct 2026): a plant
// variety, a single coffee sold in a box, a flavour named after sugar.
func TestToOffersKeepsCoffeesThatOnlySoundLikeLeaks(t *testing.T) {
	titles := []string{"Finca El Puente - Gesha Variety", "Colombia La Riviera Gesha Box", "Grandma's Sugar Cookie Ground Coffee", "Cup of Excellence Colombia",
		"Fellow Berry Spice", "Ají Fellows Farms"}
	var products []Product
	for _, title := range titles {
		products = append(products, Product{Title: title, ProductType: "Coffee", Variants: []Variant{{Title: "12 oz", Price: "20.00", Available: true}}})
	}
	if offers, skipped := ToOffers(products, Store{Kind: Coffee}); len(offers) != len(titles) {
		t.Fatalf("kept %d of %d; skipped %v", len(offers), len(titles), skipped)
	}
}

func TestToOffersSkipsNonCoffeeAndMissingSizeWithoutGuessing(t *testing.T) {
	products := []Product{
		{Title: "Logo Mug 12 oz", Handle: "mug", Variants: []Variant{{Title: "Default", Price: "18.00", Available: true}}},
		{Title: "Cup of Excellence Colombia 8 oz", Handle: "coe", Variants: []Variant{{Title: "Default Title", Price: "30.00", Available: true}}},
		{Title: "House Blend", Handle: "house", Variants: []Variant{{Title: "Default Title", Price: "16.00", Available: true}}},
	}
	offers, skipped := ToOffers(products, Store{Kind: Coffee})
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

// Harney's catalog as read on 13 Oct 2026: only "Loose" sizes are tea; the
// canned drinks and jerky beside them state a weight too.
func TestToOffersVariantRuleKeepsOnlyMatchingSizes(t *testing.T) {
	products := []Product{
		{Title: "Gyokuro", Variants: []Variant{
			{Title: "Loose / 4 oz. Tin", Price: "55.00", Available: true},
			{Title: "Sachets / Tin of 20 Sachets", Price: "14.00", Available: true},
		}},
		{Title: "Harney & Sons Coconut Water with Cinnamon", Variants: []Variant{{Title: "10.8 oz. Can / Case of 12 Cans", Price: "29.00", Available: true}}},
		{Title: "Iowa Smokehouse Jerky (Assorted Flavors)", Variants: []Variant{{Title: "Homestyle Original / 10 oz. Bag", Price: "22.00", Available: true}}},
		{Title: "Phoenix Honey Orchid", Variants: []Variant{{Title: "Loose / 2 oz. Tin", Price: "30.00", Available: true}}},
		{Title: "Lift Ground Coffee", Tags: []string{"pantry"}, Variants: []Variant{{Title: "Loose / 12 oz. Bag", Price: "18.00", Available: true}}},
	}
	offers, skipped := ToOffers(products, Store{Kind: Tea, VariantMustMatch: regexp.MustCompile(`(?i)\bloose\b`)})
	var got []string
	for _, o := range offers {
		got = append(got, fmt.Sprintf("%s %dg", o.Title, o.WeightGrams))
	}
	if want := "[Gyokuro 113g Phoenix Honey Orchid 57g]"; fmt.Sprint(got) != want {
		t.Fatalf("offers = %v, want %s (skipped %v)", got, want, skipped)
	}
}

// A catalog still going at MaxPages is returned, flagged, not dropped and
// not passed off as complete.
func TestFetchProductsFlagsTheCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"products":[{"id":1,"title":"Gyokuro","handle":"gyokuro"}]}`)
	}))
	defer srv.Close()
	products, err := newTestClient(srv, 2).FetchProducts(context.Background(), srv.URL, market.US)
	if !errors.Is(err, ErrPageCap) || len(products) != 2 {
		t.Fatalf("products=%d err=%v; want both pages and ErrPageCap", len(products), err)
	}
}

// A 5xx is retried once and the second answer is used; a 429 is not
// retried (the scrape breaker decides about refusals).
func TestGetRetriesAServerErrorOnceButNotARefusal(t *testing.T) {
	var calls int
	status := http.StatusServiceUnavailable
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(status)
			return
		}
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	c := newTestClient(srv, 1)
	body, err := c.Get(context.Background(), srv.URL+"/robots.txt", 1024)
	if err != nil || string(body) != "ok" || calls != 2 {
		t.Fatalf("503 then ok: body %q, err %v, calls %d; want one retry", body, err, calls)
	}
	calls, status = 0, http.StatusTooManyRequests
	if _, err := c.Get(context.Background(), srv.URL+"/products.json", 1024); !errors.Is(err, ErrRateLimited) || calls != 1 {
		t.Fatalf("429: err %v, calls %d; want ErrRateLimited without a retry", err, calls)
	}
}
