package web

import (
	"net/http"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The sister sites' information pages (4 Nov 2026): how it works, about,
// contact, the glossary and the legal pages. Their text is in
// views/info.templ; what they say about data was checked against the site
// as it runs: no account, no analytics, cookies only for the theme, the
// watchlist and the comparison, and Caddy's access log kept for 90 days.

// infoOrder is the order the footer and the sitemap list them in. Each
// page's title, description and lede are "info.<slug>.title" and so on in
// the catalog; their text, in every language, is under its own ids.
var infoOrder = []string{"how-it-works", "about", "contact", "glossary", "privacy", "cookies", "terms"}

var infoPages = map[string]bool{}

func init() {
	for _, slug := range infoOrder {
		infoPages[slug] = true
	}
}

func (s *Server) info(slug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Lang(r.Context())
		title := i18n.T(lang, "info."+slug+".title")
		p := s.localPage(r, "/"+slug, views.Page{Title: title + " — CoffeeProof", Description: i18n.T(lang, "info."+slug+".desc")}, "")
		facts := views.SiteFacts{}
		if slug == "about" {
			stores, err := s.Q.RoasterFreshness(r.Context())
			if err != nil {
				s.fail(w, r, err)
				return
			}
			facts.Stores = len(stores)
			for _, m := range market.All {
				for _, k := range kinds {
					n, err := s.Q.CountCoffees(r.Context(), db.CountCoffeesParams{Market: m.Key, Kind: k.name})
					if err != nil {
						s.fail(w, r, err)
						return
					}
					facts.Listed += n
				}
			}
		}
		s.render(w, r, http.StatusOK, views.Info(p, slug, title, facts))
	}
}
