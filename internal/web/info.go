package web

import (
	"net/http"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The sister sites' information pages (4 Nov 2026): how it works, about,
// contact, the glossary and the legal pages. Their text is in
// views/info.templ; what they say about data was checked against the site
// as it runs: no account, no analytics, cookies only for the theme, the
// watchlist and the comparison, and Caddy's access log kept for 90 days.

// infoOrder is the order the footer and the sitemap list them in.
var infoOrder = []string{"how-it-works", "about", "contact", "glossary", "privacy", "cookies", "terms"}

var infoPages = map[string]struct{ title, desc string }{
	"how-it-works": {"How CoffeeProof reads prices", "How we read coffee and tea prices every day, find each product's usual price and tell a real drop from a store's own sale."},
	"about":        {"About CoffeeProof", "CoffeeProof tracks the prices of independent coffee roasters and tea houses in the US, the UK and the EU."},
	"contact":      {"Contact", "How to reach CoffeeProof: wrong prices, stores, our crawler and everything else."},
	"glossary":     {"Coffee and tea glossary", "Short definitions of the coffee, tea and price words used on CoffeeProof."},
	"privacy":      {"Privacy policy", "What CoffeeProof stores about visitors: no account and no analytics; an access log on our server, and cookies only for choices you make."},
	"cookies":      {"Cookie policy", "The three cookies CoffeeProof can set, all for choices you make: the theme, the watchlist and the comparison."},
	"terms":        {"Terms of use", "The terms for using CoffeeProof: prices are read once a day and the store's own site has the final word."},
}

func (s *Server) info(slug string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := infoPages[slug]
		p := views.Page{Title: page.title + " — CoffeeProof", Description: page.desc, Lang: "en",
			Canonical: s.SiteURL + "/" + slug, Nav: nav(market.US, "")}
		p = chrome(r, p, "/en-us/")
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
		s.render(w, r, http.StatusOK, views.Info(p, slug, page.title, facts))
	}
}
