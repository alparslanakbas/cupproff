// Package shopify reads coffee offers from Shopify storefronts.
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
var notCoffee = regexp.MustCompile(`(?i)\b(mugs?|tumblers?|bottles?|carafes?|kettles?|grinders?|drippers?|scales?|candles?|` +
	`t-shirts?|shirts?|hats?|totes?|hoodies?|gift ?cards?|subscriptions?|merch(andise)?|equipment|apparel|accessor(y|ies)|unroasted|` +
	`wholesale)\b|\(ws\)`)

// The store's own product type is the stronger signal when it is set: on
// 13 Oct 2026 "Drinkware" cups ("Latte Cup 12oz"), "Gifts" (honey 12oz) and
// "Green" (unroasted beans, not comparable with roasted coffee) slipped past
// the title check because their text states a size.
// Added on the second roaster round (13 Oct 2026, product types seen in the
// stores' own catalogs): tea (Passenger sells tea), "Goods", "Sundry Retail",
// "RTD" (ready-to-drink cans); brew equipment is caught by "equipment". "retail" and
// "brew" are left out on purpose: "Retail Coffee" and "Cold Brew" blends are coffee.
var notCoffeeType = regexp.MustCompile(`(?i)\b(drinkware|gifts?|green|unroasted|merch(andise)?|apparel|equipment|` +
	`accessor(y|ies)|brewing|gear|subscriptions?|gift ?cards?|teas?|goods|sundry|rtd|ready[ -]to[ -]drink)\b`)

// White-label and wholesale products carry trade prices for another brand,
// not a price a customer pays: Onyx publishes dozens tagged "white-label"
// with code names ("IO-E-05", $4.58 for 8.8 oz; 13 Oct 2026).
var notConsumerTag = regexp.MustCompile(`(?i)^(white[- ]?label|wholesale|private[- ]?label)$`)

// Skipped explains why a product produced no offers, for the scrape report.
type Skipped struct {
	Handle string
	Reason string
}

// ToOffers turns storefront products into coffee offers. Products that
// aren't coffee, or that state no bag size, are skipped with a reason:
// a missing weight is never guessed.
func ToOffers(products []Product) ([]Offer, []Skipped) {
	var offers []Offer
	var skipped []Skipped
	for _, p := range products {
		if notCoffee.MatchString(p.Title) || notCoffeeType.MatchString(p.ProductType) {
			skipped = append(skipped, Skipped{p.Handle, "not coffee: " + p.Title})
			continue
		}
		if slices.ContainsFunc(p.Tags, func(tag string) bool { return notConsumerTag.MatchString(strings.TrimSpace(tag)) }) {
			skipped = append(skipped, Skipped{p.Handle, "not a consumer listing: " + p.Title})
			continue
		}
		bySize := map[int]Offer{}
		for _, v := range p.Variants {
			grams, label := variantWeight(p, v)
			if grams == 0 {
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
			skipped = append(skipped, Skipped{p.Handle, "no bag size stated: " + p.Title})
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
// coffees often state it only in the product title.
func variantWeight(p Product, v Variant) (int, string) {
	for _, opt := range []*string{v.Option1, v.Option2, v.Option3} {
		if opt != nil {
			if g, l := ParseWeight(*opt); g > 0 {
				return g, l
			}
		}
	}
	if g, l := ParseWeight(v.Title); g > 0 {
		return g, l
	}
	return ParseWeight(p.Title)
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
