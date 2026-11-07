package web

import (
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The sister sites' tools, for coffee and tea (4 Nov 2026): the categories,
// the calculators, the "which one?" quiz, the Black Friday check and the
// cheapest list. Like every page here they work without a script: a form
// sends its answers in the address, and the server works them out.

// categoriesPage lists a market's categories with what each has in stock.
func (s *Server) categoriesPage(w http.ResponseWriter, r *http.Request) {
	m, ok := market.ByCode(r.PathValue("market"))
	lang := m.Language()
	if !ok {
		s.notFound(w, r)
		return
	}
	var groups []views.CategoryGroup
	for _, k := range kinds {
		base := "/" + m.Code + "/" + k.listPath
		total, err := s.Q.CountCoffees(r.Context(), db.CountCoffeesParams{Market: m.Key, Kind: k.name})
		if err != nil {
			s.fail(w, r, err)
			return
		}
		g := views.CategoryGroup{Name: k.text(lang, "nav"), All: k.text(lang, "all_link"), URL: base, Icon: k.icon, Count: k.count(lang, "in_stock", total)}
		for _, c := range k.categories {
			n, err := s.Q.CountCoffees(r.Context(), db.CountCoffeesParams{Market: m.Key, Kind: k.name, Title: c.include, TitleNot: c.exclude})
			if err != nil {
				s.fail(w, r, err)
				return
			}
			g.Items = append(g.Items, views.CategoryCard{Label: c.label(lang), URL: base + "?cat=" + c.slug, Icon: c.icon,
				Count: i18n.T(lang, "cat.in_stock_today", "n", fmt.Sprint(n)), About: c.about(lang)})
		}
		groups = append(groups, g)
	}
	p := views.Page{
		Title:       i18n.T(lang, "categories.title", "in_market", inMarket(lang, m)),
		Description: i18n.T(lang, "categories.desc", "in_market", inMarket(lang, m)),
		Lang:        m.Lang, Canonical: s.SiteURL + "/" + m.Code + "/categories/", Nav: nav(m, ""),
	}
	paths := map[string]string{}
	for _, other := range market.All {
		paths[other.Key] = "/" + other.Code + "/categories/"
	}
	p = s.inMarkets(p, m, coffeeKind, paths, "")
	p = chrome(r, p, "/"+m.Code+"/")
	s.render(w, r, http.StatusOK, views.Categories(p, m.Short, groups))
}

// calculatorList is the calculators page, in its order.
var calculatorList = []struct{ slug, title, lede, icon string }{
	{"caffeine", "Caffeine calculator", "Add up the caffeine in a day of coffee and tea.", "bolt"},
	{"brew-ratio", "Brew ratio calculator", "How much water for your coffee, or how much coffee for your water.", "drop"},
	{"cost-per-cup", "Cost per cup calculator", "What a bag of coffee works out to per cup, and how many cups it makes.", "cup"},
}

func (s *Server) calculators(w http.ResponseWriter, r *http.Request) {
	var cards []views.CategoryCard
	for _, c := range calculatorList {
		cards = append(cards, views.CategoryCard{Label: c.title, URL: "/calculators/" + c.slug, Icon: c.icon, About: c.lede})
	}
	p := views.Page{Title: "Coffee calculators: caffeine, brew ratio, cost per cup — CoffeeProof", Lang: "en",
		Description: "Free coffee calculators: the caffeine in your day, the water for your coffee, and what a bag costs per cup.",
		Canonical:   s.SiteURL + "/calculators/", Nav: nav(market.US, "calculators")}
	p = chrome(r, p, "/en-us/")
	s.render(w, r, http.StatusOK, views.Calculators(p, cards))
}

func (s *Server) calculator(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	slug := r.PathValue("slug")
	p := views.Page{Lang: "en", Canonical: s.SiteURL + "/calculators/" + slug, NoIndex: len(q) > 0,
		Nav: nav(market.US, "calculators")}
	p = chrome(r, p, "/en-us/")
	switch slug {
	case "caffeine":
		p.Title = "Caffeine calculator: how much caffeine is in your day? — CoffeeProof"
		p.Description = "Add up the caffeine in your coffee, espresso and tea, against the 400 mg a day the FDA and EFSA see as safe for most healthy adults."
		s.render(w, r, http.StatusOK, views.CaffeinePage(p, caffeineFrom(q)))
	case "brew-ratio":
		p.Title = "Coffee brew ratio calculator — CoffeeProof"
		p.Description = "Work out the water for your coffee, or the coffee for your water, at the ratio you brew with."
		s.render(w, r, http.StatusOK, views.BrewRatioPage(p, brewRatio(q)))
	case "cost-per-cup":
		p.Title = "Coffee cost per cup calculator — CoffeeProof"
		p.Description = "What a bag of coffee costs per cup, and how many cups it makes at your dose."
		s.render(w, r, http.StatusOK, views.CostPerCupPage(p, costPerCup(q)))
	default:
		s.notFound(w, r)
	}
}

// number reads a form value as a number between lo and hi; ok is false
// when it is missing or out of range.
func number(q url.Values, key string, lo, hi float64) (float64, bool) {
	raw := strings.TrimSpace(strings.ReplaceAll(q.Get(key), ",", "."))
	if raw == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(v) || v < lo || v > hi {
		return 0, false
	}
	return v, true
}

// brewRatios are the calculator's ratios, coffee to water by weight (a
// gram of water is a millilitre).
var brewRatios = []struct {
	value float64
	label string
}{
	{15, "1:15 (stronger)"}, {16, "1:16"}, {17, "1:17"}, {18, "1:18 (lighter)"}, {2, "1:2 (a common espresso ratio)"},
}

func brewRatio(q url.Values) views.BrewRatio {
	b := views.BrewRatio{Coffee: "20", Ratio: "16"}
	ratio := 16.0
	if v, ok := number(q, "ratio", 1, 30); ok {
		ratio = v
	}
	b.Ratio = strconv.FormatFloat(ratio, 'f', -1, 64)
	for _, r := range brewRatios {
		b.Ratios = append(b.Ratios, views.Option{Value: strconv.FormatFloat(r.value, 'f', -1, 64), Label: r.label, Selected: r.value == ratio})
	}
	coffee, hasCoffee := number(q, "coffee", 1, 2000)
	water, hasWater := number(q, "water", 1, 20000)
	switch {
	case q.Get("from") == "water" && hasWater:
		coffee = water / ratio
		b.Water, b.Coffee = trim(water), ""
		b.Answer = fmt.Sprintf("Use %s g of coffee for %s ml of water.", trim(coffee), trim(water))
		b.From = "water"
	case hasCoffee:
		water = coffee * ratio
		b.Coffee = trim(coffee)
		b.Answer = fmt.Sprintf("Use %s ml of water for %s g of coffee.", trim(water), trim(coffee))
	case len(q) > 0:
		b.Problem = "Enter grams of coffee (1 to 2,000) or millilitres of water (1 to 20,000)."
	default:
		coffee, water = 20, 20*ratio
		b.Answer = fmt.Sprintf("Use %s ml of water for %s g of coffee.", trim(water), trim(coffee))
	}
	if b.Answer != "" && ratio >= 10 {
		b.Cups = fmt.Sprintf("That is about %s cups of 8 oz (237 ml).", trim(water/237))
	}
	return b
}

func costPerCup(q url.Values) views.CostPerCup {
	c := views.CostPerCup{Price: "17.00", Grams: "250", Dose: "15"}
	price, okP := number(q, "price", 0.01, 100000)
	grams, okG := number(q, "grams", 1, 100000)
	dose, okD := number(q, "dose", 1, 100)
	if len(q) == 0 {
		price, grams, dose, okP, okG, okD = 17, 250, 15, true, true, true
	}
	if !okP || !okG || !okD {
		c.Problem = "Enter the bag's price, its size in grams and the grams you use per cup."
		c.Price, c.Grams, c.Dose = q.Get("price"), q.Get("grams"), q.Get("dose")
		return c
	}
	c.Price, c.Grams, c.Dose = strconv.FormatFloat(price, 'f', 2, 64), trim(grams), trim(dose)
	c.PerCup = strconv.FormatFloat(price*dose/grams, 'f', 2, 64)
	c.Cups = strconv.Itoa(int(grams / dose))
	c.PerKilo = strconv.FormatFloat(price*1000/grams, 'f', 2, 64)
	return c
}

// trim shows a number with at most one decimal, without a trailing ".0".
func trim(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

// quizAnswer is one result of the quiz: a category of a section, and why.
type quizAnswer struct {
	kind  kind
	cat   string
	title string
	why   string
}

// whichCoffee is the sister sites' "which one?" quiz: three questions, an
// answer, and today's deals in it. The answers point to a category; they
// don't rank products.
func (s *Server) whichCoffee(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	drink, caffeine, brew, taste := q.Get("drink"), q.Get("caffeine"), q.Get("brew"), q.Get("taste")
	var a *quizAnswer
	switch {
	case drink == "coffee" && caffeine == "less":
		a = &quizAnswer{coffeeKind, "decaf", "Decaf coffee",
			"Decaf keeps the coffee and loses most of the caffeine. A little stays: US rules ask for at least 97% to be removed."}
	case drink == "coffee" && caffeine == "yes" && brew == "espresso":
		a = &quizAnswer{coffeeKind, "espresso", "Espresso coffee", "These are the coffees roasters name for espresso."}
	case drink == "coffee" && caffeine == "yes" && brew == "filter":
		a = &quizAnswer{coffeeKind, "single-origin", "Single-origin coffee",
			"Single origins are named after where they grew; a filter cup is a common way to taste one."}
	case drink == "coffee" && caffeine == "yes" && (brew == "press" || brew == "unsure"):
		a = &quizAnswer{coffeeKind, "blends", "Coffee blends", "A blend is the roaster's recipe for a cup that tastes the same bag after bag."}
	case drink == "tea" && caffeine == "less":
		a = &quizAnswer{teaKind, "herbal", "Herbal tea", "Herbal teas aren't made from the tea plant, so most have no caffeine."}
	case drink == "tea" && caffeine == "yes":
		switch taste {
		case "strong":
			a = &quizAnswer{teaKind, "black", "Black tea", "Fully oxidised: usually the boldest of the teas."}
		case "fresh":
			a = &quizAnswer{teaKind, "green", "Green tea", "Unoxidised, so it stays fresh and green."}
		case "between":
			a = &quizAnswer{teaKind, "oolong", "Oolong", "Partly oxidised: between green and black."}
		case "earthy":
			a = &quizAnswer{teaKind, "pu-erh", "Pu-erh", "Aged teas from Yunnan, deep and earthy."}
		case "whisked":
			a = &quizAnswer{teaKind, "matcha", "Matcha", "Powdered green tea, whisked into water."}
		case "spiced":
			a = &quizAnswer{teaKind, "chai", "Chai", "Tea with spices, or the spice blends for it."}
		}
	}
	quiz := views.Quiz{Drink: drink, Caffeine: caffeine, Brew: brew, Taste: taste}
	if a != nil {
		m := market.US
		base := "/" + m.Code + "/" + a.kind.listPath
		st, _, err := s.buildList(r.Context(), m, a.kind, base, listState{page: 1, cat: a.cat}, 0, 6)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		quiz.Result = &views.QuizResult{Title: a.title, Why: a.why, Cards: st.cards, Labels: a.kind.labels(i18n.Source),
			URL: base + "?cat=" + a.cat, Count: st.total}
		for _, other := range market.All {
			quiz.Result.Markets = append(quiz.Result.Markets, views.NavLink{Name: other.Short,
				URL: "/" + other.Code + "/" + a.kind.listPath + "?cat=" + a.cat})
		}
	}
	p := views.Page{Title: "Which coffee should I buy? A three-question quiz — CoffeeProof", Lang: "en",
		Description: "Three questions to a coffee or tea category, and today's real price drops in it.",
		Canonical:   s.SiteURL + "/which-coffee", NoIndex: len(q) > 0, Nav: nav(market.US, "quiz")}
	p = chrome(r, p, "/en-us/")
	s.render(w, r, http.StatusOK, views.WhichCoffee(p, quiz))
}

// blackFriday is the sister sites' Black Friday check: how many of today's
// sales are real drops, per market, and the deepest ones in the US.
func (s *Server) blackFriday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var rows []views.MarketCount
	for _, m := range market.All {
		row := views.MarketCount{Market: m.Name, URL: "/" + m.Code + "/"}
		for _, k := range kinds {
			drops, err := s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Key, Kind: k.name, Show: "drops"})
			if err != nil {
				s.fail(w, r, err)
				return
			}
			sales, err := s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Key, Kind: k.name, Show: "sales"})
			if err != nil {
				s.fail(w, r, err)
				return
			}
			row.Drops += drops
			row.Sales += sales
		}
		rows = append(rows, row)
	}
	var lists []views.DealsBlock
	for _, k := range kinds {
		for _, show := range []string{"drops", "sales"} {
			base := "/" + market.US.Code + "/" + k.listPath
			st, _, err := s.buildList(ctx, market.US, k, base, listState{page: 1, show: show}, 0, 6)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			title := "Real price drops in US " + k.name
			if show == "sales" {
				title = "Store sales in US " + k.name
			}
			lists = append(lists, views.DealsBlock{Title: title, Cards: st.cards, Labels: k.labels(i18n.Source),
				URL: base + "?show=" + show, Count: st.total, Show: show})
		}
	}
	p := views.Page{Title: "Black Friday coffee deals check: real drops or store claims? — CoffeeProof", Lang: "en",
		Description: "How many of today's coffee and tea sales are real price drops, checked against 30 days of prices, in the US, the UK and the EU.",
		Canonical:   s.SiteURL + "/black-friday", Nav: nav(market.US, "")}
	p = chrome(r, p, "/en-us/")
	s.render(w, r, http.StatusOK, views.BlackFriday(p, rows, lists))
}

// cheapest is the US coffees by price per kg, cheapest first.
func (s *Server) cheapest(w http.ResponseWriter, r *http.Request) {
	m := market.US
	base := "/" + m.Code + "/"
	st, _, err := s.buildList(r.Context(), m, coffeeKind, base, listState{page: 1, sort: "unit_asc"}, 0, 24)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := views.Page{Title: "The cheapest coffee per kg in the US today — CoffeeProof", Lang: "en",
		Description: "Today's US coffees from independent roasters, ordered by price per kilogram.",
		Canonical:   s.SiteURL + "/cheapest-coffee", Nav: nav(m, "")}
	p = chrome(r, p, base)
	s.render(w, r, http.StatusOK, views.Cheapest(p, views.DealsBlock{Title: "Cheapest per kg", Cards: st.cards,
		Labels: coffeeKind.labels(i18n.Source), URL: base + "?sort=unit_asc", Count: st.total}))
}
