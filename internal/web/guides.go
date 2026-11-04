package web

import (
	"net/http"
	"strconv"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The guides: editorial pages in English for every market, so one address
// each and no hreflang. Their text is in views/guides.templ; every fact there
// was checked against the source it cites on 4 Nov 2026, and the design's
// claims that the sources did not back were changed or dropped.

func (s *Server) guidesIndex(w http.ResponseWriter, r *http.Request) {
	p := views.Page{
		Title:       "Coffee guides: caffeine, health and coffee's history — CoffeeProof",
		Lang:        "en",
		Description: "Short reads on caffeine, coffee and health, and how coffee travelled the world, every fact linked to its source.",
		Canonical:   s.SiteURL + "/guides/",
		Nav:         nav(market.US, "guides"),
	}
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
// Clinic's table (read 4 Nov 2026).
var drinks = []views.Drink{
	{Key: "brewed", Name: "Brewed coffee", Serving: "8 oz (237 mL) cup", Mg: 96},
	{Key: "espresso", Name: "Espresso", Serving: "1 oz (30 mL) shot", Mg: 63},
	{Key: "instant", Name: "Instant coffee", Serving: "8 oz (237 mL) cup", Mg: 62},
	{Key: "black", Name: "Black tea", Serving: "8 oz (237 mL) cup, brewed", Mg: 48},
	{Key: "green", Name: "Green tea", Serving: "8 oz (237 mL) cup, brewed", Mg: 29},
}

// healthGuide works the caffeine calculator out on the server: it is a plain
// GET form, so it works without script. With no answers yet it shows two
// cups of brewed coffee, as the design does.
func (s *Server) healthGuide(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	c := views.Caffeine{}
	for _, d := range drinks {
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
		c.Level, c.Message = "green", "Within EFSA's 200 mg for a single dose, and within its 200 mg a day for pregnancy."
	case c.Total <= 400:
		c.Level, c.Message = "accent", "Within the 400 mg a day the FDA and EFSA see as safe for most healthy adults, but over EFSA's 200 mg a day for pregnancy."
	default:
		c.Level, c.Message = "amber", "Over 400 mg: more than the FDA and EFSA consider safe for most adults in a day."
	}
	c.BarPercent = min(100, c.Total*100/600)
	p := views.Page{
		Title:       "Coffee and your health: what the research says — CoffeeProof",
		Lang:        "en",
		Description: "How much caffeine is too much, what a big 2017 review linked coffee with, unfiltered coffee and cholesterol, and a caffeine calculator, every claim linked to its source.",
		Canonical:   s.SiteURL + "/guides/coffee-and-health",
		NoIndex:     len(q) > 0, // a filled-in calculator is the same page
		Nav:         nav(market.US, "guides"),
	}
	s.render(w, r, http.StatusOK, views.HealthGuide(p, c))
}

// historyGuide ends with coffees from today's US list whose name says Gesha
// or Geisha: the variety behind the auction records, at a price a reader can
// pay. The section is left out when there are none.
func (s *Server) historyGuide(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.ListCoffees(r.Context(), db.ListCoffeesParams{
		Market: market.US.Code, Kind: coffeeKind.name, PageSize: 2, Title: `ge(i)?sha`,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards, err := s.cards(r.Context(), market.US, coffeeKind, rows)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := views.Page{
		Title:       "How coffee travelled the world — CoffeeProof",
		Lang:        "en",
		Description: "From Sufi communities in Yemen to the Panama auction records: a map, a timeline of 19 moments and the famous names on a coffee bag, every date linked to its source.",
		Canonical:   s.SiteURL + "/guides/coffee-history",
		Nav:         nav(market.US, "guides"),
	}
	s.render(w, r, http.StatusOK, views.HistoryGuide(p, cards, coffeeKind.labels))
}
