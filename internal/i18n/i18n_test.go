package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var placeholder = regexp.MustCompile(`\{[a-z_]+\}`)

func placeholders(s string) string {
	p := placeholder.FindAllString(s, -1)
	slices.Sort(p)
	return strings.Join(slices.Compact(p), " ")
}

// Every language holds exactly the English ids, each with the same
// placeholders and none empty, and has a number and date style.
func TestCatalogsMatchEnglish(t *testing.T) {
	en := catalogs[Source]
	if len(en) == 0 {
		t.Fatal("no English catalog")
	}
	for lang, texts := range catalogs {
		if _, ok := styles[lang]; !ok {
			t.Errorf("%s has texts but no number and date style", lang)
		}
		for id, s := range texts {
			src, ok := en[id]
			switch {
			case !ok:
				t.Errorf("%s: %q is not in English", lang, id)
			case strings.TrimSpace(s) == "":
				t.Errorf("%s: %q is empty", lang, id)
			case placeholders(s) != placeholders(src):
				t.Errorf("%s: %q has %q, English %q", lang, id, placeholders(s), placeholders(src))
			}
		}
		for id := range en {
			if _, ok := texts[id]; !ok {
				t.Errorf("%s: %q is missing", lang, id)
			}
		}
	}
}

// asked finds the ids the code asks for by name, in Go (i18n.T, i18n.N) and
// in the templates (tr, trn); an id built at run time ("currency."+code) is
// checked by its caller's tests.
var asked = regexp.MustCompile(`(?:i18n\.([TN])|\b(tr|trn))\(\s*[^,()]+,\s*"([^"]+)"\s*[,)]`)

func TestCodeAsksOnlyForKnownIDs(t *testing.T) {
	n := 0
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.HasSuffix(path, "_templ.go") ||
			!(strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".templ")) {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range asked.FindAllStringSubmatch(string(b), -1) {
			n++
			id := m[3]
			if m[1] == "N" || m[2] == "trn" {
				id += ".other"
			}
			if !Has(Source, id) {
				t.Errorf("%s asks for %q, which English lacks", path, id)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("found no i18n.T or i18n.N call: the pattern no longer matches the code")
	}
}

func TestFillsPlaceholdersAndFallsBack(t *testing.T) {
	if got := T("en", "reading.above", "price", "$22.00", "usual", "$20.00"); got != "$22.00 is above its usual $20.00." {
		t.Errorf("T = %q", got)
	}
	if got := T("xx", "reading.usual", "usual", "$1.00"); got != "At its usual price of $1.00." {
		t.Errorf("a language without the text = %q, want the English one", got)
	}
	if got := T("en", "no.such.id"); got != "no.such.id" {
		t.Errorf("missing id = %q, want the id itself", got)
	}
}

func TestPluralForms(t *testing.T) {
	en := catalogs[Source]
	en["test.store.one"], en["test.store.other"] = "{n} store", "{n} stores"
	t.Cleanup(func() { delete(en, "test.store.one"); delete(en, "test.store.other") })
	for _, c := range []struct {
		lang string
		n    int64
		want string
	}{
		{"en", 1, "1 store"}, {"en", 0, "0 stores"}, {"en", 12345, "12,345 stores"},
		{"de", 12345, "12.345 stores"}, // no German text: the English one, in German digits
		{"fr", 0, "0 store"},           // French counts zero as one
		{"ja", 1, "1 stores"},          // Japanese has a single form
	} {
		if got := N(c.lang, "test.store", c.n); got != c.want {
			t.Errorf("N(%s, %d) = %q, want %q", c.lang, c.n, got, c.want)
		}
	}
}

func TestFormats(t *testing.T) {
	day := time.Date(2026, 11, 2, 15, 4, 0, 0, time.UTC)
	for _, c := range []struct{ lang, got, want string }{
		{"en", Money("en", 1925, "USD"), "$19.25"},
		{"en", Money("en", 12345678, "EUR"), "€123456.78"},
		{"en", Money("en", 1925, "SEK"), "SEK 19.25"},
		{"en", Day("en", day), "Nov 2"},
		{"en", Date("en", day), "Nov 2, 2026"},
		{"en", DayTime("en", day.In(time.FixedZone("TRT", 3*3600))), "Nov 2, 15:04"},
		{"de", Money("de", 12345678, "EUR"), "123.456,78 €"},
		{"de", Money("de", 1925, "SEK"), "19,25 SEK"},
		{"de", Date("de", day), "2. Nov. 2026"},
		{"de", DayTime("de", day), "2. Nov., 15:04"},
		{"fr", Money("fr", 123456, "EUR"), "1 234,56 €"},
		{"fr", Date("fr", day), "2 nov. 2026"},
		{"es", Money("es", 123400, "EUR"), "1234,00 €"},
		{"es", Money("es", 1234500, "EUR"), "12.345,00 €"},
		{"pt", Date("pt", day), "2 de nov. de 2026"},
		{"nl", Money("nl", 1925, "EUR"), "€ 19,25"},
		{"tr", Money("tr", 125000, "TRY"), "₺1.250,00"},
		{"tr", Date("tr", day), "2 Kas 2026"},
		{"xx", Money("xx", 1925, "USD"), "$19.25"},
	} {
		if c.got != c.want {
			t.Errorf("%s: %q, want %q", c.lang, c.got, c.want)
		}
	}
}
