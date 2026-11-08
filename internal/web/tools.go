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
// sends its answers in the address, and the server works them out. Outside
// English they speak the page's language and show the first market that
// speaks it (the EU in German); in English, the US, as they always did.

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

// calculatorList is the calculators page, in its order. Each one's texts
// are "calc.<slug>.title", ".lede", ".page_title" and ".desc".
var calculatorList = []struct{ slug, icon string }{
	{"caffeine", "bolt"}, {"brew-ratio", "drop"}, {"cost-per-cup", "cup"},
}

func (s *Server) calculators(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Lang(r.Context())
	var cards []views.CategoryCard
	for _, c := range calculatorList {
		cards = append(cards, views.CategoryCard{Label: i18n.T(lang, "calc."+c.slug+".title"),
			URL: views.LocalPath(lang, "/calculators/"+c.slug), Icon: c.icon, About: i18n.T(lang, "calc."+c.slug+".lede")})
	}
	p := s.localPage(r, "/calculators/", views.Page{Title: i18n.T(lang, "calcs.page_title"), Description: i18n.T(lang, "calcs.desc")},
		"calculators")
	s.render(w, r, http.StatusOK, views.Calculators(p, cards))
}

func (s *Server) calculator(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	slug := r.PathValue("slug")
	lang := i18n.Lang(r.Context())
	page := func() views.Page {
		return s.localPage(r, "/calculators/"+slug, views.Page{Title: i18n.T(lang, "calc."+slug+".page_title"),
			Description: i18n.T(lang, "calc."+slug+".desc"), NoIndex: len(q) > 0}, "calculators")
	}
	switch slug {
	case "caffeine":
		s.render(w, r, http.StatusOK, views.CaffeinePage(page(), caffeineFrom(lang, q)))
	case "brew-ratio":
		s.render(w, r, http.StatusOK, views.BrewRatioPage(page(), brewRatio(lang, q)))
	case "cost-per-cup":
		s.render(w, r, http.StatusOK, views.CostPerCupPage(page(), costPerCup(lang, q)))
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
// gram of water is a millilitre); the ones with a word say it in the
// catalog ("brew.ratio.15": "1:15 (stronger)").
var brewRatios = []struct {
	value float64
	named bool
}{
	{15, true}, {16, false}, {17, false}, {18, true}, {2, true},
}

// brewRatio works the brew ratio calculator out. The form keeps its numbers
// as a number field wants them ("18.8"); the answer is in lang ("18,8").
func brewRatio(lang string, q url.Values) views.BrewRatio {
	b := views.BrewRatio{Coffee: "20", Ratio: "16"}
	ratio := 16.0
	if v, ok := number(q, "ratio", 1, 30); ok {
		ratio = v
	}
	b.Ratio = strconv.FormatFloat(ratio, 'f', -1, 64)
	for _, r := range brewRatios {
		value := strconv.FormatFloat(r.value, 'f', -1, 64)
		label := "1:" + value
		if r.named {
			label = i18n.T(lang, "brew.ratio."+value)
		}
		b.Ratios = append(b.Ratios, views.Option{Value: value, Label: label, Selected: r.value == ratio})
	}
	coffee, hasCoffee := number(q, "coffee", 1, 2000)
	water, hasWater := number(q, "water", 1, 20000)
	waterFor := func() string {
		return i18n.T(lang, "brew.use_water", "water", i18n.Short(lang, water), "coffee", i18n.Short(lang, coffee))
	}
	switch {
	case q.Get("from") == "water" && hasWater:
		coffee = water / ratio
		b.Water, b.Coffee = trim(water), ""
		b.Answer = i18n.T(lang, "brew.use_coffee", "coffee", i18n.Short(lang, coffee), "water", i18n.Short(lang, water))
		b.From = "water"
	case hasCoffee:
		water = coffee * ratio
		b.Coffee = trim(coffee)
		b.Answer = waterFor()
	case len(q) > 0:
		b.Problem = i18n.T(lang, "brew.problem")
	default:
		coffee, water = 20, 20*ratio
		b.Answer = waterFor()
	}
	if b.Answer != "" && ratio >= 10 {
		b.Cups = i18n.T(lang, "brew.cups", "cups", i18n.Short(lang, water/237))
	}
	return b
}

// costPerCup works the cost per cup calculator out; like brewRatio, the
// form keeps "17.00" and the answer is in lang.
func costPerCup(lang string, q url.Values) views.CostPerCup {
	c := views.CostPerCup{Price: "17.00", Grams: "250", Dose: "15"}
	price, okP := number(q, "price", 0.01, 100000)
	grams, okG := number(q, "grams", 1, 100000)
	dose, okD := number(q, "dose", 1, 100)
	if len(q) == 0 {
		price, grams, dose, okP, okG, okD = 17, 250, 15, true, true, true
	}
	if !okP || !okG || !okD {
		c.Problem = i18n.T(lang, "cost.problem")
		c.Price, c.Grams, c.Dose = q.Get("price"), q.Get("grams"), q.Get("dose")
		return c
	}
	c.Price, c.Grams, c.Dose = strconv.FormatFloat(price, 'f', 2, 64), trim(grams), trim(dose)
	c.PerCup = i18n.Fixed(lang, price*dose/grams, 2)
	c.Cups = strconv.Itoa(int(grams / dose))
	c.PerKilo = i18n.Fixed(lang, price*1000/grams, 2)
	return c
}

// trim shows a number with at most one decimal, without a trailing ".0".
func trim(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

// quizAnswer is one result of the quiz: a category of a section. Its title,
// why and deals heading are "quiz.<cat>.title", ".why" and ".deals".
type quizAnswer struct {
	kind kind
	cat  string
}

// whichCoffee is the sister sites' "which one?" quiz: three questions, an
// answer, and today's deals in it. The answers point to a category; they
// don't rank products.
func (s *Server) whichCoffee(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lang := i18n.Lang(r.Context())
	drink, caffeine, brew, taste := q.Get("drink"), q.Get("caffeine"), q.Get("brew"), q.Get("taste")
	var a *quizAnswer
	switch {
	case drink == "coffee" && caffeine == "less":
		a = &quizAnswer{coffeeKind, "decaf"}
	case drink == "coffee" && caffeine == "yes" && brew == "espresso":
		a = &quizAnswer{coffeeKind, "espresso"}
	case drink == "coffee" && caffeine == "yes" && brew == "filter":
		a = &quizAnswer{coffeeKind, "single-origin"}
	case drink == "coffee" && caffeine == "yes" && (brew == "press" || brew == "unsure"):
		a = &quizAnswer{coffeeKind, "blends"}
	case drink == "tea" && caffeine == "less":
		a = &quizAnswer{teaKind, "herbal"}
	case drink == "tea" && caffeine == "yes":
		switch taste {
		case "strong":
			a = &quizAnswer{teaKind, "black"}
		case "fresh":
			a = &quizAnswer{teaKind, "green"}
		case "between":
			a = &quizAnswer{teaKind, "oolong"}
		case "earthy":
			a = &quizAnswer{teaKind, "pu-erh"}
		case "whisked":
			a = &quizAnswer{teaKind, "matcha"}
		case "spiced":
			a = &quizAnswer{teaKind, "chai"}
		}
	}
	quiz := views.Quiz{Drink: drink, Caffeine: caffeine, Brew: brew, Taste: taste}
	if a != nil {
		m := market.LangMarket(lang)
		base := "/" + m.Code + "/" + a.kind.listPath
		st, _, err := s.buildList(r.Context(), m, a.kind, base, listState{page: 1, cat: a.cat}, 0, 6)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		quiz.Result = &views.QuizResult{Title: i18n.T(lang, "quiz."+a.cat+".title"), Why: i18n.T(lang, "quiz."+a.cat+".why"),
			Cat: a.cat, Market: m.Short, Cards: st.cards, Labels: a.kind.labels(lang), URL: base + "?cat=" + a.cat, Count: st.total}
		for _, other := range market.All {
			v := other.VersionIn(lang)
			quiz.Result.Markets = append(quiz.Result.Markets, views.NavLink{Name: i18n.T(lang, "market."+other.Region()+".short"),
				URL: "/" + v.Code + "/" + a.kind.listPath + "?cat=" + a.cat})
		}
	}
	p := s.localPage(r, "/which-coffee", views.Page{Title: i18n.T(lang, "quiz.page_title"), Description: i18n.T(lang, "quiz.desc"),
		NoIndex: len(q) > 0}, "quiz")
	s.render(w, r, http.StatusOK, views.WhichCoffee(p, quiz))
}

// blackFriday is the sister sites' Black Friday check: how many of today's
// sales are real drops, per market, and the deepest ones in the page's
// market.
func (s *Server) blackFriday(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.Lang(ctx)
	var rows []views.MarketCount
	for _, m := range market.All {
		row := views.MarketCount{Market: i18n.T(lang, "market."+m.Region()+".name"), URL: "/" + m.VersionIn(lang).Code + "/"}
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
	lm := market.LangMarket(lang)
	var lists []views.DealsBlock
	for _, k := range kinds {
		for _, show := range []string{"drops", "sales"} {
			base := "/" + lm.Code + "/" + k.listPath
			st, _, err := s.buildList(ctx, lm, k, base, listState{page: 1, show: show}, 0, 6)
			if err != nil {
				s.fail(w, r, err)
				return
			}
			lists = append(lists, views.DealsBlock{Title: i18n.T(lang, "bf."+show+"."+k.name, "market", lm.Short), Cards: st.cards,
				Labels: k.labels(lang), URL: base + "?show=" + show, Count: st.total, Show: show})
		}
	}
	p := s.localPage(r, "/black-friday", views.Page{Title: i18n.T(lang, "bf.page_title"), Description: i18n.T(lang, "bf.desc")}, "")
	s.render(w, r, http.StatusOK, views.BlackFriday(p, rows, lists))
}

// cheapest is the page's market's coffees by price per kg, cheapest first.
func (s *Server) cheapest(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Lang(r.Context())
	m := market.LangMarket(lang)
	base := "/" + m.Code + "/"
	st, _, err := s.buildList(r.Context(), m, coffeeKind, base, listState{page: 1, sort: "unit_asc"}, 0, 24)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := s.localPage(r, "/cheapest-coffee", views.Page{Title: i18n.T(lang, "cheapest.page_title", "market", m.Short),
		Description: i18n.T(lang, "cheapest.desc", "market", m.Short)}, "")
	s.render(w, r, http.StatusOK, views.Cheapest(p, m.Short, views.DealsBlock{Title: i18n.T(lang, "cheapest.block"), Cards: st.cards,
		Labels: coffeeKind.labels(lang), URL: base + "?sort=unit_asc", Count: st.total}))
}
