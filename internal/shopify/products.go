// Package shopify reads coffee and tea offers from Shopify storefronts.
package shopify

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// Product is the subset of a /products.json entry we use.
type Product struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Handle      string    `json:"handle"`
	ProductType string    `json:"product_type"`
	Tags        []string  `json:"tags"`
	Variants    []Variant `json:"variants"`
	Images      []Image   `json:"images"`
}

// Variant is one purchasable option of a product.
type Variant struct {
	ID             int64   `json:"id"`
	Title          string  `json:"title"`
	Option1        *string `json:"option1"`
	Option2        *string `json:"option2"`
	Option3        *string `json:"option3"`
	Price          string  `json:"price"`
	CompareAtPrice *string `json:"compare_at_price"`
	Available      bool    `json:"available"`
}

// Image is a product image.
type Image struct {
	Src string `json:"src"`
}

// Offer is one bag size of a coffee: grind and other options that don't
// change the size fold into it, at the cheapest available price.
type Offer struct {
	ProductID      int64
	Handle         string
	Title          string
	ProductType    string
	ImageURL       string
	SizeLabel      string
	WeightGrams    int
	VariantID      int64
	PriceMinor     int64
	CompareAtMinor int64 // 0 when the store states no previous price
	Available      bool
}

// Not coffee beans even when a size appears in the text ("12 oz mug").
// "cup" and "filter" are deliberately absent: "Cup of Excellence" lots and
// "Filter Roast" coffees are among the coffees we most want to keep.
// Wholesale listings ("… (WS)", Madcap) are trade prices, not what a
// customer pays, so they are left out too.
// Added after the tea round (13 Oct 2026, seen in the coffee listing): SEY's
// "Monthly"/"Recurring - 12 Installments" subscriptions, boxes and packs of
// several coffees (a per-kg price says nothing about any one of them, and
// La Colombe's "3oz Filter Packs" priced a case as one 3 oz pack), and
// paper/travel cups ("hot cup", "cold cup"; a bare "cup" stays allowed).
// Community Coffee lists creamer and sugar with no type or tag at all;
// "sugar" alone can't be used ("Grandma's Sugar Cookie" is a coffee).
// UK round (13 Oct 2026): matcha typed "Coffee" (Grind's refill pouches),
// KeepCup/Huskee cups, Airscape canisters, milk jugs and pitchers.
var notCoffee = regexp.MustCompile(`(?i)\b(mugs?|tumblers?|bottles?|carafes?|kettles?|grinders?|drippers?|scales?|candles?|` +
	`t-shirts?|shirts?|hats?|totes?|hoodies?|gift ?cards?|subscriptions?|merch(andise)?|equipment|apparel|accessor(y|ies)|unroasted|` +
	`wholesale|recurring|installments?|monthly|(gift|discovery|experience) box(es)?|packs?|(travel|hot|cold|paper|coffee|camp) cups?|` +
	`creamers?|granulated|matcha|keepcup|huskee|airscape|jugs?|pitchers?|` +
	// EU round (14 Oct 2026; stores write in German, French, Spanish, Finnish):
	// subscriptions ("Abo 12 Monate", Finnish "vakiotilaus"), a public cupping
	// event, gear sold by brand or by French name (moulin = grinder), VST
	// filter baskets, canisters, bundles and drip bags.
	// "fellow" is deliberately absent: Methodical sells real coffees named for
	// the brand ("Fellow Berry Spice"); Fellow's Atmos canister is caught by "atmos".
	`abo|abonnement|vakiotilaus|cupping|moulin|comandante|delonghi|moccamaster|aarke|atmos|canisters?|` +
	`vst|baskets?|bundles?|drip ?bags?)\b|\(ws\)`)

// Deliberately absent: "variety" (Counter Culture names lots by plant
// variety, "Finca El Puente - Gesha Variety"; 13 real coffees were lost to
// it on 13 Oct 2026, a "Variety Pack" is caught by "packs") and a bare
// "box" (Onyx's "Colombia La Riviera Gesha Box" is one coffee).

// A coffee size that states one unit of a multi-unit pack: Community
// Coffee's "40 Count - 2.5 oz. Packet" ($84.57) and "12 oz. | 8-pack"
// read as 2.5 oz and 12 oz, a per-kg price 8 to 40 times too high
// (13 Oct 2026). Pods carry no comparable bean weight either.
var coffeePortion = regexp.MustCompile(`(?i)\b(count|ct|\d+[- ]?packs?|pack of|packets?|pods?|k-?cups?)\b`)

// The store's own product type is the stronger signal when it is set: on
// 13 Oct 2026 "Drinkware" cups ("Latte Cup 12oz"), "Gifts" (honey 12oz) and
// "Green" (unroasted beans, not comparable with roasted coffee) slipped past
// the title check because their text states a size.
// Added on the second roaster round (13 Oct 2026, product types seen in the
// stores' own catalogs): tea (Passenger sells tea), "Goods", "Sundry Retail",
// "RTD" (ready-to-drink cans); brew equipment is caught by "equipment". "retail" and
// "brew" are left out on purpose: "Retail Coffee" and "Cold Brew" blends are coffee.
// After the tea round: Onyx "Warehouse" (38 glasses, cups, containers),
// "Barista Provisions" (latte powders), "Box Set"; Madcap "Supplies" (cups)
// and "Alt Beverage" (cascara); George Howell "Cleaning"; Peace "Bundle".
// UK round (13 Oct 2026, the roasters' own types): hot chocolate,
// chocolate, "Drink"/"other-drinks" (Ozone sells Good & Proper teas),
// matcha, chai, cascara, "Ceramics", "Reusable Cups", "Storage", "Tech
// Parts", "Ancillary", "Kit"/"espresso kit", "Box", "coffee plans",
// "wholesale coffee", and the cup and powder brands MiiR and Blendsmiths.
var notCoffeeType = regexp.MustCompile(`(?i)\b(drinkware|gifts?|green|unroasted|merch(andise)?|apparel|equipment|` +
	`accessor(y|ies)|brewing|gear|subscriptions?|gift ?cards?|teas?|goods|sundry|rtd|ready[ -]to[ -]drink|` +
	`warehouse|supplies|provisions|box ?sets?|bundles?|cleaning|alt beverage|` +
	`wholesale|chocolate|drinks?|matcha|chai|cascara|ceramics|cups?|storage|parts|ancillary|kits?|box|plans?|miir|blendsmiths|` +
	// EU round (14 Oct 2026): "Coffee Club" (subscription), "Training", "Hats",
	// French "Matériel", "Barista Tools", "Merchandising", Spanish
	// "Suscripción" and "Limpiador" (cleaner), The Barn's "Misc".
	`club|training|hats?|caps?|mat[ée]riel|tools?|merchandising|suscripci[oó]n|limpiador|misc)\b`)

// White-label and wholesale-only products carry trade prices, not a price a
// customer pays: Onyx publishes dozens tagged "white-label" with code names
// ("IO-E-05", $4.58 for 8.8 oz), Madcap tags its trade bags "wholesale-only".
// A bare "wholesale" tag is NOT a signal: Heart puts it on its retail single
// origins (8 oz for $23) to mean "also sold wholesale" (13 Oct 2026).
var notConsumerTag = regexp.MustCompile(`(?i)^(white[- ]?label|wholesale[- ]?only|private[- ]?label)$`)

// Kind is what a store sells. Every product of a store is read as the
// store's kind: a roaster's few teas are skipped (notCoffeeType), and a tea
// house's coffee, if any, would be too.
type Kind string

const (
	Coffee Kind = "coffee"
	Tea    Kind = "tea"
)

// Tea houses sell far more than tea: on 13 Oct 2026 Song Tea listed 211
// teaware items among its first 250 products, Bellocq candles, incense and
// "Bath & Body", Yunnan Sourcing pottery, In Pursuit of Tea a "Wholesale Tea"
// type. Variety packs and samplers mix teas, so a per-100 g price says
// nothing about any one of them. Harney leaves most product types empty
// (202 of 250), so the title check carries more weight here.
var notTeaType = regexp.MustCompile(`(?i)\b(tea ?wares?|wares|utensils|pottery|ceramics|porcelain|teapots?|cups?|bowls?|mugs?|` +
	`table|home|candles?|incense|bath|body|accessor(y|ies)|gifts?|gift ?sets?|gift ?cards?|bundles?|variety packs?|assorted|` +
	`merch(andise)?|apparel|subscriptions?|membership|club|events?|snacks|sparkling|concentrates?|water|wholesale|equipment|` +
	`honey|jam|coffee|hidden|collections?|tickets?|paper)\b`)

// UK round (13 Oct 2026): Tregothnan sells honey and jam by weight, and
// Tregothnan and Waterloo Tea list coffee; a "Hidden" type is a product the
// store keeps off its own shelves.

var notTea = regexp.MustCompile(`(?i)\b(teapots?|kettles?|infusers?|strainers?|mugs?|cups?|bowls?|whisks?|scoops?|` +
	`candles?|incense|t-shirts?|shirts?|hats?|totes?|hoodies?|gift ?cards?|sets?|kits?|samplers?|variety|assortment|` +
	`flights?|subscriptions?|membership|chocolates?|cookies?|shortbread|biscuits?|wholesale|green coffee)\b`)

// "honey" is deliberately absent: "Phoenix Honey Orchid" is an oolong.

// A tea size that is not comparable per 100 g: a one-cup sample ("3g Sample
// (1 Serving)", "Single brew | 5g", Rishi's "Teaser") or a box of bags whose
// text gives one bag's weight ("10 3g Teabags" would read as 3 g). These
// variants are skipped; the loose sizes of the same tea stay.
var teaPortion = regexp.MustCompile(`(?i)\b(samples?|single brew|servings?|teaser|trial|teabags?|tea bags?|sachets?|pyramids?|ct|count|enveloped)\b`)

// Store is what ToOffers needs to know about the store a catalog came from.
type Store struct {
	Kind Kind
	// VariantMustMatch, when set, keeps only variants whose text matches it.
	// It is for a store whose catalog mixes its kind with goods no title or
	// type tells apart: Harney leaves 202 of 250 product types empty and sells
	// coconut water, jerky and coffee next to its teas, but every tea size is
	// written "Loose / 2 oz. Tin" (84 of 84 on 13 Oct 2026, none of the 38
	// other weighted products).
	VariantMustMatch *regexp.Regexp
}

// Harney sells ground coffee and rock sugar as "Loose / 12 oz. Bag", so the
// Loose rule alone lets them in; its own "pantry", "Bottles" and "treats"
// tags mark them (13 Oct 2026). "Speyside Whisky" is a tea, tagged
// "All Loose Tea", so the title is no help there either.
var notTeaTag = regexp.MustCompile(`(?i)^(pantry|bottles|treats)$`)

// filter picks a store's products of its kind.
type filter struct {
	title, productType *regexp.Regexp
	tag                *regexp.Regexp // nil when the kind has no tag rule
	portion            *regexp.Regexp // sizes that are not comparable; nil when none
}

var filters = map[Kind]filter{
	Coffee: {title: notCoffee, productType: notCoffeeType, portion: coffeePortion},
	Tea:    {title: notTea, productType: notTeaType, tag: notTeaTag, portion: teaPortion},
}

func hasTag(p Product, re *regexp.Regexp) bool {
	return re != nil && slices.ContainsFunc(p.Tags, func(tag string) bool { return re.MatchString(strings.TrimSpace(tag)) })
}

// Skipped explains why a product produced no offers, for the scrape report.
type Skipped struct {
	Handle string
	Reason string
}

// ToOffers turns a store's products into offers of its kind. Products of
// another kind, or that state no size, are skipped with a reason: a missing
// weight is never guessed.
func ToOffers(products []Product, store Store) ([]Offer, []Skipped) {
	kind := store.Kind
	f, ok := filters[kind]
	if !ok {
		return nil, []Skipped{{"", fmt.Sprintf("unknown store kind %q", kind)}}
	}
	var offers []Offer
	var skipped []Skipped
	for _, p := range products {
		if f.title.MatchString(p.Title) || f.productType.MatchString(p.ProductType) || hasTag(p, f.tag) {
			skipped = append(skipped, Skipped{p.Handle, "not " + string(kind) + ": " + p.Title})
			continue
		}
		if hasTag(p, notConsumerTag) {
			skipped = append(skipped, Skipped{p.Handle, "not a consumer listing: " + p.Title})
			continue
		}
		bySize := map[int]Offer{}
		for _, v := range p.Variants {
			grams, label, src := variantWeight(p, v)
			if grams == 0 {
				continue
			}
			if f.portion != nil && (f.portion.MatchString(src) || f.portion.MatchString(v.Title)) {
				continue
			}
			if must := store.VariantMustMatch; must != nil && !must.MatchString(src) && !must.MatchString(v.Title) {
				continue
			}
			// A zero price is a placeholder or a free sample (Madcap, Onyx and SEY
			// each had one on 13 Oct 2026), not a price; storing it would also
			// break the price > 0 check and roll back the whole store.
			price, err := parseMinor(v.Price)
			if err != nil || price <= 0 {
				continue
			}
			o := Offer{
				ProductID: p.ID, Handle: p.Handle, Title: p.Title, ProductType: p.ProductType,
				SizeLabel: label, WeightGrams: grams, VariantID: v.ID, PriceMinor: price, Available: v.Available,
			}
			if len(p.Images) > 0 {
				o.ImageURL = p.Images[0].Src
			}
			if v.CompareAtPrice != nil {
				if c, err := parseMinor(*v.CompareAtPrice); err == nil && c > price {
					o.CompareAtMinor = c
				}
			}
			if cur, ok := bySize[grams]; !ok || better(o, cur) {
				bySize[grams] = o
			}
		}
		if len(bySize) == 0 {
			skipped = append(skipped, Skipped{p.Handle, "no size stated: " + p.Title})
			continue
		}
		sizes := make([]int, 0, len(bySize))
		for g := range bySize {
			sizes = append(sizes, g)
		}
		sort.Ints(sizes)
		for _, g := range sizes {
			offers = append(offers, bySize[g])
		}
	}
	return offers, skipped
}

// An available variant beats an unavailable one; then the cheaper wins.
func better(a, b Offer) bool {
	if a.Available != b.Available {
		return a.Available
	}
	return a.PriceMinor < b.PriceMinor
}

// The size usually sits in an option ("12 oz / Whole Bean"); single-size
// products often state it only in the product title. The text the size was
// read from is returned too, for the portion check.
func variantWeight(p Product, v Variant) (int, string, string) {
	for _, opt := range []*string{v.Option1, v.Option2, v.Option3} {
		if opt != nil {
			if g, l := ParseWeight(*opt); g > 0 {
				return g, l, *opt
			}
		}
	}
	if g, l := ParseWeight(v.Title); g > 0 {
		return g, l, v.Title
	}
	g, l := ParseWeight(p.Title)
	return g, l, p.Title
}

// parseMinor turns a decimal price string ("12.50") into minor units (1250)
// without going through float, so no cent is ever lost to rounding.
func parseMinor(s string) (int64, error) {
	s = strings.TrimSpace(s)
	whole, frac, _ := strings.Cut(s, ".")
	if len(frac) > 2 {
		return 0, fmt.Errorf("price %q has more than two decimals", s)
	}
	for len(frac) < 2 {
		frac += "0"
	}
	return strconv.ParseInt(whole+frac, 10, 64)
}
