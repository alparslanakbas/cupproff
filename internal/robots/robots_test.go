package robots

import "testing"

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
