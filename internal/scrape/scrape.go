// Package scrape runs one catalog pass over the active roasters and stores
// what it reads.
package scrape

import (
	"context"
	"errors"
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

// Run reads every active Shopify roaster for one market and returns how
// many failed.
func (r *Runner) Run(ctx context.Context, m market.Market) (failed int, err error) {
	roasters, err := r.Q.ListActiveRoasters(ctx)
	if err != nil {
		return 0, err
	}
	for _, ro := range roasters {
		if ro.Platform != "shopify" || (len(r.Only) > 0 && !slices.Contains(r.Only, ro.Slug)) {
			continue
		}
		if err := r.one(ctx, ro.Slug, ro.BaseUrl, m); err != nil {
			failed++
			r.Log.Error("roaster failed", "roaster", ro.Slug, "market", m.Code, "err", err)
		}
	}
	return failed, nil
}

func (r *Runner) one(ctx context.Context, slug, baseURL string, m market.Market) error {
	allowed, err := r.allowed(ctx, baseURL, "/products.json")
	if err != nil {
		return fmt.Errorf("robots.txt: %w", err)
	}
	if !allowed {
		return errors.New("robots.txt disallows /products.json")
	}
	products, err := r.Client.FetchProducts(ctx, baseURL, m)
	if err != nil {
		return err
	}
	rep, err := r.Ingest.Shopify(ctx, slug, m, products, time.Now())
	if err != nil {
		return err
	}
	r.Log.Info("roaster read", "roaster", slug, "market", m.Code,
		"products", rep.Products, "offers", rep.Offers, "skipped", len(rep.Skipped))
	return nil
}

// A missing robots.txt (404) allows everything; any other failure is an
// error, because "couldn't check" is not the same as "allowed".
func (r *Runner) allowed(ctx context.Context, baseURL, path string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/robots.txt", nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", r.Client.UserAgent)
	resp, err := r.Client.HTTP.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return false, err
	}
	return robots.Parse(string(body), BotToken).Allowed(path), nil
}
