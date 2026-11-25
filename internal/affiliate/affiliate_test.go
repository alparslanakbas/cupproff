package affiliate

import (
	"net/url"
	"testing"
)

func TestApprovedStoreGoesThroughAwinWithItsAddress(t *testing.T) {
	l := New(Rules, "")
	product := "https://www.intelligentsia.com/products/black-cat-espresso?variant=123"

	got, err := url.Parse(l.Apply(product))
	if err != nil {
		t.Fatal(err)
	}
	q := got.Query()
	if got.Host != "www.awin1.com" || got.Path != "/cread.php" {
		t.Fatalf("link = %s, want an Awin redirect", got)
	}
	if q.Get("awinmid") != "35273" || q.Get("awinaffid") != "3090545" || q.Get("clickref") != "coffeeproof" {
		t.Errorf("query = %v", q)
	}
	// The product address, variant included, must survive the round trip.
	if q.Get("ued") != product {
		t.Errorf("ued = %q, want %q", q.Get("ued"), product)
	}
}

func TestStoresWithoutARuleStayAsTheyAreUnlessThereIsAFallback(t *testing.T) {
	address := "https://onyxcoffeelab.com/products/geometry?variant=9"

	if got := New(Rules, "").Apply(address); got != address {
		t.Errorf("no fallback: %s", got)
	}
	sovrn := New(Rules, "https://redirect.viglink.com?key=K&u={url}")
	if got := sovrn.Apply(address); got != "https://redirect.viglink.com?key=K&u="+url.QueryEscape(address) {
		t.Errorf("fallback: %s", got)
	}
	// A store's own rule wins over the fallback.
	if got := sovrn.Apply("https://intelligentsia.com/products/x"); got == "https://redirect.viglink.com?key=K&u="+url.QueryEscape("https://intelligentsia.com/products/x") {
		t.Error("the fallback took a store that has its own rule")
	}
}

func TestBadConfigurationAndAddressesLeaveTheLinkAlone(t *testing.T) {
	address := "https://onyxcoffeelab.com/products/geometry"

	// A fallback that isn't https, or has no {url}, is a mistake: off.
	for _, f := range []string{"http://redirect.viglink.com?u={url}", "https://redirect.viglink.com?key=K"} {
		if got := New(nil, f).Apply(address); got != address {
			t.Errorf("fallback %q: %s", f, got)
		}
	}
	for _, a := range []string{"", "/products/x", "mailto:a@b.c"} {
		if got := New(Rules, "https://x.test?u={url}").Apply(a); got != a {
			t.Errorf("address %q: %s", a, got)
		}
	}
	var none *Links
	if got := none.Apply(address); got != address {
		t.Errorf("nil links: %s", got)
	}
}

func TestQueryPairRuleKeepsTheFragmentAndNeverDoublesTheParameter(t *testing.T) {
	l := New([]Rule{{Host: "store.test", Link: "ref=42"}}, "")

	cases := map[string]string{
		"https://store.test/p":         "https://store.test/p?ref=42",
		"https://www.store.test/p?v=1": "https://www.store.test/p?v=1&ref=42",
		"https://store.test/p#reviews": "https://store.test/p?ref=42#reviews",
		"https://store.test/p?ref=7":   "https://store.test/p?ref=7",
	}
	for in, want := range cases {
		if got := l.Apply(in); got != want {
			t.Errorf("Apply(%q) = %q, want %q", in, got, want)
		}
	}
}
