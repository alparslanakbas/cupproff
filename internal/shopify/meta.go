package shopify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/alparslanakbas/cupproof/internal/market"
)

// Meta is what a Shopify store says about itself in /meta.json.
type Meta struct {
	Name     string   `json:"name"`
	Country  string   `json:"country"`
	Currency string   `json:"currency"`
	ShipsTo  []string `json:"ships_to_countries"`
}

// FetchMeta reads a store's /meta.json.
func (c *Client) FetchMeta(ctx context.Context, baseURL string) (Meta, error) {
	var m Meta
	if err := c.getJSON(ctx, baseURL+"/meta.json", &m); err != nil {
		return Meta{}, fmt.Errorf("meta.json: %w", err)
	}
	m.Currency = strings.ToUpper(strings.TrimSpace(m.Currency))
	return m, nil
}

// HomePath is the page PresentedCurrency reads for a market; the caller
// checks it against robots.txt first.
func HomePath(m market.Market) string {
	q := url.Values{}
	q.Set("country", m.Country)
	q.Set("currency", m.Currency)
	return "/?" + q.Encode()
}

// The storefront writes the buyer's currency into every page's head:
// Shopify.currency = {"active":"EUR","rate":"1.207374"}.
var activeCurrency = regexp.MustCompile(`Shopify\.currency\s*=\s*\{\s*"active"\s*:\s*"([A-Za-z]{3})"`)

// PresentedCurrency asks which currency the store shows a buyer in m's
// country. products.json prices carry no currency, and a store that has not
// enabled the asked currency answers in its own without saying so (Klatch
// shows USD to Germany), so the catalog may be read for a market only after
// this says the store presents that market's currency.
//
// It reads the home page with the market's country and currency, as a buyer
// there would see it. /cart.js would be lighter, but Shopify's robots.txt
// disallows /cart for every bot, and we follow robots.txt. Measured on
// 13-14 Oct 2026: Kiss the Hippo EUR (rate 1.207 from GBP), Klatch USD.
// A home page can weigh 1.5 MB, which is why the answer is kept for a week.
func (c *Client) PresentedCurrency(ctx context.Context, baseURL string, m market.Market) (string, error) {
	body, err := c.Get(ctx, baseURL+HomePath(m), 4<<20)
	if err != nil {
		return "", fmt.Errorf("home page: %w", err)
	}
	match := activeCurrency.FindSubmatch(body)
	if match == nil {
		return "", errors.New("home page: no Shopify.currency")
	}
	return strings.ToUpper(string(match[1])), nil
}

// getJSON reads a small JSON document from a store.
func (c *Client) getJSON(ctx context.Context, u string, v any) error {
	body, err := c.Get(ctx, u, 1<<20)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, v)
}

// MarketsFor sorts the markets for a store: read now (it ships there and
// its default currency is the market's), check first (it ships there in
// another default currency, and may still present the market's currency
// through Shopify Markets: see PresentedCurrency), or skipped with a reason.
func MarketsFor(meta Meta, markets []market.Market) (read, check []market.Market, skipped []string) {
	for _, m := range markets {
		switch {
		case !slices.Contains(meta.ShipsTo, m.Country):
			skipped = append(skipped, fmt.Sprintf("%s: does not ship to %s", m.Code, m.Country))
		case meta.Currency != m.Currency:
			check = append(check, m)
		default:
			read = append(read, m)
		}
	}
	return read, check, skipped
}
