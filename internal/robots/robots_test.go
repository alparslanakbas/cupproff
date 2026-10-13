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
