// Package i18n holds the site's text in each language and the way each
// language writes prices, numbers and dates.
//
// Texts live in locales/<lang>.json under fixed ids ("reading.under"), not
// under their English wording, so that correcting an English sentence never
// orphans its translations. Wherever languages order the parts of a sentence
// differently, the catalog holds the whole sentence and {name} marks a value
// the caller fills in: Go code never glues the parts of a sentence together.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

// Source is the language every text is written in first. A text missing in
// another language falls back to it.
const Source = "en"

//go:embed locales/*.json
var files embed.FS

var catalogs = load()

func load() map[string]map[string]string {
	entries, err := files.ReadDir("locales")
	if err != nil {
		panic(err)
	}
	out := make(map[string]map[string]string, len(entries))
	for _, e := range entries {
		b, err := files.ReadFile("locales/" + e.Name())
		if err != nil {
			panic(err)
		}
		texts := map[string]string{}
		if err := json.Unmarshal(b, &texts); err != nil {
			panic(fmt.Sprintf("i18n: locales/%s: %v", e.Name(), err))
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = texts
	}
	return out
}

// Has reports whether lang's own catalog, not its fallback, holds id.
func Has(lang, id string) bool {
	_, ok := catalogs[lang][id]
	return ok
}

// T is the text id in lang, its {name} placeholders filled from name, value
// pairs: T("en", "reading.above", "price", "$22.00", "usual", "$20.00"). A
// text missing in lang comes from English; an id missing there too comes
// back as itself, which the catalog tests catch before it reaches a page.
func T(lang, id string, pairs ...string) string {
	s, ok := catalogs[lang][id]
	if !ok {
		if s, ok = catalogs[Source][id]; !ok {
			return id
		}
	}
	if len(pairs) < 2 {
		return s
	}
	r := make([]string, 0, len(pairs))
	for i := 0; i+1 < len(pairs); i += 2 {
		r = append(r, "{"+pairs[i]+"}", pairs[i+1])
	}
	return strings.NewReplacer(r...).Replace(s)
}

// N is T for a counted text. The catalog holds id.one, id.other and any
// form lang needs beyond them; {n} is the count as lang writes numbers.
func N(lang, id string, n int64, pairs ...string) string {
	form := id + "." + plural(lang, n)
	if !Has(lang, form) && !Has(Source, form) {
		form = id + ".other"
	}
	return T(lang, form, append([]string{"n", Number(lang, n)}, pairs...)...)
}

// plural is lang's CLDR plural category for a whole number. Russian and
// Arabic need more categories (zero, two, few, many): add them with those
// languages, and let the catalog test accept their extra forms.
func plural(lang string, n int64) string {
	switch lang {
	case "fr":
		if n == 0 || n == 1 {
			return "one"
		}
	case "ja":
	default:
		if n == 1 {
			return "one"
		}
	}
	return "other"
}
