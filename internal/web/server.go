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
	mux.HandleFunc("GET /{market}/{$}", s.marketHome)
	mux.HandleFunc("GET /{market}/coffee/{slug}", s.coffee)
	// A "GET" pattern also answers HEAD, which uptime monitors send (the
	// sister sites' monitor once stayed red because their endpoint didn't).
	mux.HandleFunc("GET /api/health/sources", s.health)
	mux.HandleFunc("/", s.notFound)
	return mux
}

func (s *Server) home(w http.ResponseWriter, r *http.Request) {
	var links []views.MarketLink
	for _, m := range market.All {
		links = append(links, views.MarketLink{Name: m.Name, URL: "/" + m.Code + "/"})
	}
	p := views.Page{
		Title: "CupProof — coffee price history from independent roasters", Lang: "en",
		Description: "Compare specialty coffee prices from independent roasters and see whether a sale is a real price drop.",
		Canonical:   s.SiteURL + "/",
	}
	s.render(w, r, http.StatusOK, views.Home(p, links))
}

func (s *Server) addSlash(w http.ResponseWriter, r *http.Request) {
	if _, ok := market.ByCode(r.PathValue("market")); !ok {
		s.notFound(w, r)
		return
	}
	http.Redirect(w, r, "/"+r.PathValue("market")+"/", http.StatusMovedPermanently)
}

func (s *Server) marketHome(w http.ResponseWriter, r *http.Request) {
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
	total, err := s.Q.CountCoffees(r.Context(), m.Code)
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
		Market: m.Code, PageSize: pageSize, Skip: int32((page - 1) * pageSize),
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	cards := make([]views.Card, 0, len(rows))
	for _, c := range rows {
		card := views.Card{
			URL: "/" + m.Code + "/coffee/" + c.Slug, Title: c.Title, Roaster: c.RoasterName,
			Size: c.SizeLabel, Price: money(c.PriceMinor, c.Currency),
			PerKilo: perKilo(c.PriceMinor, c.WeightGrams, c.Currency), Available: c.Available,
		}
		if c.ImageUrl != nil {
			card.ImageURL = cdnImage(*c.ImageUrl, 480)
		}
		if c.CompareAtMinor != nil {
			card.StoreWas = money(*c.CompareAtMinor, c.Currency)
		}
		cards = append(cards, card)
	}
	base := "/" + m.Code + "/"
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
	title := "Specialty coffee prices in the " + m.Name + " — CupProof"
	if page > 1 {
		title = fmt.Sprintf("Specialty coffee prices in the %s, page %d — CupProof", m.Name, page)
	}
	// Each page is its own canonical (not page 1): pointing every page at
	// page 1 would tell search engines to drop the coffees listed further on.
	url := s.SiteURL + pageURL(page)
	p := views.Page{
		Title: title, Lang: m.Lang,
		Description: fmt.Sprintf("%d coffees from independent roasters with today's price and price history.", total),
		Canonical:   url, Alternates: []views.Alternate{{Lang: m.Lang, URL: url}, {Lang: "x-default", URL: url}},
	}
	s.render(w, r, http.StatusOK, views.MarketHome(p, "Coffee prices in the "+m.Name, cards, pg))
}

// pageSize is the number of coffees per listing page.
const pageSize = 48

func (s *Server) coffee(w http.ResponseWriter, r *http.Request) {
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
	view := views.Coffee{Title: c.Title, Roaster: c.RoasterName, RoasterURL: c.RoasterUrl, BackURL: "/" + m.Code + "/"}
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
			PerKilo: perKilo(o.PriceMinor, o.WeightGrams, o.Currency), Available: o.Available, BuyURL: buy,
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
			ov.Chart = sparkline(pts, now, 120, 32, fmt.Sprintf("%s price since %s", o.SizeLabel, ov.Since))
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
	url := s.SiteURL + "/" + m.Code + "/coffee/" + c.Slug
	p := views.Page{
		Title: c.Title + " by " + c.RoasterName + " — price history — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf("%s by %s: today's price for every bag size and whether it is a real price drop.", c.Title, c.RoasterName),
		Canonical:   url, Alternates: []views.Alternate{{Lang: m.Lang, URL: url}, {Lang: "x-default", URL: url}},
		JSONLD:  string(ldJSON),
		NoIndex: !inStockRecently,
	}
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
	for _, m := range market.All {
		fmt.Fprintf(&b, "<url><loc>%s/%s/</loc></url>\n", s.SiteURL, m.Code)
		rows, err := s.Q.SitemapCoffees(r.Context(), m.Code)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, c := range rows {
			fmt.Fprintf(&b, "<url><loc>%s/%s/coffee/%s</loc><lastmod>%s</lastmod></url>\n",
				s.SiteURL, m.Code, c.Slug, c.LastSeen.Time.UTC().Format("2006-01-02"))
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
		LastSeen *time.Time `json:"lastSeen"`
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
