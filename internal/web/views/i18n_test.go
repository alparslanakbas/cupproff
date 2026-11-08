package views

import (
	"testing"

	"github.com/alparslanakbas/cupproof/internal/i18n"
)

// The footer's category links are named by catalog id.
func TestFooterCategoriesAreInTheCatalog(t *testing.T) {
	for _, c := range footerCategories {
		if !i18n.Has(i18n.Source, c.Name) {
			t.Errorf("English lacks %q", c.Name)
		}
	}
}

// The information pages build their ids from lists ("how."+step+".h"), which
// the catalog test's scan of the code can't see.
func TestInfoPagesAreInTheCatalog(t *testing.T) {
	var ids []string
	for _, s := range howSteps {
		ids = append(ids, "how."+s+".h", "how."+s+".p")
	}
	for _, g := range glossaryGroups {
		ids = append(ids, g.head)
		for _, term := range g.terms {
			ids = append(ids, "glossary."+term+".t", "glossary."+term+".d")
		}
	}
	for _, s := range privacySections {
		ids = append(ids, "privacy."+s+".h", "privacy."+s+".p")
	}
	for _, s := range termsSections {
		ids = append(ids, "terms."+s+".h", "terms."+s+".p")
	}
	for _, c := range cookieRows {
		ids = append(ids, "cookies."+c+".what", "cookies."+c+".when")
	}
	for _, s := range []string{"independent", "sisters", "since"} {
		ids = append(ids, "about."+s+".h", "about."+s+".p")
	}
	for _, s := range []string{"wrong", "store", "else"} {
		ids = append(ids, "contact."+s+".h", "contact."+s+".p")
	}
	for _, slug := range []string{"how-it-works", "about", "contact", "glossary", "privacy", "cookies", "terms"} {
		ids = append(ids, "info."+slug+".title", "info."+slug+".desc", "info."+slug+".lede")
	}
	for _, id := range ids {
		if !i18n.Has(i18n.Source, id) {
			t.Errorf("English lacks %q", id)
		}
	}
}

// The guides build their ids from their tables.
func TestGuidesAreInTheCatalog(t *testing.T) {
	ids := []string{"year.late1400s"}
	for _, s := range stories {
		ids = append(ids, "story."+s.ID+".title", "story."+s.ID+".text", "place."+s.Place)
	}
	for _, e := range eras {
		ids = append(ids, "era."+e.ID)
		for _, m := range e.Moments {
			ids = append(ids, "moment."+m.Year+".text", "place."+m.Place)
		}
	}
	for _, n := range bagNames {
		ids = append(ids, "bag."+n.ID+".name", "bag."+n.ID+".text", "bagkind."+n.Kind)
		if n.LinkURL != "" {
			ids = append(ids, "bag."+n.ID+".link")
		}
	}
	for _, a := range answers {
		ids = append(ids, "answer."+a.ID+".q", "answer."+a.ID+".a", a.Tag)
	}
	for _, c := range goodCards {
		ids = append(ids, "health.good."+c.id+".h", "health.good."+c.id+".p")
	}
	for _, c := range badCards {
		ids = append(ids, "health.bad."+c.id+".h", "health.bad."+c.id+".p")
	}
	for _, c := range carefulItems {
		ids = append(ids, "health.careful."+c.id)
	}
	for _, m := range myths {
		ids = append(ids, "health.myth."+m.id+".q", "health.myth."+m.id+".a")
	}
	for _, s := range []string{"li1", "li2", "li3"} {
		ids = append(ids, "guides.write."+s)
	}
	for _, e := range healthToc {
		ids = append(ids, e.name)
	}
	for _, id := range ids {
		if !i18n.Has(i18n.Source, id) {
			t.Errorf("English lacks %q", id)
		}
	}
}

// A page with language versions is at "/<lang>/…" in each of the site's
// languages and at its own address in English; nothing else moves.
func TestLocalPath(t *testing.T) {
	for _, c := range []struct{ lang, path, want string }{
		{"de", "/about", "/de/about"},
		{"fr", "/watchlist", "/fr/watchlist"},
		{"en", "/about", "/about"},
		{"de", "/en-us/", "/en-us/"},
		{"de", "/guides/", "/de/guides/"},
		{"pt", "/guides/coffee-and-health#filter", "/pt/guides/coffee-and-health#filter"},
		{"es", "/calculators/brew-ratio", "/es/calculators/brew-ratio"},
		{"es", "/calculators", "/calculators"}, // the redirect to "/calculators/" isn't a page
		{"de", "/aboutx", "/aboutx"},
	} {
		if got := LocalPath(c.lang, c.path); got != c.want {
			t.Errorf("LocalPath(%q, %q) = %q, want %q", c.lang, c.path, got, c.want)
		}
	}
	for _, c := range []struct{ path, lang, rest string }{
		{"/de/about", "de", "/about"},
		{"/about", "en", "/about"},
		{"/de-eu/coffee/x", "en", "/de-eu/coffee/x"},
		{"/xx/about", "en", "/xx/about"},
	} {
		if lang, rest := SplitLang(c.path); lang != c.lang || rest != c.rest {
			t.Errorf("SplitLang(%q) = %q, %q, want %q, %q", c.path, lang, rest, c.lang, c.rest)
		}
	}
}

// Every language a market offers has a flag.
func TestFlags(t *testing.T) {
	for lang, svg := range flags {
		if svg == "" || !i18n.Has(lang, "lang.name") {
			t.Errorf("%s: a flag needs its svg and the language's name in its own catalog", lang)
		}
	}
}
