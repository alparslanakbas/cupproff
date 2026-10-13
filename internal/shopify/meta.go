package shopify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/meta.json", nil)
	if err != nil {
		return Meta{}, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Meta{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Meta{}, ErrNotShopify
	case resp.StatusCode == http.StatusTooManyRequests:
		return Meta{}, ErrRateLimited
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("Cf-Mitigated") != "":
		return Meta{}, ErrChallenged
	case resp.StatusCode != http.StatusOK:
		return Meta{}, fmt.Errorf("meta.json: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(body, &m); err != nil {
		return Meta{}, fmt.Errorf("meta.json: %w", err)
	}
	m.Currency = strings.ToUpper(strings.TrimSpace(m.Currency))
	return m, nil
}

// MarketsFor picks the markets to read for a store and says why the others
// are skipped.
//
// A market is read only when the store ships to its country AND its default
// currency is the market's. The currency rule is strict on purpose: a store
// can show another country converted prices (Shopify Markets), and checking
// which currency it actually presents is not built yet; until it is, such a
// market is skipped rather than risk storing a price in the wrong currency.
func MarketsFor(meta Meta, markets []market.Market) (read []market.Market, skipped []string) {
	for _, m := range markets {
		switch {
		case !slices.Contains(meta.ShipsTo, m.Country):
			skipped = append(skipped, fmt.Sprintf("%s: does not ship to %s", m.Code, m.Country))
		case meta.Currency != m.Currency:
			skipped = append(skipped, fmt.Sprintf("%s: sells in %s, market needs %s", m.Code, meta.Currency, m.Currency))
		default:
			read = append(read, m)
		}
	}
	return read, skipped
}
