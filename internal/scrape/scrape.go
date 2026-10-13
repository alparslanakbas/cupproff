// Package scrape runs one catalog pass over the active roasters and stores
// what it reads.
package scrape

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/ingest"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/robots"
	"github.com/alparslanakbas/cupproof/internal/shopify"
)

// BotToken is the product token our robots.txt checks use.
const BotToken = "cupproofbot"

// Runner reads one store at a time and checks each store's robots.txt
// before touching its catalog.
type Runner struct {
	Q      *db.Queries
	Ingest *ingest.Service
	Client *shopify.Client
	Log    *slog.Logger
	Only   []string // when set, only these roaster slugs are read
}

// Run reads every active Shopify roaster for the live markets it serves and
// returns how many roasters failed.
func (r *Runner) Run(ctx context.Context, markets []market.Market) (failed int, err error) {
	roasters, err := r.Q.ListActiveRoasters(ctx)
	if err != nil {
		return 0, err
	}
	for _, ro := range roasters {
		if ro.Platform != "shopify" || (len(r.Only) > 0 && !slices.Contains(r.Only, ro.Slug)) {
			continue
		}
		if err := r.one(ctx, ro.Slug, ro.BaseUrl, markets); err != nil {
			failed++
			r.Log.Error("roaster failed", "roaster", ro.Slug, "err", err)
		}
	}
	return failed, nil
}

// one reads a store: robots.txt first, then its /meta.json (where it ships,
// what it sells in), then the catalog once per market it serves. The meta
// is stored so the site knows each roaster's reach.
func (r *Runner) one(ctx context.Context, slug, baseURL string, markets []market.Market) error {
	rules, err := r.robots(ctx, baseURL)
	if err != nil {
		return fmt.Errorf("robots.txt: %w", err)
	}
	for _, path := range []string{"/meta.json", "/products.json"} {
		if !rules.Allowed(path) {
			return fmt.Errorf("robots.txt disallows %s", path)
		}
	}
	meta, err := r.Client.FetchMeta(ctx, baseURL)
	if err != nil {
		return fmt.Errorf("meta.json: %w", err)
	}
	if err := r.Q.UpdateRoasterMeta(ctx, db.UpdateRoasterMetaParams{
		Slug: slug, Country: nonEmpty(meta.Country), Currency: nonEmpty(meta.Currency), ShipsTo: meta.ShipsTo,
	}); err != nil {
		return err
	}
	read, skipped := shopify.MarketsFor(meta, markets)
	for _, why := range skipped {
		r.Log.Info("market skipped", "roaster", slug, "why", why)
	}
	for _, m := range read {
		products, err := r.Client.FetchProducts(ctx, baseURL, m)
		if err != nil {
			return fmt.Errorf("%s: %w", m.Code, err)
		}
		rep, err := r.Ingest.Shopify(ctx, slug, m, products, time.Now())
		if err != nil {
			return fmt.Errorf("%s: %w", m.Code, err)
		}
		r.Log.Info("roaster read", "roaster", slug, "market", m.Code,
			"products", rep.Products, "offers", rep.Offers, "skipped", len(rep.Skipped))
	}
	return nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// A missing robots.txt (404) allows everything; any other failure is an
// error, because "couldn't check" is not the same as "allowed".
func (r *Runner) robots(ctx context.Context, baseURL string) (robots.Rules, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/robots.txt", nil)
	if err != nil {
		return robots.Rules{}, err
	}
	req.Header.Set("User-Agent", r.Client.UserAgent)
	resp, err := r.Client.HTTP.Do(req)
	if err != nil {
		return robots.Rules{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return robots.Rules{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return robots.Rules{}, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return robots.Rules{}, err
	}
	return robots.Parse(string(body), BotToken), nil
}
