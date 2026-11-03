package robots

import (
	"testing"
	"time"
)

const sample = `
# comment
User-agent: *
Disallow: /checkout
Disallow: /*?*sort_by
Allow: /checkout/thanks$

User-agent: cupproofbot
User-agent: otherbot
Disallow: /private
Allow: /private/ok
Disallow: /*.php$
`

func TestOurGroupWinsOverStar(t *testing.T) {
	r := Parse(sample, "CupProofBot")
	cases := map[string]bool{
		"/checkout":        true, // only "*" disallows it
		"/private":         false,
		"/private/ok/page": true, // longer Allow wins
		"/a.php/b.php":     false,
		"/a.php/b.phpx":    true,
		"/products.json":   true,
	}
	for path, want := range cases {
		if got := r.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestStarGroupWhenNoOwnGroup(t *testing.T) {
	r := Parse(sample, "someone")
	cases := map[string]bool{
		"/checkout":                  false,
		"/checkout/thanks":           true, // the longer Allow rule wins
		"/checkout/thanks/x":         false,
		"/collections/all?sort_by=x": false,
		"/products.json?page=2":      true,
	}
	for path, want := range cases {
		if got := r.Allowed(path); got != want {
			t.Errorf("Allowed(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestEmptyFileAllowsEverything(t *testing.T) {
	if !Parse("", "x").Allowed("/anything") {
		t.Fatal("an empty robots.txt must allow everything")
	}
}

// Shopify's default rules, as Kiss the Hippo served them on 14 Oct 2026:
// the currency check reads the home page with query parameters, which is
// allowed; /cart.js, the lighter alternative, is not.
func TestShopifyDefaultsAllowTheHomePageNotTheCart(t *testing.T) {
	r := Parse("User-agent: *\nDisallow: /cart\nDisallow: /carts\nDisallow: /search\nDisallow: /*?*oseid=*\nDisallow: /*/*?*ls=*&ls=*\n", "cupproofbot")
	if !r.Allowed("/?country=DE&currency=EUR") {
		t.Error("home page with country and currency must be allowed")
	}
	if r.Allowed("/cart.js?country=DE&currency=EUR") {
		t.Error("/cart.js must be disallowed by Disallow: /cart")
	}
}

func TestAllAllowsOnlyWhatEveryTokenAllows(t *testing.T) {
	const body = "User-agent: wheyproofbot\nDisallow: /products.json\n\nUser-agent: *\nAllow: /\n"
	alone := All{Parse(body, "cupproofbot")}
	signed := All{Parse(body, "cupproofbot"), Parse(body, "wheyproofbot")}

	if !alone.Allowed("/products.json") {
		t.Fatal("cupproofbot alone falls under *, which allows it")
	}
	if signed.Allowed("/products.json") {
		t.Fatal("with the signing bot's name, its Disallow must apply")
	}
	if !signed.Allowed("/meta.json") {
		t.Fatal("a path no group disallows stays allowed")
	}
	if !(All{}).Allowed("/products.json") {
		t.Fatal("no robots.txt allows everything")
	}
}

// Shopify's bot review asked how the crawler handles Crawl-delay (2 Nov 2026).
func TestCrawlDelayComesFromTheGroupThatApplies(t *testing.T) {
	cases := []struct {
		body string
		want time.Duration
	}{
		{"User-agent: *\nCrawl-delay: 10\n", 10 * time.Second},
		{"User-agent: cupproofbot\nCrawl-delay: 2.5\n\nUser-agent: *\nCrawl-delay: 30\n", 2500 * time.Millisecond},
		{"User-agent: cupproofbot\nDisallow: /cart\n\nUser-agent: *\nCrawl-delay: 30\n", 0},
		{"User-agent: *\nCrawl-delay: soon\n", 0},
		{"User-agent: *\nCrawl-delay: -5\n", 0},
		{"User-agent: *\nDisallow: /cart\n", 0},
	}
	for _, c := range cases {
		if got := Parse(c.body, "cupproofbot").Delay; got != c.want {
			t.Errorf("%q: delay %v, want %v", c.body, got, c.want)
		}
	}
}

func TestAllTakesTheLongestCrawlDelay(t *testing.T) {
	body := "User-agent: cupproofbot\nCrawl-delay: 2\n\nUser-agent: wheyproofbot\nCrawl-delay: 7\n"
	all := All{Parse(body, "cupproofbot"), Parse(body, "wheyproofbot")}
	if got := all.Delay(); got != 7*time.Second {
		t.Errorf("delay %v, want 7s", got)
	}
}
