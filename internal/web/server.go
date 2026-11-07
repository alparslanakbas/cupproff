// Package web serves the site: server-rendered HTML, no client framework.
package web

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/pricing"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

//go:embed static
var staticFiles embed.FS

// Server holds what the handlers need.
type Server struct {
	Q       *db.Queries
	SiteURL string // canonical origin, no trailing slash
	Log     *slog.Logger
}

// Handler returns the site's routes.
func (s *Server) Handler() http.Handler {
	// Go's built-in table has no .woff2, and the distroless image has no
	// system MIME table to fall back on.
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	static, _ := fs.Sub(staticFiles, "static")
	files := http.FileServerFS(static)
	if css, err := fs.ReadFile(static, "site.css"); err == nil {
		sum := sha256.Sum256(css)
		views.StylesheetURL = "/static/site.css?v=" + hex.EncodeToString(sum[:4])
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.home)
	mux.HandleFunc("GET /robots.txt", s.robots)
	mux.HandleFunc("GET /sitemap.xml", s.sitemap)
	mux.Handle("GET /static/{file}", http.StripPrefix("/static/", cacheFor(24*time.Hour, files)))
	// ServeMux would add the trailing slash with a 307 (temporary) redirect
	// (Go 1.26+). Canonical moves must be permanent, so it is done here.
	mux.HandleFunc("GET /{market}", s.addSlash)
	mux.HandleFunc("GET /{market}/{$}", s.listing(coffeeKind))
	// "/{market}/tea" would clash with "/static/{file}" (both match
	// "/static/tea", and ServeMux refuses the pair), so the slash is added
	// by one handler for every section.
	mux.HandleFunc("GET /{market}/{section}", s.addSectionSlash)
	mux.HandleFunc("GET /{market}/tea/{$}", s.listing(teaKind))
	mux.HandleFunc("GET /{market}/coffee/{slug}", s.product(coffeeKind))
	mux.HandleFunc("GET /{market}/tea/{slug}", s.product(teaKind))
	mux.HandleFunc("GET /{market}/brands/{$}", s.brandsIndex)
	mux.HandleFunc("GET /{market}/brands/{slug}", s.brandPage)
	mux.HandleFunc("GET /{market}/categories/{$}", s.categoriesPage)
	// The sister sites' other pages (4 Nov 2026). Literal paths: ServeMux
	// prefers them to the market patterns.
	mux.HandleFunc("GET /theme/{mode}", s.setTheme)
	mux.HandleFunc("GET /watchlist", s.watchlist)
	mux.HandleFunc("GET /watchlist/toggle", s.toggle(watchCookie, maxWatch))
	mux.HandleFunc("GET /compare", s.compare)
	mux.HandleFunc("GET /compare/toggle", s.toggle(compareCookie, maxCompare))
	mux.HandleFunc("GET /compare/clear", s.clearCompare)
	mux.HandleFunc("GET /calculators", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/calculators/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /calculators/{$}", s.calculators)
	mux.HandleFunc("GET /calculators/{slug}", s.calculator)
	mux.HandleFunc("GET /which-coffee", s.whichCoffee)
	mux.HandleFunc("GET /black-friday", s.blackFriday)
	mux.HandleFunc("GET /cheapest-coffee", s.cheapest)
	for slug := range infoPages {
		mux.HandleFunc("GET /"+slug, s.info(slug))
	}
	// The guides are one set for every market. These patterns are narrower
	// than the market ones above, so ServeMux prefers them.
	mux.HandleFunc("GET /guides", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/guides/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /guides/{$}", s.guidesIndex)
	mux.HandleFunc("GET /guides/{slug}", s.guide)
	// A "GET" pattern also answers HEAD, which uptime monitors send (the
	// sister sites' monitor once stayed red because their endpoint didn't).
	mux.HandleFunc("GET /api/health/sources", s.health)
	mux.HandleFunc("/", s.notFound)
	return s.withVisitor(mux)
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m := market.US
	p := views.Page{
		Title: "CoffeeProof — coffee and tea price history from independent sellers", Lang: "en",
		Description: "Compare specialty coffee and tea prices from independent roasters and tea houses, and see whether a sale is a real price drop.",
		Canonical:   s.SiteURL + "/",
		Nav:         nav(m, ""),
	}
	// The home page is every market's x-default: searchers outside the listed
	// regions land here and pick theirs in the header.
	for _, mk := range market.All {
		for _, h := range mk.Hreflangs {
			p.Alternates = append(p.Alternates, views.Alternate{Lang: h, URL: s.SiteURL + "/" + mk.Code + "/"})
		}
		p.Regions = append(p.Regions, views.NavLink{Name: mk.Short, URL: "/" + mk.Code + "/", Current: mk.Key == m.Key})
	}
	p.Alternates = append(p.Alternates, views.Alternate{Lang: "x-default", URL: s.SiteURL + "/"})
	// Today's top deal: the US coffees' deepest real drop; on a day without
	// one, the teas' (4 Nov 2026: US coffee 0, US tea 61), then a store sale.
	h, err := s.hero(ctx, m, coffeeKind, &teaKind)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	teas, err := s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Key, Kind: teaKind.name})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h.Tracked = views.Amount(h.Count+teas, "product tracked", "products tracked")
	h.Heading = "Find the real coffee deal."
	h.Lede = "We check coffee and tea prices every day. We show a price when it really drops, and say when a sale is only the store's own claim."
	// Like the sister sites' home page, the list itself: the US coffees'
	// first page. Its controls lead to the US list, which has every page, so
	// this page never repeats a filtered view under its own address.
	st, _, err := s.buildList(ctx, m, coffeeKind, "/"+m.Code+"/", listState{page: 1}, 0, homeSize)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := st.deals
	d.Pager = views.Pager{}
	d.MoreURL, d.MoreLabel = "/"+m.Code+"/", fmt.Sprintf("See all %d US coffees", st.total)
	p.Strip = strip(m, coffeeKind, "")
	p = chrome(r, p, "/"+m.Code+"/")
	s.render(w, r, http.StatusOK, views.Home(p, views.Listing{Hero: h, Deals: d}))
}

// homeSize is the number of rows on the home page.
const homeSize = 24

// inMarkets fills in what every page of a market shares: hreflang
// alternates to the same page in the markets where it exists (paths, by
// market code), region links (to the same page where it exists, otherwise
// to that market's listing of the section) and the market's note. xDefault
// is a path, or "" for none.
func (s *Server) inMarkets(p views.Page, current market.Market, k kind, paths map[string]string, xDefault string) views.Page {
	for _, m := range market.All {
		path, ok := paths[m.Key]
		if ok {
			for _, h := range m.Hreflangs {
				p.Alternates = append(p.Alternates, views.Alternate{Lang: h, URL: s.SiteURL + path})
			}
		} else {
			path = "/" + m.Code + "/" + k.listPath
		}
		p.Regions = append(p.Regions, views.NavLink{Name: m.Short, URL: path, Current: m.Key == current.Key})
	}
	if xDefault != "" {
		p.Alternates = append(p.Alternates, views.Alternate{Lang: "x-default", URL: s.SiteURL + xDefault})
	}
	p.MarketNote = current.Note
	return p
}

// kind is what differs between the coffee and the tea sections. A store
// sells one kind (roasters.kind); the pages, rules and tables are shared.
type kind struct {
	name       string // roasters.kind
	segment    string // product pages live at /{market}/{segment}/{slug}
	listPath   string // the listing under the market URL: "" or "tea/"
	nav        string
	icon       string // the category strip's icon
	title      string // listing <title>, %s = market name
	heading    string // listing <h1>, %s = market name
	lede       string // %d = number of products in stock
	desc       string // %d = number of products in stock
	perUnit    func(lang string, priceMinor int64, grams int32, currency string) string
	labels     views.Labels
	categories []category
}

var (
	coffeeKind = kind{
		name: "coffee", segment: "coffee", listPath: "", nav: "Coffee", icon: "cup",
		title: "Specialty coffee prices in the %s", heading: "Coffee prices in the %s",
		lede:    "%d coffees from independent roasters, with the price each one shows today.",
		desc:    "%d coffees from independent roasters with today's price and price history.",
		perUnit: perKilo,
		labels: views.Labels{Back: "All coffees", Maker: "Roasted by", Size: "Bag", Unit: "Per kg", Buy: "Go to roaster",
			StoreNote: "The roaster's own shop"},
		categories: coffeeCategories,
	}
	// Tea is priced per 100 g: tea is bought in 25-250 g packs, and a per-kg
	// figure would read as a price nobody pays.
	teaKind = kind{
		name: "tea", segment: "tea", listPath: "tea/", nav: "Tea", icon: "leaf",
		title: "Specialty tea prices in the %s", heading: "Tea prices in the %s",
		lede:    "%d teas from specialist tea houses, with the price each one shows today.",
		desc:    "%d teas from specialist tea houses with today's price and price history.",
		perUnit: perHundredGrams,
		labels: views.Labels{Back: "All teas", Maker: "Sold by", Size: "Size", Unit: "Per 100 g", Buy: "Go to store",
			StoreNote: "The tea house's own shop"},
		categories: teaCategories,
	}
	kinds = []kind{coffeeKind, teaKind}
)

// nav is the header's menu in a market, as on the sister sites: the
// sections, the stores, the quiz, the calculators and the guides, marking
// the current one by name ("coffee", "tea", "brands", "quiz", "calculators",
// "guides").
func nav(m market.Market, current string) []views.NavLink {
	links := kindLinks(m, current)
	links = append(links,
		views.NavLink{Name: "Brands", URL: "/" + m.Code + "/brands/", Current: current == "brands"},
		views.NavLink{Name: "Which coffee?", URL: "/which-coffee", Current: current == "quiz"},
		views.NavLink{Name: "Calculators", URL: "/calculators/", Current: current == "calculators"},
		views.NavLink{Name: "Guides", URL: "/guides/", Current: current == "guides"})
	return links
}

// kindLinks are a market's sections alone: the coffee/tea switch of a listing.
func kindLinks(m market.Market, current string) []views.NavLink {
	links := make([]views.NavLink, 0, len(kinds))
	for _, k := range kinds {
		links = append(links, views.NavLink{Name: k.nav, URL: "/" + m.Code + "/" + k.listPath, Current: k.name == current})
	}
	return links
}

func (s *Server) addSlash(w http.ResponseWriter, r *http.Request) {
	if _, ok := market.ByCode(r.PathValue("market")); !ok {
		s.notFound(w, r)
		return
	}
	http.Redirect(w, r, "/"+r.PathValue("market")+"/", http.StatusMovedPermanently)
}

// addSectionSlash moves "/en-us/tea" to "/en-us/tea/" (and "/en-us/brands");
// anything else at that depth is not a page.
func (s *Server) addSectionSlash(w http.ResponseWriter, r *http.Request) {
	if _, ok := market.ByCode(r.PathValue("market")); ok {
		if sec := r.PathValue("section"); sec == "brands" || sec == "categories" {
			http.Redirect(w, r, "/"+r.PathValue("market")+"/"+sec+"/", http.StatusMovedPermanently)
			return
		}
		for _, k := range kinds {
			if k.listPath != "" && k.listPath == r.PathValue("section")+"/" {
				http.Redirect(w, r, "/"+r.PathValue("market")+"/"+k.listPath, http.StatusMovedPermanently)
				return
			}
		}
	}
	s.notFound(w, r)
}

func (s *Server) listing(k kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.listPage(w, r, k) }
}

func (s *Server) listPage(w http.ResponseWriter, r *http.Request, k kind) {
	m, ok := market.ByCode(r.PathValue("market"))
	lang := m.Language()
	if !ok {
		s.notFound(w, r)
		return
	}
	base := "/" + m.Code + "/" + k.listPath
	st, ok := s.readList(w, r, m, k, base, 0)
	if !ok {
		return
	}
	cat, _ := k.category(st.cat)
	heading := fmt.Sprintf(k.heading, m.Name)
	if st.cat != "" {
		heading = fmt.Sprintf("%s prices in the %s", cat.noun, m.Name)
	}
	title := heading
	if st.cat == "" {
		title = fmt.Sprintf(k.title, m.Name)
	}
	if st.page > 1 {
		title = fmt.Sprintf("%s, page %d", title, st.page)
	}
	// A category alone is a page of its own, with its own canonical; any
	// other filter, order or search repeats the plain list's products and
	// asks not to be indexed. Each page is its own canonical (not page 1):
	// pointing every page at page 1 would drop the products further on.
	onlyCat := st.cat != "" && st.show == "" && st.roaster == "" && st.sort == "" && st.q == ""
	p := views.Page{
		Title: title + " — CoffeeProof", Lang: m.Lang,
		Description: fmt.Sprintf(k.desc, st.total),
		Canonical:   s.SiteURL + st.pageURL(st.page),
		NoIndex:     st.filtered() && !onlyCat,
		Nav:         nav(m, k.name),
		Query:       st.q,
	}
	// Page 1 of a plain listing (or of a category) has a sibling in every
	// market. Page N does not: the third page of US coffees and of UK coffees
	// list different coffees, so a later page names only itself.
	paths := map[string]string{m.Key: st.pageURL(st.page)}
	xDefault := ""
	if st.page == 1 && (!st.filtered() || onlyCat) {
		for _, other := range market.All {
			paths[other.Key] = "/" + other.Code + "/" + k.listPath
			if onlyCat {
				paths[other.Key] += "?cat=" + st.cat
			}
		}
		if !onlyCat {
			xDefault = "/"
		}
	}
	p = s.inMarkets(p, m, k, paths, xDefault)
	h, err := s.hero(r.Context(), m, k, nil)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	h.Crumbs = []views.NavLink{{Name: "Home", URL: "/"}, {Name: m.Short, URL: "/" + m.Code + "/"}, {Name: k.nav, URL: base}}
	if st.cat != "" {
		h.Crumbs = append(h.Crumbs, views.NavLink{Name: cat.label, URL: base + "?cat=" + cat.slug})
	}
	h.Heading = heading
	h.Lede = fmt.Sprintf(k.lede, st.total)
	h.Currency = "Prices in " + currencyName(lang, m.Currency)
	p.Strip = strip(m, k, st.cat)
	p = chrome(r, p, base)
	s.render(w, r, http.StatusOK, views.MarketHome(p, views.Listing{Hero: h, Deals: st.deals}))
}

// cards turns one page of ListCoffees into listing rows, with one history
// query for all of them, never one per row.
func (s *Server) cards(ctx context.Context, m market.Market, k kind, rows []db.ListCoffeesRow) ([]views.Card, error) {
	lang := m.Language()
	ids := make([]int64, len(rows))
	for i, c := range rows {
		ids[i] = c.OfferID
	}
	hist, err := s.Q.PriceHistory(ctx, ids)
	if err != nil {
		return nil, err
	}
	history := map[int64][]pricePoint{}
	for _, h := range hist {
		history[h.OfferID] = append(history[h.OfferID], pricePoint{At: h.SeenAt.Time, Minor: h.PriceMinor})
	}
	watched, compared := picks(ctx)
	now := time.Now()
	cards := make([]views.Card, 0, len(rows))
	for _, c := range rows {
		card := views.Card{
			URL: "/" + m.Code + "/" + k.segment + "/" + c.Slug, Title: c.Title, Roaster: c.RoasterName,
			RoasterURL: "/" + m.Code + "/brands/" + c.RoasterSlug,
			Size:       c.SizeLabel, Price: money(lang, c.PriceMinor, c.Currency),
			PerUnit:   k.perUnit(lang, c.PriceMinor, c.WeightGrams, c.Currency),
			BuyURL:    fmt.Sprintf("%s/products/%s?variant=%d", c.RoasterUrl, c.Handle, c.VariantID),
			Chart:     sparkArea(history[c.OfferID], now, 180, 48, c.Title+" price history"),
			StoreNote: k.labels.StoreNote,
			Ref:       m.Key + "/" + k.segment + "/" + c.Slug,
		}
		card.Watched, card.Compared = watched[card.Ref], slices.Contains(compared, card.Ref)
		card.WatchURL, card.CompareURL = pickLinks(ctx, card.Ref)
		if c.LastRead.Valid {
			card.Checked = checked(lang, c.LastRead.Time, now)
		}
		if c.Usual > 0 {
			card.Usual = money(lang, c.Usual, c.Currency)
		}
		// Same rules as the coffee page, from the same summary row, so a card
		// and its page can never disagree.
		sum := pricing.Summary{Latest: c.PriceMinor, Low30: c.Low30, Usual: c.Usual,
			TrackedSince: c.TrackedSince.Time, LowestTracked: c.LowestTracked, OwnPrice: c.OwnPrice}
		card.Lower = c.PriceMinor < c.Usual && !sum.RateOnly()
		card.RealDrop = sum.DropPercent()
		if card.Lower && card.RealDrop > 0 {
			card.Saved = money(lang, c.Usual-c.PriceMinor, c.Currency)
		}
		if since, ok := sum.LowestSince(now); ok {
			card.LowestSince = i18n.Day(lang, since)
		} else {
			card.LowestIn30 = sum.LowestIn30Days()
		}
		if c.ImageUrl != nil {
			card.ImageURL = cdnImage(*c.ImageUrl, 240) // a 112 px image well, sharp on 2x screens
		}
		if c.CompareAtMinor != nil {
			card.StoreWas = money(lang, *c.CompareAtMinor, c.Currency)
			if was := *c.CompareAtMinor; was > c.PriceMinor {
				card.StorePercent = int((was - c.PriceMinor) * 100 / was)
			}
		}
		card.Reading = reading(lang, sum, now, c.CompareAtMinor, c.Currency, orEmpty(c.StoreCurrency))
		cards = append(cards, card)
	}
	return cards, nil
}

// reading is the one-sentence verdict of a listing row and of a product
// page, from the same summary, so the two can never disagree. storeCurrency
// is the store's own currency, named when a converted price needs explaining.
// Each case is one whole sentence in the catalog: languages order its parts
// differently, so they are never glued together here.
func reading(lang string, sum pricing.Summary, now time.Time, storeWas *int64, currency, storeCurrency string) string {
	price, usual := money(lang, sum.Latest, currency), money(lang, sum.Usual, currency)
	diff := money(lang, sum.Usual-sum.Latest, currency)
	var s string
	switch {
	// Lower, but only as converted: say why that is no drop.
	case sum.Latest < sum.Usual && sum.RateOnly():
		own := i18n.T(lang, "reading.own_price")
		if storeCurrency != "" {
			own = i18n.T(lang, "reading.own_price_in", "currency", currencyName(lang, storeCurrency))
		}
		if sum.OwnPrice == pricing.OwnNoDrop {
			s = i18n.T(lang, "reading.rate_only.no_drop", "price", price, "diff", diff, "usual", usual, "own", own)
		} else {
			s = i18n.T(lang, "reading.rate_only.unknown", "price", price, "diff", diff, "usual", usual, "own", own)
		}
	// Any price under the usual one, also a drop too small to show as a whole
	// percent: "at its usual price" would be false there.
	case sum.Latest < sum.Usual:
		if since, ok := sum.LowestSince(now); ok {
			s = i18n.T(lang, "reading.under.lowest_since", "price", price, "diff", diff, "usual", usual, "date", i18n.Date(lang, since))
		} else if sum.LowestIn30Days() {
			s = i18n.T(lang, "reading.under.lowest_30", "price", price, "diff", diff, "usual", usual)
		} else {
			s = i18n.T(lang, "reading.under", "price", price, "diff", diff, "usual", usual)
		}
	case sum.Usual == 0:
		if sum.TrackedSince.IsZero() {
			s = i18n.T(lang, "reading.no_history")
		} else {
			s = i18n.T(lang, "reading.no_history.since", "date", i18n.Date(lang, sum.TrackedSince))
		}
	case sum.Latest > sum.Usual:
		s = i18n.T(lang, "reading.above", "price", price, "usual", usual)
	default:
		s = i18n.T(lang, "reading.usual", "usual", usual)
	}
	if storeWas != nil {
		s += " " + i18n.T(lang, "reading.store_sale", "was", money(lang, *storeWas, currency))
	}
	return s
}

// featured is the size a coffee page opens on: the one its listing row
// shows (ListCoffees picks it the same way), so a visitor who clicked a 1 kg
// drop doesn't land on the unchanged 250 g bag. In stock first; then a price
// under the usual one, then a store sale, then the smallest bag.
func featured(offers []views.Offer) int {
	best, rank := 0, -1
	for i, o := range offers {
		if !o.Available {
			continue
		}
		r := 0
		switch {
		case o.Lower:
			r = 2
		case o.StoreWas != "":
			r = 1
		}
		if r > rank { // strict: the smaller bag wins a tie, offers come by weight
			best, rank = i, r
		}
	}
	return best
}

// currencyName is how a listing names its prices' currency; a currency the
// catalog doesn't name shows as its code.
func currencyName(lang, code string) string {
	if !i18n.Has(i18n.Source, "currency."+code) {
		return code
	}
	return i18n.T(lang, "currency."+code)
}

// orEmpty reads a nullable text column.
func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// pageSize is the number of products per listing page.
const pageSize = 48

func (s *Server) product(k kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.productPage(w, r, k) }
}

func (s *Server) productPage(w http.ResponseWriter, r *http.Request, k kind) {
	m, ok := market.ByCode(r.PathValue("market"))
	lang := m.Language()
	if !ok {
		s.notFound(w, r)
		return
	}
	c, err := s.Q.CoffeeBySlug(r.Context(), r.PathValue("slug"))
	if errors.Is(err, pgx.ErrNoRows) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A tea under /coffee/ (or the reverse) is not a page of ours: each
	// product has exactly one address.
	if c.Kind != k.name {
		s.notFound(w, r)
		return
	}
	rows, err := s.Q.OffersForCoffee(r.Context(), db.OffersForCoffeeParams{CoffeeID: c.ID, Market: m.Key})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A coffee without a current offer in this market has no page here:
	// a thin "not sold" page would only dilute the market's pages.
	if len(rows) == 0 {
		s.notFound(w, r)
		return
	}
	// "Price checked" is when we last read this product, not today: a store
	// that failed to load keeps the date of its last reading, as its list rows do.
	var seen time.Time
	for _, o := range rows {
		if o.SeenAt.Valid && o.SeenAt.Time.After(seen) {
			seen = o.SeenAt.Time
		}
	}
	view := views.Coffee{Title: c.Title, Roaster: c.RoasterName, RoasterURL: c.RoasterUrl,
		BackURL: "/" + m.Code + "/" + k.listPath, Labels: k.labels, Section: m.Short + " " + k.name,
		Checked: i18n.Day(lang, seen)}
	if c.ImageUrl != nil {
		view.ImageURL = cdnImage(*c.ImageUrl, 960)
	}
	// One query for the history of every size, not one per row.
	ids := make([]int64, len(rows))
	for i, o := range rows {
		ids[i] = o.ID
	}
	hist, err := s.Q.PriceHistory(r.Context(), ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	history := map[int64][]pricePoint{}
	for _, h := range hist {
		history[h.OfferID] = append(history[h.OfferID], pricePoint{At: h.SeenAt.Time, Minor: h.PriceMinor})
	}
	now := time.Now()
	ld := productLD{Context: "https://schema.org", Type: "Product", Name: c.Title,
		Brand: &thing{Type: "Brand", Name: c.RoasterName}, Image: view.ImageURL}
	for i, o := range rows {
		buy := fmt.Sprintf("%s/products/%s?variant=%d", c.RoasterUrl, c.Handle, o.VariantID)
		ov := views.Offer{
			Size: o.SizeLabel, Price: money(lang, o.PriceMinor, o.Currency), Amount: decimal(o.PriceMinor), Grams: o.WeightGrams,
			PerUnit: k.perUnit(lang, o.PriceMinor, o.WeightGrams, o.Currency), Available: o.Available, BuyURL: buy,
		}
		if o.CompareAtMinor != nil {
			ov.StoreWas = money(lang, *o.CompareAtMinor, o.Currency)
		}
		sum := pricing.Summary{Latest: o.PriceMinor, Low30: o.Low30, Usual: o.Usual,
			TrackedSince: o.TrackedSince.Time, LowestTracked: o.LowestTracked, OwnPrice: o.OwnPrice}
		if o.Usual > 0 {
			ov.Usual = money(lang, o.Usual, o.Currency)
		}
		ov.RealDrop = sum.DropPercent()
		ov.LowestIn30 = sum.LowestIn30Days()
		if since, ok := sum.LowestSince(now); ok {
			ov.LowestSince = i18n.Date(lang, since)
		}
		ov.Anchor = fmt.Sprintf("size-%d", i+1)
		ov.Reading = reading(lang, sum, now, o.CompareAtMinor, o.Currency, orEmpty(c.StoreCurrency))
		ov.Today = i18n.Day(lang, now)
		if o.Low30 > 0 {
			ov.Low30 = money(lang, o.Low30, o.Currency)
		}
		if o.Usual > 0 {
			switch diff := o.PriceMinor - o.Usual; {
			case diff < 0:
				ov.Against = "\u2212" + money(lang, -diff, o.Currency)
				ov.AgainstNote = fmt.Sprintf("%.1f%% lower", float64(-diff)*100/float64(o.Usual))
				switch {
				case !sum.RateOnly():
					ov.Lower = true
				case sum.OwnPrice == pricing.OwnNoDrop:
					ov.AgainstNote += ", exchange rate"
					ov.RateNote = "Exchange rate, not a price cut"
				default:
					ov.AgainstNote += ", not confirmed"
					ov.RateNote = "Can't confirm a price cut"
				}
			case diff > 0:
				ov.Against, ov.AgainstNote = "+"+money(lang, diff, o.Currency),
					fmt.Sprintf("%.1f%% higher", float64(diff)*100/float64(o.Usual))
			default:
				ov.Against, ov.AgainstNote = "Same", "As usual"
			}
		}
		if pts := history[o.ID]; len(pts) > 0 {
			ov.Big = historyChart(lang, pts, now, o.Usual, o.Currency,
				fmt.Sprintf("%s price since %s", o.SizeLabel, i18n.Date(lang, pts[0].At)), ov.Anchor)
			ov.Since = i18n.Date(lang, pts[0].At)
			// Wide and flat, like a roast curve: the page's CSS scales it to
			// the column, the viewBox keeps the proportions.
			ov.Chart = sparkline(pts, now, 640, 96, fmt.Sprintf("%s price since %s", o.SizeLabel, ov.Since))
			hi, lo := pts[0].Minor, pts[0].Minor
			for _, p := range pts {
				hi, lo = max(hi, p.Minor), min(lo, p.Minor)
			}
			ov.High, ov.Low = money(lang, hi, o.Currency), money(lang, lo, o.Currency)
		}
		view.Offers = append(view.Offers, ov)
		availability := "https://schema.org/OutOfStock"
		if o.Available {
			availability = "https://schema.org/InStock"
		}
		ld.Offers = append(ld.Offers, offerLD{Type: "Offer", Price: decimal(o.PriceMinor), PriceCurrency: o.Currency,
			Availability: availability, URL: buy, Seller: &thing{Type: "Organization", Name: c.RoasterName}})
	}
	view.Featured = featured(view.Offers)
	ldJSON, err := json.Marshal(ld) // HTML-escapes <, > and &, safe inside <script>
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// A finished lot keeps its page (bookmarks, price history) but asks not to
	// be indexed once it hasn't been in stock for 30 days (decision K10).
	inStockRecently := false
	for _, o := range rows {
		if o.LastAvailable.Valid && now.Sub(o.LastAvailable.Time) <= 30*24*time.Hour {
			inStockRecently = true
		}
	}
	url := s.SiteURL + "/" + m.Code + "/" + k.segment + "/" + c.Slug
	p := views.Page{
		Title: c.Title + " by " + c.RoasterName + " — price history — CoffeeProof", Lang: m.Lang,
		Description: fmt.Sprintf("%s by %s: today's price for every size and whether it is a real price drop.", c.Title, c.RoasterName),
		Canonical:   url,
		JSONLD:      string(ldJSON),
		NoIndex:     !inStockRecently,
		Nav:         nav(m, k.name),
	}
	// The same coffee's page in the other markets where it is sold now; the
	// US page, when there is one, is the fallback for everyone else.
	codes, err := s.Q.CoffeeMarkets(r.Context(), c.ID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	paths := map[string]string{}
	for _, code := range codes {
		paths[code] = "/" + code + "/" + k.segment + "/" + c.Slug
	}
	p = s.inMarkets(p, m, k, paths, paths[market.US.Key])
	view.Ref = m.Key + "/" + k.segment + "/" + c.Slug
	watched, compared := picks(r.Context())
	view.Watched, view.Compared = watched[view.Ref], slices.Contains(compared, view.Ref)
	view.WatchURL, view.CompareURL = pickLinks(r.Context(), view.Ref)
	view.Kind = k.name
	p = chrome(r, p, "/"+m.Code+"/"+k.listPath)
	s.render(w, r, http.StatusOK, views.CoffeePage(p, view))
}

// Structured data. The store's own rating is never copied in (Google rule;
// the sister site removed it on 6 Oct 2026).
type productLD struct {
	Context string    `json:"@context"`
	Type    string    `json:"@type"`
	Name    string    `json:"name"`
	Image   string    `json:"image,omitempty"`
	Brand   *thing    `json:"brand,omitempty"`
	Offers  []offerLD `json:"offers"`
}

type offerLD struct {
	Type          string `json:"@type"`
	Price         string `json:"price"`
	PriceCurrency string `json:"priceCurrency"`
	Availability  string `json:"availability"`
	URL           string `json:"url"`
	Seller        *thing `json:"seller,omitempty"`
}

type thing struct {
	Type string `json:"@type"`
	Name string `json:"name"`
}

func (s *Server) robots(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	// The theme, watchlist and comparison switches are personal links, not pages.
	fmt.Fprintf(w, "User-agent: *\nDisallow: /internal/\nDisallow: /theme/\nDisallow: /watchlist\nDisallow: /compare\n\nSitemap: %s/sitemap.xml\n", s.SiteURL)
}

func (s *Server) sitemap(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	segment := map[string]string{}
	for _, k := range kinds {
		segment[k.name] = k.segment
	}
	for _, m := range market.All {
		for _, k := range kinds {
			fmt.Fprintf(&b, "<url><loc>%s/%s/%s</loc></url>\n", s.SiteURL, m.Code, k.listPath)
		}
		fmt.Fprintf(&b, "<url><loc>%s/%s/brands/</loc></url>\n", s.SiteURL, m.Code)
		fmt.Fprintf(&b, "<url><loc>%s/%s/categories/</loc></url>\n", s.SiteURL, m.Code)
		// A category alone is a page of its own (see listPage).
		for _, k := range kinds {
			for _, c := range k.categories {
				fmt.Fprintf(&b, "<url><loc>%s/%s/%s?cat=%s</loc></url>\n", s.SiteURL, m.Code, k.listPath, c.slug)
			}
		}
		brands, err := s.Q.ListBrands(r.Context(), m.Key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, br := range brands {
			fmt.Fprintf(&b, "<url><loc>%s/%s/brands/%s</loc></url>\n", s.SiteURL, m.Code, br.Slug)
		}
		rows, err := s.Q.SitemapCoffees(r.Context(), m.Key)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, c := range rows {
			fmt.Fprintf(&b, "<url><loc>%s/%s/%s/%s</loc><lastmod>%s</lastmod></url>\n",
				s.SiteURL, m.Code, segment[c.Kind], c.Slug, c.LastSeen.Time.UTC().Format("2006-01-02"))
		}
	}
	pages := []string{"/guides/", "/guides/coffee-and-health", "/guides/coffee-history", "/calculators/",
		"/which-coffee", "/black-friday", "/cheapest-coffee"}
	for _, c := range calculatorList {
		pages = append(pages, "/calculators/"+c.slug)
	}
	for _, slug := range infoOrder {
		pages = append(pages, "/"+slug)
	}
	for _, g := range pages {
		fmt.Fprintf(&b, "<url><loc>%s%s</loc></url>\n", s.SiteURL, g)
	}
	b.WriteString("</urlset>\n")
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	io.WriteString(w, b.String())
}

// staleAfter is how long a roaster may go unread before the health check
// fails: one daily read plus slack, and well under the 48 hours after which
// its offers drop off the pages, so an alarm comes before coffees vanish.
const staleAfter = 30 * time.Hour

// health answers 503 when any active roaster hasn't been read recently, so
// an uptime monitor notices a silently broken reader.
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Q.RoasterFreshness(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	type source struct {
		Roaster  string     `json:"roaster"`
		LastSeen *time.Time `json:"lastRead"` // the last read without an error
		Stale    bool       `json:"stale"`
	}
	out := struct {
		Stale   int      `json:"stale"`
		Sources []source `json:"sources"`
	}{}
	for _, row := range rows {
		src := source{Roaster: row.Slug, Stale: true}
		if row.LastSeen.Valid {
			t := row.LastSeen.Time.UTC()
			src.LastSeen = &t
			src.Stale = time.Since(t) > staleAfter
		}
		if src.Stale {
			out.Stale++
		}
		out.Sources = append(out.Sources, src)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if out.Stale > 0 {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(out)
}

func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	p := chrome(r, views.Page{Title: "Page not found — CoffeeProof", Lang: "en", NoIndex: true, Nav: nav(market.US, "")}, "/en-us/")
	s.render(w, r, http.StatusNotFound, views.NotFound(p, "/"))
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request failed", "path", r.URL.Path, "err", err)
	http.Error(w, "Something went wrong. Try again in a moment.", http.StatusInternalServerError)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The theme, the watchlist and the comparison live in cookies: a cached
	// page must not be reused for another set of them.
	w.Header().Add("Vary", "Cookie")
	if status == http.StatusOK && w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		s.Log.Error("render failed", "path", r.URL.Path, "err", err)
	}
}

func cacheFor(d time.Duration, h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(d.Seconds())))
		h.ServeHTTP(w, r)
	})
}
