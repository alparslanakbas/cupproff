package views

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
)

// tr is the catalog text id in the language the page renders in: the
// market's, from the page's address (see web.withLang).
func tr(ctx context.Context, id string, pairs ...string) string {
	return i18n.T(i18n.Lang(ctx), id, pairs...)
}

// trn is tr for a counted text.
func trn(ctx context.Context, id string, n int64, pairs ...string) string {
	return i18n.N(i18n.Lang(ctx), id, n, pairs...)
}

// trh is tr for a text that carries its own markup: a link or a bold
// phrase inside a sentence, where the words around it move with the
// language. The catalog is ours, so it is written out as it is; the
// catalog test checks that every language keeps English's tags and links.
func trh(ctx context.Context, id string, pairs ...string) templ.Component {
	return templ.Raw(tr(ctx, id, pairs...))
}

// localPages are the pages outside the markets that have a version in each
// of the site's languages, at "/de/about": the information pages, the
// watchlist and the comparison, the tools. A page joins only once all of its
// text is in the catalog, so a link never leads to a half-translated page.
// A path ending in "/" stands for every page under it.
var localPages = []string{"/how-it-works", "/about", "/contact", "/glossary", "/privacy", "/cookies", "/terms",
	"/watchlist", "/compare", "/calculators/", "/which-coffee", "/black-friday", "/cheapest-coffee"}

// IsLocalPage says whether path (no language, no query) has language versions.
func IsLocalPage(path string) bool {
	path, _, _ = strings.Cut(path, "#")
	path, _, _ = strings.Cut(path, "?")
	for _, p := range localPages {
		if path == p || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
			return true
		}
	}
	return false
}

// LocalPath is path's address in lang: "/about" is "/de/about" in German.
// English, and pages without language versions, keep their address.
func LocalPath(lang, path string) string {
	if lang == i18n.Source || !IsLocalPage(path) {
		return path
	}
	return "/" + lang + path
}

// SplitLang takes a language off an address: "/de/about" is "de" and
// "/about". Anything else is English, as it is.
func SplitLang(path string) (lang, rest string) {
	seg, tail, ok := strings.Cut(strings.TrimPrefix(path, "/"), "/")
	if ok && slices.Contains(market.Languages(), seg) {
		return seg, "/" + tail
	}
	return i18n.Source, path
}

// lp is LocalPath in the page's language, for a template's links.
func lp(ctx context.Context, path string) string {
	return LocalPath(i18n.Lang(ctx), path)
}

// homeURL is where a page's "Home" leads: its market's list in its
// language, or the home page.
func homeURL(p Page) string {
	if p.MarketBase != "" {
		return p.MarketBase
	}
	return "/"
}

// localURL sends a footer link to the page's own market and language.
// English pages keep linking the US lists, as they always have.
func localURL(ctx context.Context, p Page, url string) string {
	if i18n.Lang(ctx) == i18n.Source || p.MarketBase == "" {
		return url
	}
	if url == "/" {
		return p.MarketBase
	}
	if rest, ok := strings.CutPrefix(url, "/en-us/"); ok {
		return p.MarketBase + rest
	}
	return url
}

// verdictTitle is a size's verdict on the product page.
func verdictTitle(ctx context.Context, o Offer) string {
	switch {
	case !o.Available:
		return tr(ctx, "deal.out_of_stock")
	case o.LowestSince != "":
		return tr(ctx, "deal.lowest_since", "date", o.LowestSince)
	case o.LowestIn30:
		return tr(ctx, "deal.lowest_30")
	case o.RealDrop > 0:
		return tr(ctx, "deal.real_drop", "pct", fmt.Sprint(o.RealDrop))
	case o.StoreWas != "":
		return tr(ctx, "deal.store_sale_not_drop")
	case o.RateNote != "":
		return o.RateNote
	case o.Usual == "":
		return tr(ctx, "deal.too_new")
	}
	return tr(ctx, "deal.none_today")
}

// sizeNoun is "250 g bag": the label's word in lower case, except in German,
// which writes every noun with a capital ("250 g Beutel").
func sizeNoun(ctx context.Context, size, word string) string {
	if word == "" {
		return size
	}
	if i18n.Lang(ctx) != "de" {
		word = lower(word)
	}
	return size + " " + word
}

// currentRegion is the short name of the region the page belongs to.
func currentRegion(ctx context.Context, regions []NavLink) string {
	for _, r := range regions {
		if r.Current {
			return r.Name
		}
	}
	return tr(ctx, "nav.region")
}

// tabs are the phone's tab bar: the deals, then the menu's sections that
// have a tab icon (by link, not by name: the names change with the language).
func tabs(ctx context.Context, p Page) []StripItem {
	items := []StripItem{{Label: tr(ctx, "nav.deals"), URL: localURL(ctx, p, "/"), Icon: "home"}}
	for _, n := range p.Nav {
		if n.Icon != "" {
			items = append(items, StripItem{Label: n.Name, URL: n.URL, Icon: n.Icon, Current: n.Current})
		}
	}
	return items
}

// themeTitle names a theme control.
func themeTitle(ctx context.Context, name string) string {
	switch name {
	case "light":
		return tr(ctx, "theme.light")
	case "dark":
		return tr(ctx, "theme.dark")
	}
	return tr(ctx, "theme.system")
}

// LangLink is the page in one of its market's languages.
type LangLink struct {
	Lang, Name, URL string // "de", "Deutsch", "/de-eu/…"
	Current         bool
}

// currentLang is the language the selector shows as chosen.
func currentLang(langs []LangLink) string {
	for _, l := range langs {
		if l.Current {
			return l.Lang
		}
	}
	return ""
}

// currentName is the chosen language's own name: "Deutsch".
func currentName(langs []LangLink) string {
	for _, l := range langs {
		if l.Current {
			return l.Name
		}
	}
	return ""
}

// flag is a language's flag, drawn plain: at 20 px a coat of arms would be a
// smudge, and a file per flag would be a request per flag. English shows the
// Union Jack.
func flag(lang string) templ.Component {
	return templ.Raw(flags[lang])
}

const flagAttrs = `class="flag" width="20" height="15" preserveAspectRatio="none" aria-hidden="true"`

var flags = map[string]string{
	"en": `<svg ` + flagAttrs + ` viewBox="0 0 60 30"><path fill="#012169" d="M0 0h60v30H0z"/>` +
		`<path stroke="#fff" stroke-width="6" d="M0 0l60 30M60 0L0 30"/><path stroke="#C8102E" stroke-width="2" d="M0 0l60 30M60 0L0 30"/>` +
		`<path stroke="#fff" stroke-width="10" d="M30 0v30M0 15h60"/><path stroke="#C8102E" stroke-width="6" d="M30 0v30M0 15h60"/></svg>`,
	"de": `<svg ` + flagAttrs + ` viewBox="0 0 5 3"><path fill="#000" d="M0 0h5v1H0z"/><path fill="#D00" d="M0 1h5v1H0z"/>` +
		`<path fill="#FFCE00" d="M0 2h5v1H0z"/></svg>`,
	"fr": `<svg ` + flagAttrs + ` viewBox="0 0 3 2"><path fill="#002395" d="M0 0h1v2H0z"/><path fill="#fff" d="M1 0h1v2H1z"/>` +
		`<path fill="#ED2939" d="M2 0h1v2H2z"/></svg>`,
	"es": `<svg ` + flagAttrs + ` viewBox="0 0 3 4"><path fill="#AA151B" d="M0 0h3v4H0z"/><path fill="#F1BF00" d="M0 1h3v2H0z"/></svg>`,
	"it": `<svg ` + flagAttrs + ` viewBox="0 0 3 2"><path fill="#009246" d="M0 0h1v2H0z"/><path fill="#fff" d="M1 0h1v2H1z"/>` +
		`<path fill="#CE2B37" d="M2 0h1v2H2z"/></svg>`,
	"pt": `<svg ` + flagAttrs + ` viewBox="0 0 5 3"><path fill="#046A38" d="M0 0h2v3H0z"/><path fill="#DA291C" d="M2 0h3v3H2z"/></svg>`,
	"nl": `<svg ` + flagAttrs + ` viewBox="0 0 3 3"><path fill="#AE1C28" d="M0 0h3v1H0z"/><path fill="#fff" d="M0 1h3v1H0z"/>` +
		`<path fill="#21468B" d="M0 2h3v1H0z"/></svg>`,
}
