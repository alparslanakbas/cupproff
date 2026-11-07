package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/i18n"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/web/views"
)

// The lists wear the sister sites' controls (4 Nov 2026): a view switch
// (real price drops, store sales, all), a store, a category and an order,
// a search in the header and a category strip. The site has no script, so
// every control is a link or a GET form, and every state has an address.

// category is one entry of the category strip: a section's products whose
// title matches include and not exclude. The stores share no field for it,
// so like the sister sites we read it from the product's name. Counted on
// the US list on 4 Nov 2026: 326 single origins, 104 blends, 71 espressos,
// 63 decafs; teas from 1,284 pu-erhs to 42 chais. "Filter" (11) and "cold
// brew" (6) were too few for a place on the strip.
type category struct {
	slug, icon       string
	include, exclude string // PostgreSQL regular expressions, case-insensitive
}

// origins are the producing countries and regions a single-origin coffee is
// named after.
const origins = `ethiopia|kenya|colombia|brazil|guatemala|costa rica|rwanda|burundi|peru|honduras|el salvador|` +
	`panama|sumatra|indonesia|yemen|mexico|nicaragua|uganda|tanzania|ecuador|bolivia|congo|kona|hawai`

var (
	coffeeCategories = []category{
		{slug: "single-origin", icon: "globe", include: origins, exclude: `\mblends?\M`},
		{slug: "blends", icon: "layers", include: `\mblends?\M`},
		{slug: "espresso", icon: "bolt", include: `espresso`},
		{slug: "decaf", icon: "moon", include: `decaf`},
	}
	teaCategories = []category{
		{slug: "black", icon: "drop",
			include: `black tea|assam|darjeeling|ceylon|earl grey|breakfast|keemun|lapsang|dian ?hong`},
		{slug: "green", icon: "sprout",
			include: `green tea|sencha|gyokuro|genmaicha|hojicha|dragon ?well|long ?jing|gunpowder|bancha|kukicha`, exclude: `matcha`},
		{slug: "oolong", icon: "ring", include: `oolong|tie ?guan ?yin|da ?hong ?pao|dan ?cong`},
		{slug: "white", icon: "asterisk", include: `white tea|silver needle|bai ?mu ?dan|white peony|shou ?mei`},
		{slug: "pu-erh", icon: "disc", include: `pu-?erh|\mpuer\M`},
		{slug: "matcha", icon: "bowl", include: `matcha`},
		{slug: "chai", icon: "star", include: `\mchai\M`},
		{slug: "herbal", icon: "flower", include: `herbal|rooibos|honeybush|chamomile|peppermint|hibiscus|tisane`},
	}
)

func (k kind) category(slug string) (category, bool) {
	for _, c := range k.categories {
		if c.slug == slug {
			return c, true
		}
	}
	return category{}, false
}

// label is the category's name on the strip and in the filters, noun its
// name in a heading ("Decaf coffee" prices), about its line on the
// categories page: the catalog's cat.<slug>.* entries.
func (c category) label(lang string) string { return i18n.T(lang, "cat."+c.slug+".label") }

func (c category) noun(lang string) string { return i18n.T(lang, "cat."+c.slug+".noun") }

func (c category) about(lang string) string { return i18n.T(lang, "cat."+c.slug+".about") }

// sorts are the sister sites' orders; "" is the default, biggest drop.
// unit_asc's label depends on the section (per kg, per 100 g).
var sorts = []struct{ value, label string }{
	{"", "sort.biggest_drop"}, {"newest", "sort.newest"}, {"oldest", "sort.oldest"},
	{"price_asc", "sort.price_asc"}, {"price_desc", "sort.price_desc"}, {"unit_asc", ""},
	{"name_asc", "sort.name_asc"}, {"name_desc", "sort.name_desc"},
}

func validSort(v string) bool {
	return slices.ContainsFunc(sorts, func(s struct{ value, label string }) bool { return s.value == v })
}

// maxQuery bounds what a search may hold (runes, words): a pattern is built
// from it and run against every title.
const (
	maxQuery = 80
	maxWords = 8
)

// searchPattern turns what a visitor typed into a PostgreSQL regular
// expression that needs every word, in any order, in the title or the store
// name ("onyx ethiopia"). Words are quoted, so "." or "(" means itself.
func searchPattern(q string) string {
	var b strings.Builder
	for i, w := range strings.Fields(q) {
		if i == maxWords {
			break
		}
		b.WriteString("(?=.*" + regexp.QuoteMeta(w) + ")")
	}
	return b.String()
}

// listState is one page of a filtered list. A market listing and a store's
// page share it, so their controls, pages and rows work alike.
type listState struct {
	show, cat, roaster, sort, q string
	page                        int
	total                       int64
	cards                       []views.Card
	deals                       views.Deals
	pageURL                     func(page int) string // under the current filters
}

// filtered is true when anything but the page narrows or orders the list:
// such a view asks not to be indexed (it repeats the plain list's products).
func (st listState) filtered() bool {
	return st.show != "" || st.cat != "" || st.roaster != "" || st.sort != "" || st.q != ""
}

// readList reads the list's query (show, cat, roaster, sort, q, page) at base
// and the rows it asks for; storeID pins the list to one store (its page),
// which also drops the store control. Page 1 is the URL without page; a
// ?page=1, a page past the end or an unknown value is not a page of ours,
// so each view has exactly one address. When the request is not a page,
// readList answers it (404 or 500) and returns false.
func (s *Server) readList(w http.ResponseWriter, r *http.Request, m market.Market, k kind, base string, storeID int64) (listState, bool) {
	query := r.URL.Query()
	st := listState{show: query.Get("show"), cat: query.Get("cat"), sort: query.Get("sort"), page: 1}
	if storeID == 0 {
		st.roaster = query.Get("roaster")
	}
	st.q = strings.Join(strings.Fields(query.Get("q")), " ")
	if utf8.RuneCountInString(st.q) > maxQuery {
		st.q = string([]rune(st.q)[:maxQuery])
	}
	_, catOK := k.category(st.cat)
	if (st.show != "" && st.show != "drops" && st.show != "sales") || (st.cat != "" && !catOK) || !validSort(st.sort) {
		s.notFound(w, r)
		return st, false
	}
	if raw := query.Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 2 {
			s.notFound(w, r)
			return st, false
		}
		st.page = n
	}
	// One address per view: the filter form sends its empty fields
	// ("?roaster=&cat=") in its own order, so the visitor is sent on to the
	// view's own address, the one its links and canonical use.
	if want := listQuery(st); r.URL.RawQuery != want {
		target := base
		if want != "" {
			target += "?" + want
		}
		http.Redirect(w, r, target, http.StatusFound)
		return st, false
	}
	st, found, err := s.buildList(r.Context(), m, k, base, st, storeID, pageSize)
	switch {
	case err != nil:
		s.fail(w, r, err)
		return st, false
	case !found:
		s.notFound(w, r)
		return st, false
	}
	return st, true
}

// listQuery is the query string of a list view's own address: the set
// values only, in a fixed order, as buildList's links write them.
func listQuery(st listState) string {
	q := url.Values{}
	for key, v := range map[string]string{"show": st.show, "cat": st.cat, "roaster": st.roaster, "sort": st.sort, "q": st.q} {
		if v != "" {
			q.Set(key, v)
		}
	}
	if st.page > 1 {
		q.Set("page", strconv.Itoa(st.page))
	}
	return q.Encode()
}

// buildList runs a list's queries for st and builds its rows and controls;
// found is false when st names a store or a page the list doesn't have.
func (s *Server) buildList(ctx context.Context, m market.Market, k kind, base string, st listState, storeID int64, size int) (listState, bool, error) {
	lang := m.Language()
	cat, _ := k.category(st.cat)
	// The stores of this section with stock here: the store control's
	// options, and the check that ?roaster= names one of them.
	brands, err := s.Q.ListBrands(ctx, m.Key)
	if err != nil {
		return st, false, err
	}
	var stores []db.ListBrandsRow
	for _, b := range brands {
		if b.Kind == k.name {
			stores = append(stores, b)
		}
	}
	slices.SortFunc(stores, func(a, b db.ListBrandsRow) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	roasterName := ""
	if st.roaster != "" {
		i := slices.IndexFunc(stores, func(b db.ListBrandsRow) bool { return b.Slug == st.roaster })
		if i < 0 {
			return st, false, nil
		}
		storeID, roasterName = stores[i].ID, stores[i].Name
	}
	count := db.CountCoffeesParams{Market: m.Key, Kind: k.name, Show: st.show, Roaster: storeID,
		Title: cat.include, TitleNot: cat.exclude, Search: searchPattern(st.q)}
	if st.total, err = s.Q.CountCoffees(ctx, count); err != nil {
		return st, false, err
	}
	pages := max(1, int((st.total+int64(size)-1)/int64(size)))
	if st.page > pages {
		return st, false, nil
	}
	rows, err := s.Q.ListCoffees(ctx, db.ListCoffeesParams{
		Market: m.Key, Kind: k.name, Show: st.show, PageSize: int32(size), Skip: int32((st.page - 1) * size),
		Roaster: storeID, Title: cat.include, TitleNot: cat.exclude, Search: count.Search, Sort: st.sort,
	})
	if err != nil {
		return st, false, err
	}
	if st.cards, err = s.cards(ctx, m, k, rows); err != nil {
		return st, false, err
	}

	// link is the list's address with some of the current state changed;
	// any change starts again at page 1.
	state := map[string]string{"show": st.show, "cat": st.cat, "roaster": st.roaster, "sort": st.sort, "q": st.q}
	link := func(changes map[string]string, page int) string {
		q := url.Values{}
		for _, key := range []string{"show", "cat", "roaster", "sort", "q"} {
			v := state[key]
			if c, ok := changes[key]; ok {
				v = c
			}
			if v != "" {
				q.Set(key, v)
			}
		}
		if page > 1 {
			q.Set("page", strconv.Itoa(page))
		}
		if len(q) == 0 {
			return base
		}
		return base + "?" + q.Encode()
	}
	st.pageURL = func(n int) string { return link(nil, n) }

	d := views.Deals{Action: base, Show: st.show, Query: st.q, Cards: st.cards, Labels: k.labels(lang), Total: st.total,
		Pager: pager(st.page, pages, int(st.total), st.pageURL)}
	for _, v := range []struct{ show, label string }{{"drops", "view.drops"}, {"sales", "view.sales"}, {"", "view.all"}} {
		d.Views = append(d.Views, views.NavLink{Name: i18n.T(lang, v.label), URL: link(map[string]string{"show": v.show}, 1), Current: v.show == st.show})
	}
	switch st.show {
	case "drops":
		d.ViewNote = i18n.T(lang, "view.note.drops")
	case "sales":
		d.ViewNote = i18n.T(lang, "view.note.sales")
	default:
		d.ViewNote = i18n.T(lang, "view.note.all")
	}
	if storeID == 0 || st.roaster != "" {
		d.Roasters = append(d.Roasters, views.Option{Label: k.text(lang, "all_stores")})
		for _, b := range stores {
			d.Roasters = append(d.Roasters, views.Option{Value: b.Slug, Label: b.Name, Selected: b.Slug == st.roaster})
		}
	}
	d.Categories = append(d.Categories, views.Option{Label: i18n.T(lang, "nav.all_categories")})
	for _, c := range k.categories {
		d.Categories = append(d.Categories, views.Option{Value: c.slug, Label: c.label(lang), Selected: c.slug == st.cat})
	}
	for _, o := range sorts {
		label := i18n.T(lang, o.label)
		if o.value == "unit_asc" {
			label = k.text(lang, "sort_unit")
		}
		d.Sorts = append(d.Sorts, views.Option{Value: o.value, Label: label, Selected: o.value == st.sort})
	}
	if st.roaster != "" {
		d.Chips = append(d.Chips, views.NavLink{Name: roasterName, URL: link(map[string]string{"roaster": ""}, 1)})
	}
	if st.cat != "" {
		d.Chips = append(d.Chips, views.NavLink{Name: cat.label(lang), URL: link(map[string]string{"cat": ""}, 1)})
	}
	if st.q != "" {
		d.Chips = append(d.Chips, views.NavLink{Name: i18n.T(lang, "filter.query", "q", st.q), URL: link(map[string]string{"q": ""}, 1)})
	}
	if st.cat != "" || st.roaster != "" || st.sort != "" || st.q != "" {
		d.ClearURL = link(map[string]string{"cat": "", "roaster": "", "sort": "", "q": ""}, 1)
	}
	// A search with nothing here may have its answer in the other section.
	if st.q != "" && st.total == 0 && storeID == 0 {
		for _, other := range kinds {
			if other.name != k.name {
				d.Elsewhere = &views.NavLink{Name: other.text(lang, "search", "q", st.q),
					URL: "/" + m.Code + "/" + other.listPath + "?q=" + url.QueryEscape(st.q)}
			}
		}
	}
	st.deals = d
	return st, true, nil
}

// pager is the sister sites' page row: the first and last page, the pages
// around this one, and gaps between.
func pager(page, pages, total int, pageURL func(int) string) views.Pager {
	p := views.Pager{Page: page, Pages: pages, Total: total}
	if page > 1 {
		p.PrevURL = pageURL(page - 1)
	}
	if page < pages {
		p.NextURL = pageURL(page + 1)
	}
	last := 0
	for n := 1; n <= pages; n++ {
		if n != 1 && n != pages && (n < page-1 || n > page+1) {
			continue
		}
		if last != 0 && n > last+1 {
			p.Items = append(p.Items, views.PageItem{Gap: true})
		}
		p.Items = append(p.Items, views.PageItem{N: n, URL: pageURL(n), Current: n == page})
		last = n
	}
	return p
}

// strip is the category strip of a market: each section, then its
// categories; current marks the list being shown.
func strip(m market.Market, current kind, cat string) []views.StripItem {
	lang := m.Language()
	var items []views.StripItem
	for _, k := range kinds {
		base := "/" + m.Code + "/" + k.listPath
		items = append(items, views.StripItem{Label: k.text(lang, "nav"), URL: base, Icon: k.icon, Current: k.name == current.name && cat == ""})
		for _, c := range k.categories {
			items = append(items, views.StripItem{Label: c.label(lang), URL: base + "?cat=" + c.slug, Icon: c.icon,
				Current: k.name == current.name && c.slug == cat})
		}
	}
	return items
}

// hero is the band's left column and today's top deal: the section's
// deepest real drop, else the deepest store sale. fallback, when set, is
// tried before the store sales (the home page's tea, on a day without a real
// coffee drop).
func (s *Server) hero(ctx context.Context, m market.Market, k kind, fallback *kind) (views.Hero, error) {
	lang := m.Language()
	var h views.Hero
	total, err := s.Q.CountCoffees(ctx, db.CountCoffeesParams{Market: m.Key, Kind: k.name})
	if err != nil {
		return h, err
	}
	h.Count = total
	h.Tracked = k.count(lang, "tracked", total)
	stores, err := s.Q.RoasterFreshness(ctx)
	if err != nil {
		return h, err
	}
	var last time.Time
	for _, st := range stores {
		if st.LastSeen.Valid && st.LastSeen.Time.After(last) {
			last = st.LastSeen.Time
		}
	}
	if !last.IsZero() {
		h.LastCheck = i18n.DayTime(lang, last) + " UTC"
	}
	tries := []struct {
		k    kind
		show string
	}{{k, "drops"}}
	if fallback != nil {
		tries = append(tries, struct {
			k    kind
			show string
		}{*fallback, "drops"})
	}
	tries = append(tries, struct {
		k    kind
		show string
	}{k, "sales"})
	for _, t := range tries {
		rows, err := s.Q.ListCoffees(ctx, db.ListCoffeesParams{Market: m.Key, Kind: t.k.name, Show: t.show, PageSize: 1})
		if err != nil {
			return h, err
		}
		if len(rows) == 0 {
			continue
		}
		cards, err := s.cards(ctx, m, t.k, rows)
		if err != nil {
			return h, err
		}
		top := cards[0]
		h.Top = &top
		hist, err := s.Q.PriceHistory(ctx, []int64{rows[0].OfferID})
		if err != nil {
			return h, err
		}
		pts := make([]pricePoint, 0, len(hist))
		for _, p := range hist {
			pts = append(pts, pricePoint{At: p.SeenAt.Time, Minor: p.PriceMinor})
		}
		h.TopChart = sparkArea(pts, time.Now(), 280, 90, i18n.T(lang, "chart.label", "title", rows[0].Title))
		h.Labels = t.k.labels(lang)
		break
	}
	return h, nil
}

// The theme control (light, dark, the system's) without a script: a link
// that keeps the choice in a cookie and returns to the page. The layout reads
// it back through the request's context (see withVisitor).
type ctxKey int

const (
	themeKey ctxKey = iota
	pathKey
)

func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	mode := r.PathValue("mode")
	if mode != "light" && mode != "dark" && mode != "system" {
		s.notFound(w, r)
		return
	}
	c := &http.Cookie{Name: "theme", Value: mode, Path: "/", MaxAge: 365 * 24 * 3600,
		SameSite: http.SameSiteLaxMode, HttpOnly: true, Secure: strings.HasPrefix(s.SiteURL, "https://")}
	if mode == "system" {
		c.Value, c.MaxAge = "", -1
	}
	http.SetCookie(w, c)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	http.Redirect(w, r, localPath(r.URL.Query().Get("back")), http.StatusSeeOther)
}

// localPath keeps a redirect on this site: a path of ours, never "//host"
// or "/\host", which browsers read as another site. Only printable ASCII
// without spaces or backslashes passes: browsers drop tabs and newlines from
// a Location, so "/\t/host" would become "//host". A request's own address
// (RequestURI) is already percent-encoded, so it always passes.
func localPath(p string) string {
	if len(p) == 0 || len(p) > 1000 || p[0] != '/' || (len(p) > 1 && (p[1] == '/' || p[1] == '\\')) {
		return "/"
	}
	for i := 0; i < len(p); i++ {
		if c := p[i]; c <= ' ' || c >= 0x7f || c == '\\' {
			return "/"
		}
	}
	return p
}

// themeLinks are the control's three choices for the page at path.
func themeLinks(theme, path string) []views.NavLink {
	back := "?back=" + url.QueryEscape(localPath(path))
	return []views.NavLink{
		{Name: "light", URL: "/theme/light" + back, Current: theme == "light"},
		{Name: "dark", URL: "/theme/dark" + back, Current: theme == "dark"},
		{Name: "system", URL: "/theme/system" + back, Current: theme == ""},
	}
}

// chrome fills in what the layout needs from the request: the theme and
// its control, the watchlist's and the comparison's counts, and where the
// header's search goes.
func chrome(r *http.Request, p views.Page, searchIn string) views.Page {
	theme, _ := r.Context().Value(themeKey).(string)
	path, _ := r.Context().Value(pathKey).(string)
	watched, compared := picks(r.Context())
	p.Theme = theme
	p.Themes = themeLinks(theme, path)
	p.Watching, p.Comparing = len(watched), len(compared)
	p.ClearCompare = "/compare/clear?back=" + url.QueryEscape(localPath(path))
	if p.Search == "" {
		p.Search = searchIn
	}
	if len(p.Languages) == 0 {
		p.Languages = siteLanguages(r.Context(), path)
	}
	return p
}

// pickLinks are a product's watchlist and comparison switches: links that
// return to the page they are on.
func pickLinks(ctx context.Context, ref string) (watch, compare string) {
	path, _ := ctx.Value(pathKey).(string)
	tail := "?item=" + url.QueryEscape(ref) + "&back=" + url.QueryEscape(localPath(path))
	return "/watchlist/toggle" + tail, "/compare/toggle" + tail
}

// checked says when a store's prices were last read, as a row shows it.
func checked(lang string, t time.Time, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	t, now = t.UTC(), now.UTC()
	switch {
	case t.Year() == now.Year() && t.YearDay() == now.YearDay():
		return i18n.T(lang, "checked.today", "time", t.Format("15:04"))
	case t.Year() == now.Year() && t.YearDay() == now.YearDay()-1:
		return i18n.T(lang, "checked.yesterday", "time", t.Format("15:04"))
	}
	return i18n.T(lang, "checked.on", "date", i18n.Day(lang, t))
}
