// Package web serves the site: server-rendered HTML, no client framework.
package web

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/market"
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
	rows, err := s.Q.ListCoffees(r.Context(), m.Code)
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
	url := s.SiteURL + "/" + m.Code + "/"
	p := views.Page{
		Title: "Specialty coffee prices in the " + m.Name + " — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf("%d coffees from independent roasters with today's price and price history.", len(cards)),
		Canonical:   url, Alternates: []views.Alternate{{Lang: m.Lang, URL: url}, {Lang: "x-default", URL: url}},
	}
	s.render(w, r, http.StatusOK, views.MarketHome(p, "Coffee prices in the "+m.Name, cards))
}

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
		if o.Usual > 0 {
			ov.Usual = money(o.Usual, o.Currency)
			ov.RealDrop = dropPercent(o.PriceMinor, o.Usual)
		}
		if o.Low30 > 0 {
			ov.Low30 = money(o.Low30, o.Currency)
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
	url := s.SiteURL + "/" + m.Code + "/coffee/" + c.Slug
	p := views.Page{
		Title: c.Title + " by " + c.RoasterName + " — price history — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf("%s by %s: today's price for every bag size and whether it is a real price drop.", c.Title, c.RoasterName),
		Canonical:   url, Alternates: []views.Alternate{{Lang: m.Lang, URL: url}, {Lang: "x-default", URL: url}},
		JSONLD: string(ldJSON),
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
