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

// Every language a market offers has a flag.
func TestFlags(t *testing.T) {
	for lang, svg := range flags {
		if svg == "" || !i18n.Has(lang, "lang.name") {
			t.Errorf("%s: a flag needs its svg and the language's name in its own catalog", lang)
		}
	}
}
