package web

import (
	"errors"
	"fmt"
	"hash/fnv"
	"net/http"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The stores behind the listings: one index per market, one page per store
// and market. A store's page exists only where it has something in stock
// today, like a coffee's page.

func (s *Server) brandsIndex(w http.ResponseWriter, r *http.Request) {
	m, ok := market.ByCode(r.PathValue("market"))
	lang := m.Language()
	if !ok {
		s.notFound(w, r)
		return
	}
	rows, err := s.Q.ListBrands(r.Context(), m.Key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	b := views.Brands{Market: m.Short}
	for _, row := range rows {
		if row.Kind == teaKind.name {
			b.TeaHouses = append(b.TeaHouses, brandCard(m, row))
		} else {
			b.Roasters = append(b.Roasters, brandCard(m, row))
		}
	}
	path := "/" + m.Code + "/brands/"
	p := views.Page{
		Title: i18n.T(lang, "brands.title", "in_market", inMarket(lang, m)), Lang: m.Lang,
		Description: i18n.T(lang, "brands.desc", "roasters", fmt.Sprint(len(b.Roasters)), "houses", fmt.Sprint(len(b.TeaHouses)),
			"in_market", inMarket(lang, m)),
		Canonical: s.SiteURL + path,
		Nav:       nav(m, "brands"),
	}
	paths := map[string]string{}
	for _, other := range market.All {
		paths[other.Key] = "/" + other.Code + "/brands/"
	}
	p = s.inMarkets(p, m, coffeeKind, paths, "")
	p = chrome(r, p, "/"+m.Code+"/")
	s.render(w, r, http.StatusOK, views.BrandsIndex(p, b))
}

func (s *Server) brandPage(w http.ResponseWriter, r *http.Request) {
	m, ok := market.ByCode(r.PathValue("market"))
	lang := m.Language()
	if !ok {
		s.notFound(w, r)
		return
	}
	store, err := s.Q.BrandBySlug(r.Context(), r.PathValue("slug"))
	if errors.Is(err, pgx.ErrNoRows) {
		s.notFound(w, r)
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	k := kindOf(store.Kind)
	brands, err := s.Q.ListBrands(r.Context(), m.Key)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var me *db.ListBrandsRow
	var others []views.BrandCard
	for i, row := range brands {
		switch {
		case row.ID == store.ID:
			me = &brands[i]
		case row.Kind == store.Kind && len(others) < 3: // the list is most drops first
			others = append(others, brandCard(m, row))
		}
	}
	if me == nil { // nothing in stock in this market today
		s.notFound(w, r)
		return
	}
	base := "/" + m.Code + "/brands/" + store.Slug
	st, ok := s.readList(w, r, m, k, base, store.ID)
	if !ok {
		return
	}
	title := k.text(lang, "brand_title", "store", store.Name, "in_market", inMarket(lang, m))
	if st.page > 1 {
		title = i18n.T(lang, "listing.page_title", "title", title, "n", fmt.Sprint(st.page))
	}
	p := views.Page{
		Title: title + " — CoffeeProof", Lang: m.Lang,
		Description: i18n.T(lang, "brand.desc", "store", store.Name, "in_market", inMarket(lang, m),
			"products", k.count(lang, "count", me.Coffees), "drops", i18n.N(lang, "count.real_price_drops", me.Drops)),
		Canonical: s.SiteURL + st.pageURL(st.page),
		NoIndex:   st.filtered(),
		Nav:       nav(m, "brands"),
	}
	// Like a listing: page 1 has a sibling in every market where the store
	// has stock today; later pages and filtered views name only themselves.
	paths := map[string]string{m.Key: st.pageURL(st.page)}
	if st.page == 1 && !st.filtered() {
		markets, err := s.Q.BrandMarkets(r.Context(), store.ID)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		for _, code := range markets {
			paths[code] = "/" + code + "/brands/" + store.Slug
		}
	}
	p = s.inMarkets(p, m, k, paths, "")

	unit := k.text(lang, "best_unit")
	v := views.Brand{
		Name: store.Name, Monogram: monogram(store.Name), Tone: tone(store.Slug),
		Market: m.Short, Kind: k.name, Noun: k.name + "s", StoreURL: store.BaseUrl,
		Coffees: me.Coffees, Drops: me.Drops, Sales: me.Sales,
		BestLabel: unit, Best: k.perUnit(lang, me.BestPrice, me.BestGrams, m.Currency),
		Deals: st.deals, Labels: k.labels(lang), Others: others,
		BrandsURL: "/" + m.Code + "/brands/",
	}
	if c := countryName(lang, store.Country); c != "" {
		v.About = append(v.About, views.Fact{Label: i18n.T(lang, "brand.based_in"), Value: c})
	}
	v.About = append(v.About, views.Fact{Label: i18n.T(lang, "brand.prices_here"), Value: i18n.T(lang, "brand.prices_here_value", "currency", currencyName(lang, m.Currency), "market", m.Short)})
	// A store selling in another currency than its own: say so, because its
	// drops here count only when its own price drops too (see own_price).
	if own := orEmpty(store.Currency); own != "" && own != m.Currency {
		v.About = append(v.About, views.Fact{Label: i18n.T(lang, "brand.own_currency"), Value: currencyName(lang, own)})
		v.RateNote = i18n.T(lang, "brand.rate_note", "store", store.Name, "own", currencyName(lang, own),
			"currency", currencyName(lang, m.Currency))
	}
	if n := len(store.ShipsTo); n > 0 {
		v.About = append(v.About, views.Fact{Label: i18n.T(lang, "brand.ships_to"), Value: i18n.N(lang, "count.countries", int64(n))})
	}
	v.About = append(v.About, views.Fact{Label: i18n.T(lang, "brand.price_checks"), Value: i18n.T(lang, "brand.every_day")})
	p.Query = st.q
	p = chrome(r, p, "/"+m.Code+"/"+k.listPath)
	s.render(w, r, http.StatusOK, views.BrandPage(p, v))
}

// brandCard is a store as the brands index and "other stores" show it.
func brandCard(m market.Market, row db.ListBrandsRow) views.BrandCard {
	k, lang := kindOf(row.Kind), m.Language()
	return views.BrandCard{
		URL:  "/" + m.Code + "/brands/" + row.Slug,
		Name: row.Name, Monogram: monogram(row.Name), Tone: tone(row.Slug),
		Country: countryName(lang, row.Country),
		Count:   k.count(lang, "card_count", row.Coffees, "market", m.Short),
		Drops:   row.Drops, Sales: row.Sales,
	}
}

func kindOf(name string) kind {
	for _, k := range kinds {
		if k.name == name {
			return k
		}
	}
	return coffeeKind
}

// monogram is the two letters of a store's badge: the start of its name,
// like the design's "OZ" for Ozone.
func monogram(name string) string {
	var out []rune
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out = append(out, unicode.ToUpper(r))
			if len(out) == 2 {
				break
			}
		} else if len(out) > 0 {
			break // one word only: "Bird & Blend" is "BI", not "BB"
		}
	}
	return string(out)
}

// tone picks one of the badge colours from the slug, so a store keeps its
// colour from page to page.
func tone(slug string) int {
	h := fnv.New32a()
	h.Write([]byte(slug))
	return int(h.Sum32()%6) + 1
}

// countryName names the country a store's /meta.json gives, "" when it gave
// none. The catalog names the countries of the stores we read (country.*);
// another code shows as is.
func countryName(lang string, code *string) string {
	if code == nil {
		return ""
	}
	c := strings.ToUpper(strings.TrimSpace(*code))
	if !i18n.Has(i18n.Source, "country."+c) {
		return c
	}
	return i18n.T(lang, "country."+c)
}
