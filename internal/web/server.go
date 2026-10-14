// Package web serves the site: server-rendered HTML, no client framework.
package web

import (
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
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
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
	// A "GET" pattern also answers HEAD, which uptime monitors send (the
	// sister sites' monitor once stayed red because their endpoint didn't).
	mux.HandleFunc("GET /api/health/sources", s.health)
	mux.HandleFunc("/", s.notFound)
	return mux
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var links []views.MarketLink
	for _, m := range market.All {
		link := views.MarketLink{Name: m.Name, URL: "/" + m.Code + "/", Note: m.Hint}
		var err error
		if link.Coffees, err = s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Code, Kind: coffeeKind.name}); err != nil {
			s.fail(w, r, err)
			return
		}
		if link.Teas, err = s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Code, Kind: teaKind.name}); err != nil {
			s.fail(w, r, err)
			return
		}
		links = append(links, link)
	}
	stores, err := s.Q.RoasterFreshness(ctx) // one row per active store
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := views.Page{
		Title: "CupProof — coffee and tea price history from independent sellers", Lang: "en",
		Description: "Compare specialty coffee and tea prices from independent roasters and tea houses, and see whether a sale is a real price drop.",
		Canonical:   s.SiteURL + "/",
	}
	// The region chooser is every market's x-default: searchers outside the
	// listed regions land where they can pick one.
	for _, m := range market.All {
		for _, h := range m.Hreflangs {
			p.Alternates = append(p.Alternates, views.Alternate{Lang: h, URL: s.SiteURL + "/" + m.Code + "/"})
		}
	}
	p.Alternates = append(p.Alternates, views.Alternate{Lang: "x-default", URL: s.SiteURL + "/"})
	s.render(w, r, http.StatusOK, views.Home(p, len(stores), links))
}

// inMarkets fills in what every page of a market shares: hreflang
// alternates to the same page in the markets where it exists (paths, by
// market code), region links (to the same page where it exists, otherwise
// to that market's listing of the section) and the market's note. xDefault
// is a path, or "" for none.
func (s *Server) inMarkets(p views.Page, current market.Market, k kind, paths map[string]string, xDefault string) views.Page {
	for _, m := range market.All {
		path, ok := paths[m.Code]
		if ok {
			for _, h := range m.Hreflangs {
				p.Alternates = append(p.Alternates, views.Alternate{Lang: h, URL: s.SiteURL + path})
			}
		} else {
			path = "/" + m.Code + "/" + k.listPath
		}
		p.Regions = append(p.Regions, views.NavLink{Name: m.Short, URL: path, Current: m.Code == current.Code})
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
	name     string // roasters.kind
	segment  string // product pages live at /{market}/{segment}/{slug}
	listPath string // the listing under the market URL: "" or "tea/"
	nav      string
	title    string // listing <title>, %s = market name
	heading  string // listing <h1>, %s = market name
	lede     string // %d = number of products in stock
	desc     string // %d = number of products in stock
	perUnit  func(priceMinor int64, grams int32, currency string) string
	labels   views.Labels
}

var (
	coffeeKind = kind{
		name: "coffee", segment: "coffee", listPath: "", nav: "Coffee",
		title: "Specialty coffee prices in the %s", heading: "Coffee prices in the %s",
		lede:    "%d coffees from independent roasters, with the price each one shows today.",
		desc:    "%d coffees from independent roasters with today's price and price history.",
		perUnit: perKilo,
		labels:  views.Labels{Back: "All coffees", Maker: "Roasted by", Size: "Bag", Unit: "Per kg", Buy: "Go to roaster"},
	}
	// Tea is priced per 100 g: tea is bought in 25-250 g packs, and a per-kg
	// figure would read as a price nobody pays.
	teaKind = kind{
		name: "tea", segment: "tea", listPath: "tea/", nav: "Tea",
		title: "Specialty tea prices in the %s", heading: "Tea prices in the %s",
		lede:    "%d teas from specialist tea houses, with the price each one shows today.",
		desc:    "%d teas from specialist tea houses with today's price and price history.",
		perUnit: perHundredGrams,
		labels:  views.Labels{Back: "All teas", Maker: "Sold by", Size: "Size", Unit: "Per 100 g", Buy: "Go to store"},
	}
	kinds = []kind{coffeeKind, teaKind}
)

// nav lists a market's sections for the header, marking the current one.
func nav(m market.Market, current string) []views.NavLink {
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

// addSectionSlash moves "/en-us/tea" to "/en-us/tea/"; anything else at
// that depth is not a page.
func (s *Server) addSectionSlash(w http.ResponseWriter, r *http.Request) {
	if _, ok := market.ByCode(r.PathValue("market")); ok {
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
	if !ok {
		s.notFound(w, r)
		return
	}
	// Page 1 is the bare market URL; ?page=1 or a page past the end is not a
	// page of ours (404), so each listing page has exactly one address.
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 2 {
			s.notFound(w, r)
			return
		}
		page = n
	}
	total, err := s.Q.CountCoffees(r.Context(), db.CountCoffeesParams{Market: m.Code, Kind: k.name})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	pages := max(1, int((total+pageSize-1)/pageSize))
	if page > pages {
		s.notFound(w, r)
		return
	}
	rows, err := s.Q.ListCoffees(r.Context(), db.ListCoffeesParams{
		Market: m.Code, Kind: k.name, PageSize: pageSize, Skip: int32((page - 1) * pageSize),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	now := time.Now()
	cards := make([]views.Card, 0, len(rows))
	for _, c := range rows {
		card := views.Card{
			URL: "/" + m.Code + "/" + k.segment + "/" + c.Slug, Title: c.Title, Roaster: c.RoasterName,
			Size: c.SizeLabel, Price: money(c.PriceMinor, c.Currency),
			PerUnit: k.perUnit(c.PriceMinor, c.WeightGrams, c.Currency), Available: true, // the listing is in-stock only
		}
		// Same rules as the coffee page, from the same summary row, so a card
		// and its page can never disagree.
		sum := pricing.Summary{Latest: c.PriceMinor, Low30: c.Low30, Usual: c.Usual,
			TrackedSince: c.TrackedSince.Time, LowestTracked: c.LowestTracked}
		card.RealDrop = sum.DropPercent()
		if since, ok := sum.LowestSince(now); ok {
			card.LowestSince = since.UTC().Format("Jan 2, 2006")
		} else {
			card.LowestIn30 = sum.LowestIn30Days()
		}
		if c.ImageUrl != nil {
			card.ImageURL = cdnImage(*c.ImageUrl, 160) // an 80 px thumbnail, sharp on 2x screens
		}
		if c.CompareAtMinor != nil {
			card.StoreWas = money(*c.CompareAtMinor, c.Currency)
		}
		cards = append(cards, card)
	}
	base := "/" + m.Code + "/" + k.listPath
	pageURL := func(n int) string {
		if n == 1 {
			return base
		}
		return fmt.Sprintf("%s?page=%d", base, n)
	}
	pg := views.Pager{Page: page, Pages: pages, Total: int(total)}
	if page > 1 {
		pg.PrevURL = pageURL(page - 1)
	}
	if page < pages {
		pg.NextURL = pageURL(page + 1)
	}
	title := fmt.Sprintf(k.title, m.Name)
	if page > 1 {
		title = fmt.Sprintf("%s, page %d", title, page)
	}
	// Each page is its own canonical (not page 1): pointing every page at
	// page 1 would tell search engines to drop the products listed further on.
	url := s.SiteURL + pageURL(page)
	p := views.Page{
		Title: title + " — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf(k.desc, total),
		Canonical:   url,
		Nav:         nav(m, k.name),
	}
	// Page 1 of a listing has a sibling in every market. Page N does not:
	// the third page of US coffees and of UK coffees list different coffees,
	// so a later page names only itself.
	paths := map[string]string{m.Code: pageURL(page)}
	xDefault := ""
	if page == 1 {
		for _, other := range market.All {
			paths[other.Code] = "/" + other.Code + "/" + k.listPath
		}
		xDefault = "/"
	}
	p = s.inMarkets(p, m, k, paths, xDefault)
	// The rows come ordered by store; each store is one group on the page.
	var groups []views.Group
	for _, c := range cards {
		if len(groups) == 0 || groups[len(groups)-1].Store != c.Roaster {
			groups = append(groups, views.Group{Store: c.Roaster})
		}
		groups[len(groups)-1].Cards = append(groups[len(groups)-1].Cards, c)
	}
	s.render(w, r, http.StatusOK, views.MarketHome(p, fmt.Sprintf(k.heading, m.Name), fmt.Sprintf(k.lede, total), groups, pg))
}

// pageSize is the number of products per listing page.
const pageSize = 48

func (s *Server) product(k kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { s.productPage(w, r, k) }
}

func (s *Server) productPage(w http.ResponseWriter, r *http.Request, k kind) {
	m, ok := market.ByCode(r.PathValue("market"))
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
	rows, err := s.Q.OffersForCoffee(r.Context(), db.OffersForCoffeeParams{CoffeeID: c.ID, Market: m.Code})
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
	view := views.Coffee{Title: c.Title, Roaster: c.RoasterName, RoasterURL: c.RoasterUrl,
		BackURL: "/" + m.Code + "/" + k.listPath, Labels: k.labels}
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
	for _, o := range rows {
		buy := fmt.Sprintf("%s/products/%s?variant=%d", c.RoasterUrl, c.Handle, o.VariantID)
		ov := views.Offer{
			Size: o.SizeLabel, Price: money(o.PriceMinor, o.Currency),
			PerUnit: k.perUnit(o.PriceMinor, o.WeightGrams, o.Currency), Available: o.Available, BuyURL: buy,
		}
		if o.CompareAtMinor != nil {
			ov.StoreWas = money(*o.CompareAtMinor, o.Currency)
		}
		sum := pricing.Summary{Latest: o.PriceMinor, Low30: o.Low30, Usual: o.Usual,
			TrackedSince: o.TrackedSince.Time, LowestTracked: o.LowestTracked}
		if o.Usual > 0 {
			ov.Usual = money(o.Usual, o.Currency)
		}
		ov.RealDrop = sum.DropPercent()
		ov.LowestIn30 = sum.LowestIn30Days()
		if since, ok := sum.LowestSince(now); ok {
			ov.LowestSince = since.UTC().Format("Jan 2, 2006")
		}
		if pts := history[o.ID]; len(pts) > 0 {
			ov.Since = pts[0].At.UTC().Format("Jan 2, 2006")
			// Wide and flat, like a roast curve: the page's CSS scales it to
			// the column, the viewBox keeps the proportions.
			ov.Chart = sparkline(pts, now, 640, 96, fmt.Sprintf("%s price since %s", o.SizeLabel, ov.Since))
			hi, lo := pts[0].Minor, pts[0].Minor
			for _, p := range pts {
				hi, lo = max(hi, p.Minor), min(lo, p.Minor)
			}
			ov.High, ov.Low = money(hi, o.Currency), money(lo, o.Currency)
		}
		view.Offers = append(view.Offers, ov)
		availability := "https://schema.org/OutOfStock"
		if o.Available {
			availability = "https://schema.org/InStock"
		}
		ld.Offers = append(ld.Offers, offerLD{Type: "Offer", Price: decimal(o.PriceMinor), PriceCurrency: o.Currency,
			Availability: availability, URL: buy, Seller: &thing{Type: "Organization", Name: c.RoasterName}})
	}
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
		Title: c.Title + " by " + c.RoasterName + " — price history — CupProof", Lang: m.Lang,
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
	p = s.inMarkets(p, m, k, paths, paths[market.US.Code])
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
	fmt.Fprintf(w, "User-agent: *\nDisallow: /internal/\n\nSitemap: %s/sitemap.xml\n", s.SiteURL)
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
		rows, err := s.Q.SitemapCoffees(r.Context(), m.Code)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, c := range rows {
			fmt.Fprintf(&b, "<url><loc>%s/%s/%s/%s</loc><lastmod>%s</lastmod></url>\n",
				s.SiteURL, m.Code, segment[c.Kind], c.Slug, c.LastSeen.Time.UTC().Format("2006-01-02"))
		}
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
	p := views.Page{Title: "Page not found — CupProof", Lang: "en", NoIndex: true}
	s.render(w, r, http.StatusNotFound, views.NotFound(p, "/"))
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	s.Log.Error("request failed", "path", r.URL.Path, "err", err)
	http.Error(w, "Something went wrong. Try again in a moment.", http.StatusInternalServerError)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if status == http.StatusOK {
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
