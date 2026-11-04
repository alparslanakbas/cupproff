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
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The stores behind the listings: one index per market, one page per store
// and market. A store's page exists only where it has something in stock
// today, like a coffee's page.

func (s *Server) brandsIndex(w http.ResponseWriter, r *http.Request) {
	m, ok := market.ByCode(r.PathValue("market"))
	if !ok {
		s.notFound(w, r)
		return
	}
	rows, err := s.Q.ListBrands(r.Context(), m.Code)
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
		Title: "Coffee roasters and tea houses in the " + m.Name + " — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf("%d coffee roasters and %d tea houses selling in the %s, with today's prices and how many show a real price drop.",
			len(b.Roasters), len(b.TeaHouses), m.Name),
		Canonical: s.SiteURL + path,
		Nav:       nav(m, "brands"),
	}
	paths := map[string]string{}
	for _, other := range market.All {
		paths[other.Code] = "/" + other.Code + "/brands/"
	}
	p = s.inMarkets(p, m, coffeeKind, paths, "")
	s.render(w, r, http.StatusOK, views.BrandsIndex(p, b))
}

func (s *Server) brandPage(w http.ResponseWriter, r *http.Request) {
	m, ok := market.ByCode(r.PathValue("market"))
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
	brands, err := s.Q.ListBrands(r.Context(), m.Code)
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
	title := fmt.Sprintf("%s %s prices and price history in the %s", store.Name, k.name, m.Name)
	if st.page > 1 {
		title = fmt.Sprintf("%s, page %d", title, st.page)
	}
	p := views.Page{
		Title: title + " — CupProof", Lang: m.Lang,
		Description: fmt.Sprintf("%s in the %s: %s, %s today. Each discount is judged against our own 30-day price record.",
			store.Name, m.Name, views.Amount(me.Coffees, k.name, k.name+"s"), views.Amount(me.Drops, "real price drop", "real price drops")),
		Canonical: s.SiteURL + st.pageURL(st.page),
		NoIndex:   st.show != "",
		Nav:       nav(m, "brands"),
	}
	// Like a listing: page 1 has a sibling in every market where the store
	// has stock today; later pages and filtered views name only themselves.
	paths := map[string]string{m.Code: st.pageURL(st.page)}
	if st.page == 1 && st.show == "" {
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

	unit := "Best price per kg"
	if k.name == teaKind.name {
		unit = "Best price per 100 g"
	}
	v := views.Brand{
		Name: store.Name, Monogram: monogram(store.Name), Tone: tone(store.Slug),
		Market: m.Short, Kind: k.name, Noun: k.name + "s", StoreURL: store.BaseUrl,
		Coffees: me.Coffees, Drops: me.Drops, Sales: me.Sales,
		BestLabel: unit, Best: k.perUnit(me.BestPrice, me.BestGrams, m.Currency),
		Filters: st.filters, Cards: st.cards, Pager: st.pager, Labels: k.labels, Others: others,
		BrandsURL: "/" + m.Code + "/brands/",
	}
	if c := countryName(store.Country); c != "" {
		v.About = append(v.About, views.Fact{Label: "Based in", Value: c})
	}
	v.About = append(v.About, views.Fact{Label: "Prices here", Value: "In " + currencyName(m.Currency) + ", as the store shows them to buyers in the " + m.Short})
	// A store selling in another currency than its own: say so, because its
	// drops here count only when its own price drops too (see own_price).
	if own := orEmpty(store.Currency); own != "" && own != m.Currency {
		v.About = append(v.About, views.Fact{Label: "Its own currency", Value: currencyName(own)})
		v.RateNote = fmt.Sprintf("%s sets its prices in %s; the %s shows here are converted. A lower price here counts as a real drop only when its price in %s dropped too.",
			store.Name, currencyName(own), currencyName(m.Currency), currencyName(own))
	}
	if n := len(store.ShipsTo); n > 0 {
		v.About = append(v.About, views.Fact{Label: "Ships to", Value: views.Amount(int64(n), "country", "countries")})
	}
	v.About = append(v.About, views.Fact{Label: "Price checks", Value: "Every day"})
	s.render(w, r, http.StatusOK, views.BrandPage(p, v))
}

// brandCard is a store as the brands index and "other stores" show it.
func brandCard(m market.Market, row db.ListBrandsRow) views.BrandCard {
	k := kindOf(row.Kind)
	return views.BrandCard{
		URL:  "/" + m.Code + "/brands/" + row.Slug,
		Name: row.Name, Monogram: monogram(row.Name), Tone: tone(row.Slug),
		Country: countryName(row.Country),
		Count:   views.Amount(row.Coffees, k.name, k.name+"s") + " on today's " + m.Short + " list",
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
// none. Only the countries of the stores we read; another code shows as is.
func countryName(code *string) string {
	if code == nil {
		return ""
	}
	names := map[string]string{
		"US": "the United States", "GB": "the United Kingdom", "DE": "Germany", "ES": "Spain", "SE": "Sweden",
		"DK": "Denmark", "FR": "France", "NO": "Norway", "FI": "Finland", "IE": "Ireland", "IT": "Italy",
		"NL": "the Netherlands", "AU": "Australia", "CA": "Canada", "NZ": "New Zealand", "BE": "Belgium",
		"AT": "Austria", "CH": "Switzerland", "PT": "Portugal",
	}
	c := strings.ToUpper(strings.TrimSpace(*code))
	if n, ok := names[c]; ok {
		return n
	}
	return c
}
