package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/pricing"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// A watchlist and a comparison without accounts or a script, as the sister
// sites offer them (they keep both in the browser): two cookies of product
// references ("en-us/coffee/onyx-geometry"), changed by links that return to
// the page. Nothing but these references is stored, and only in the
// visitor's browser.
const (
	watchCookie   = "watch"
	compareCookie = "compare"
	maxWatch      = 40
	maxCompare    = 4
)

// refPattern is a product reference: market, section, slug.
var refPattern = regexp.MustCompile(`^[a-z]{2}-[a-z]{2}/(coffee|tea)/[a-z0-9-]{1,120}$`)

const (
	watchKey ctxKey = iota + 10
	compareKey
)

// readRefs reads a cookie of references: the valid ones, once each, at most
// limit. "." separates them (a comma isn't allowed in a cookie value).
func readRefs(r *http.Request, name string, limit int) []string {
	c, err := r.Cookie(name)
	if err != nil {
		return nil
	}
	var refs []string
	for _, ref := range strings.Split(c.Value, ".") {
		if refPattern.MatchString(ref) && !slices.Contains(refs, ref) {
			refs = append(refs, ref)
		}
		if len(refs) == limit {
			break
		}
	}
	return refs
}

func writeRefs(w http.ResponseWriter, s *Server, name string, refs []string) {
	c := &http.Cookie{Name: name, Value: strings.Join(refs, "."), Path: "/", MaxAge: 365 * 24 * 3600,
		SameSite: http.SameSiteLaxMode, HttpOnly: true, Secure: strings.HasPrefix(s.SiteURL, "https://")}
	if len(refs) == 0 {
		c.Value, c.MaxAge = "", -1
	}
	http.SetCookie(w, c)
}

// picks are the visitor's watched (a set) and compared (in order) products.
func picks(ctx context.Context) (map[string]bool, []string) {
	watched, _ := ctx.Value(watchKey).(map[string]bool)
	compared, _ := ctx.Value(compareKey).([]string)
	return watched, compared
}

// withVisitor puts what the visitor chose in the request's context: the
// theme, the watchlist and the comparison, and the page's own address (the
// links that change them return there).
func (s *Server) withVisitor(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		theme := ""
		if c, err := r.Cookie("theme"); err == nil && (c.Value == "light" || c.Value == "dark") {
			theme = c.Value
		}
		watched := map[string]bool{}
		for _, ref := range readRefs(r, watchCookie, maxWatch) {
			watched[ref] = true
		}
		ctx := context.WithValue(r.Context(), themeKey, theme)
		ctx = context.WithValue(ctx, pathKey, r.URL.RequestURI())
		ctx = context.WithValue(ctx, watchKey, watched)
		ctx = context.WithValue(ctx, compareKey, readRefs(r, compareCookie, maxCompare))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// toggle adds a reference to a cookie's list or takes it out, and returns
// to the page. A full list takes no more (the sister sites disable the
// button); the page says so.
func (s *Server) toggle(name string, limit int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ref := r.URL.Query().Get("item")
		refs := readRefs(r, name, limit)
		switch i := slices.Index(refs, ref); {
		case !refPattern.MatchString(ref):
			s.notFound(w, r)
			return
		case i >= 0:
			refs = slices.Delete(refs, i, i+1)
		case len(refs) < limit:
			refs = append(refs, ref)
		}
		writeRefs(w, s, name, refs)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		http.Redirect(w, r, localPath(r.URL.Query().Get("back")), http.StatusSeeOther)
	}
}

func (s *Server) clearCompare(w http.ResponseWriter, r *http.Request) {
	writeRefs(w, s, compareCookie, nil)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, localPath(r.URL.Query().Get("back")), http.StatusSeeOther)
}

// cardFor is today's card of a referenced product: the size its page opens
// on, with the same rules as a listing row. ok is false when the product is
// gone or no longer sold in that market.
func (s *Server) cardFor(ctx context.Context, ref string) (views.Card, bool, error) {
	parts := strings.Split(ref, "/")
	m, ok := market.ByKey(parts[0])
	lang := m.Language()
	if !ok {
		return views.Card{}, false, nil
	}
	k := coffeeKind
	if parts[1] == teaKind.segment {
		k = teaKind
	}
	c, err := s.Q.CoffeeBySlug(ctx, parts[2])
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && c.Kind != k.name) {
		return views.Card{}, false, nil
	}
	if err != nil {
		return views.Card{}, false, err
	}
	rows, err := s.Q.OffersForCoffee(ctx, db.OffersForCoffeeParams{CoffeeID: c.ID, Market: m.Key})
	if err != nil || len(rows) == 0 {
		return views.Card{}, false, err
	}
	// The size the page opens on (see featured): in stock, then a price under
	// the usual one, then a store sale, then the smallest bag.
	best, rank := 0, -1
	for i, o := range rows {
		sum := pricing.Summary{Latest: o.PriceMinor, Usual: o.Usual, OwnPrice: o.OwnPrice}
		r := 0
		switch {
		case !o.Available:
			r = -1
		case o.PriceMinor < o.Usual && !sum.RateOnly():
			r = 2
		case o.CompareAtMinor != nil:
			r = 1
		}
		if r > rank {
			best, rank = i, r
		}
	}
	o := rows[best]
	now := time.Now()
	card := views.Card{
		URL: "/" + m.Code + "/" + k.segment + "/" + c.Slug, Title: c.Title, Roaster: c.RoasterName,
		Size: o.SizeLabel, Price: money(lang, o.PriceMinor, o.Currency), PerUnit: k.perUnit(lang, o.PriceMinor, o.WeightGrams, o.Currency),
		BuyURL:    fmt.Sprintf("%s/products/%s?variant=%d", c.RoasterUrl, c.Handle, o.VariantID),
		StoreNote: k.labels.StoreNote, Ref: ref, Market: m.Short, OutOfStock: !o.Available,
	}
	watched, compared := picks(ctx)
	card.Watched, card.Compared = watched[ref], slices.Contains(compared, ref)
	card.WatchURL, card.CompareURL = pickLinks(ctx, ref)
	if c.ImageUrl != nil {
		card.ImageURL = cdnImage(*c.ImageUrl, 240)
	}
	sum := pricing.Summary{Latest: o.PriceMinor, Low30: o.Low30, Usual: o.Usual,
		TrackedSince: o.TrackedSince.Time, LowestTracked: o.LowestTracked, OwnPrice: o.OwnPrice}
	if o.Usual > 0 {
		card.Usual = money(lang, o.Usual, o.Currency)
	}
	if o.Low30 > 0 {
		card.Low30 = money(lang, o.Low30, o.Currency)
	}
	card.Lower = o.PriceMinor < o.Usual && !sum.RateOnly()
	card.RealDrop = sum.DropPercent()
	if card.Lower && card.RealDrop > 0 {
		card.Saved = money(lang, o.Usual-o.PriceMinor, o.Currency)
	}
	if since, ok := sum.LowestSince(now); ok {
		card.LowestSince = i18n.Day(lang, since)
	} else {
		card.LowestIn30 = sum.LowestIn30Days()
	}
	if o.CompareAtMinor != nil {
		card.StoreWas = money(lang, *o.CompareAtMinor, o.Currency)
		if was := *o.CompareAtMinor; was > o.PriceMinor {
			card.StorePercent = int((was - o.PriceMinor) * 100 / was)
		}
	}
	if o.SeenAt.Valid {
		card.Checked = checked(lang, o.SeenAt.Time, now)
	}
	card.Reading = reading(lang, sum, now, o.CompareAtMinor, o.Currency, orEmpty(c.StoreCurrency))
	return card, true, nil
}

// cardsFor is cardFor for a list, keeping its order and skipping what is gone.
func (s *Server) cardsFor(ctx context.Context, refs []string) ([]views.Card, error) {
	var cards []views.Card
	for _, ref := range refs {
		c, ok, err := s.cardFor(ctx, ref)
		if err != nil {
			return nil, err
		}
		if ok {
			cards = append(cards, c)
		}
	}
	return cards, nil
}

func (s *Server) watchlist(w http.ResponseWriter, r *http.Request) {
	watched, _ := picks(r.Context())
	refs := readRefs(r, watchCookie, maxWatch) // in the order they were added
	cards, err := s.cardsFor(r.Context(), refs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := views.Page{Title: "Your watchlist — CoffeeProof", Lang: "en", NoIndex: true, Nav: nav(market.US, "")}
	p = chrome(r, p, "/"+market.US.Code+"/")
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, r, http.StatusOK, views.Watchlist(p, cards, len(watched)))
}

func (s *Server) compare(w http.ResponseWriter, r *http.Request) {
	_, refs := picks(r.Context())
	cards, err := s.cardsFor(r.Context(), refs)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	p := views.Page{Title: "Compare — CoffeeProof", Lang: "en", NoIndex: true, Nav: nav(market.US, "")}
	p = chrome(r, p, "/"+market.US.Code+"/")
	p.Comparing = 0 // this page is the comparison: no bar pointing to it
	w.Header().Set("Cache-Control", "no-store")
	s.render(w, r, http.StatusOK, views.Compare(p, cards, "/compare/clear?back="+url.QueryEscape("/compare")))
}
