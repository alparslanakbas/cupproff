package shopify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/alparslanakbas/cupproof/internal/market"
)

// ErrNotShopify means the store has no /products.json (not Shopify, or it
// left the Online Store channel, as Huel did on 8 Oct 2026).
var ErrNotShopify = errors.New("store has no /products.json")

// ErrRateLimited means Shopify answered 429; the run for this store stops
// with what it has so far instead of hammering the store.
var ErrRateLimited = errors.New("rate limited (429)")

// ErrChallenged means Cloudflare put a "Verifying your connection" page in
// front of the store (403 with cf-mitigated: challenge).
var ErrChallenged = errors.New("bot challenge (403)")

// ErrPageCap comes back WITH the products read so far when the catalog
// still had pages at MaxPages. The cap is a guard against a store that never
// returns an empty page, not a limit on purpose: a capped catalog is stored
// and the caller warns about it.
var ErrPageCap = errors.New("catalog cut at the page cap")

// NewTransport returns the transport store requests must use.
//
// Compression is OFF on purpose. With an Accept-Encoding header, Cloudflare
// answered our bot requests to Shopify storefronts with a 403 challenge;
// without it the same request got 200. Measured 13 Oct 2026, from home and
// from the server, with Go, .NET and curl alike: the header, not the client
// language, triggers it. Uncompressed catalogs are larger; we read them once
// a day, so the trade is worth it.
func NewTransport() *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DisableCompression = true
	return t
}

// Client reads storefront catalogs politely: one request at a time per
// store, with a pause between pages.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	PageDelay time.Duration
	MaxPages  int
}

// FetchProducts reads every catalog page of a store as a customer of m.
//
// The market is always stated explicitly (currency AND country): without
// country, Shopify picks the market from the caller's location and silently
// returns other prices or hides products (WheyProof, 25 Sep 2026).
//
// A short page does NOT end the catalog: Shopify paginates first and filters
// by market afterwards, so a page can hold fewer than 250 products while
// more follow (WheyProof, 5 Oct 2026). Only an empty page ends it.
func (c *Client) FetchProducts(ctx context.Context, baseURL string, m market.Market) ([]Product, error) {
	var all []Product
	for page := 1; page <= c.MaxPages; page++ {
		if page > 1 && c.PageDelay > 0 {
			select {
			case <-ctx.Done():
				return all, ctx.Err()
			case <-time.After(c.PageDelay):
			}
		}
		q := url.Values{}
		q.Set("limit", "250")
		q.Set("page", fmt.Sprint(page))
		q.Set("currency", m.Currency)
		q.Set("country", m.Country)
		products, err := c.fetchPage(ctx, baseURL+"/products.json?"+q.Encode())
		if err != nil {
			return all, err
		}
		if len(products) == 0 {
			return all, nil
		}
		all = append(all, products...)
	}
	return all, ErrPageCap
}

func (c *Client) fetchPage(ctx context.Context, pageURL string) ([]Product, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrNotShopify
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("Cf-Mitigated") != "":
		return nil, ErrChallenged
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s: HTTP %d", pageURL, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return nil, err
	}
	return DecodeProducts(body)
}
