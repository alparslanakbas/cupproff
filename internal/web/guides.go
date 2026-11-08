package web

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The guides: editorial pages, one for every market in each of the site's
// languages ("/guides/", "/de/guides/"). Their text is in the catalog, their
// shape in views/guides.templ; every fact was checked against the source it
// cites on 4 Nov 2026, and the design's claims that the sources did not
// back were changed or dropped.

func (s *Server) guidesIndex(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Lang(r.Context())
	p := s.localPage(r, "/guides/", views.Page{Title: i18n.T(lang, "guides.page_title"), Description: i18n.T(lang, "guides.desc")}, "guides")
	s.render(w, r, http.StatusOK, views.GuidesIndex(p))
}

func (s *Server) guide(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("slug") {
	case "coffee-and-health":
		s.healthGuide(w, r)
	case "coffee-history":
		s.historyGuide(w, r)
	default:
		s.notFound(w, r)
	}
}

// drinks are the caffeine calculator's rows, typical values from the Mayo
// Clinic's table (read 4 Nov 2026). A drink's name and serving are
// "drink.<key>.name" and "drink.<key>.serving" in the catalog.
var drinks = []views.Drink{
	{Key: "brewed", Mg: 96},
	{Key: "espresso", Mg: 63},
	{Key: "instant", Mg: 62},
	{Key: "black", Mg: 48},
	{Key: "green", Mg: 29},
}

// healthGuide works the caffeine calculator out on the server: it is a plain
// GET form, so it works without script. With no answers yet it shows two
// cups of brewed coffee, as the design does.
// caffeineFrom works the calculator out from its form: the health guide's
// and the calculators' page share it. An empty form starts at two cups of
// brewed coffee.
func caffeineFrom(lang string, q url.Values) views.Caffeine {
	c := views.Caffeine{}
	for _, d := range drinks {
		d.Name, d.Serving = i18n.T(lang, "drink."+d.Key+".name"), i18n.T(lang, "drink."+d.Key+".serving")
		switch raw := q.Get(d.Key); {
		case len(q) == 0 && d.Key == "brewed":
			d.Count = 2
		case raw != "":
			n, _ := strconv.Atoi(raw)
			d.Count = min(max(n, 0), 10)
		}
		c.Total += d.Count * d.Mg
		c.Drinks = append(c.Drinks, d)
	}
	switch {
	case c.Total <= 200:
		c.Level = "green"
	case c.Total <= 400:
		c.Level = "accent"
	default:
		c.Level = "amber"
	}
	c.Message = i18n.T(lang, "caffeine."+c.Level)
	c.BarPercent = min(100, c.Total*100/600)
	return c
}

func (s *Server) healthGuide(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := i18n.Lang(r.Context())
	c := caffeineFrom(lang, q)
	p := s.localPage(r, "/guides/coffee-and-health", views.Page{Title: i18n.T(lang, "health.page_title"),
		Description: i18n.T(lang, "health.desc"),
		NoIndex:     len(q) > 0, // a filled-in calculator is the same page
	}, "guides")
	// "Back to prices" leads to today's drops in the page's market.
	m := market.LangMarket(lang)
	drops := views.NavLink{Name: i18n.T(lang, "guides.todays_drops", "market", m.Short), URL: "/" + m.Code + "/?show=drops"}
	s.render(w, r, http.StatusOK, views.HealthGuide(p, c, drops))
}

// historyGuide ends with coffees from today's list whose name says Gesha or
// Geisha: the variety behind the auction records, at a price a reader can
// pay, in the page's market (the US in English). The section is left out
// when there are none.
func (s *Server) historyGuide(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Lang(r.Context())
	m := market.LangMarket(lang)
	rows, err := s.Q.ListCoffees(r.Context(), db.ListCoffeesParams{
		Market: m.Key, Kind: coffeeKind.name, PageSize: 2, Title: `ge(i)?sha`,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards, err := s.cards(r.Context(), m, coffeeKind, rows)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := s.localPage(r, "/guides/coffee-history", views.Page{Title: i18n.T(lang, "history.page_title"),
		Description: i18n.T(lang, "history.desc")}, "guides")
	s.render(w, r, http.StatusOK, views.HistoryGuide(p, cards, coffeeKind.labels(lang), m.Short))
}
