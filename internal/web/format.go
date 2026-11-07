package web

import (
	"fmt"
	"math"
	"net/url"
	"strconv"

	"github.com/alparslanakbas/cupproof/internal/i18n"
)

// money formats minor units as lang writes prices: 1925, "USD" → "$19.25"
// in English.
func money(lang string, minor int64, currency string) string {
	return i18n.Money(lang, minor, currency)
}

// perKilo is the price of one kilogram, from the bag's stated weight.
// Without a stated weight there is no per-kilo price (never assumed).
func perKilo(lang string, priceMinor int64, grams int32, currency string) string {
	if grams <= 0 {
		return ""
	}
	return money(lang, int64(math.Round(float64(priceMinor)*1000/float64(grams))), currency) + "/kg"
}

// perHundredGrams is the price of 100 g, the unit tea is compared in.
func perHundredGrams(lang string, priceMinor int64, grams int32, currency string) string {
	if grams <= 0 {
		return ""
	}
	return money(lang, int64(math.Round(float64(priceMinor)*100/float64(grams))), currency) + "/100 g"
}

// cdnImage asks Shopify's image CDN for a resized copy. The CDN already
// serves WebP/AVIF to browsers that accept it; with width=480 a product
// image went from 118 KB to 15 KB (13 Oct 2026). Other hosts are returned
// unchanged.
func cdnImage(raw string, width int) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host != "cdn.shopify.com" {
		return raw
	}
	q := u.Query()
	q.Set("width", strconv.Itoa(width))
	u.RawQuery = q.Encode()
	return u.String()
}

// decimal renders minor units for structured data: 1925 → "19.25". It is the
// same in every language: schema.org wants a dot.
func decimal(minor int64) string {
	return fmt.Sprintf("%d.%02d", minor/100, minor%100)
}
