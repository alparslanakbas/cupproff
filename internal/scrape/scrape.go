// Package scrape runs one catalog pass over the active roasters and stores
// what it reads.
package scrape

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

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
	var todo []db.ListActiveRoastersRow
	for _, ro := range roasters {
		if ro.Platform == "shopify" && (len(r.Only) == 0 || slices.Contains(r.Only, ro.Slug)) {
			todo = append(todo, ro)
		}
	}
	var refusals breaker
	for i, ro := range todo {
		err := r.one(ctx, ro.Slug, ro.BaseUrl, markets)
		if err != nil {
			failed++
			r.Log.Error("roaster failed", "roaster", ro.Slug, "err", err)
		}
		if refusals.record(err) {
			// Not an error return: the stores read so far must still reach
			// the pages (the caller refreshes summaries), and the unread ones
			// count as failed so the exit code shows the stop.
			r.Log.Error("run stopped: stores keep refusing us; the IP is shared with the sister sites",
				"in_a_row", refusals.inARow, "not_read", len(todo)-i-1)
			return failed + len(todo) - i - 1, nil
		}
	}
	return failed, nil
}

// stopAfter is how many stores may refuse us in a row (429, or a bot
// challenge) before the run stops. The server's IP is shared with the
// sister sites (WheyProof, ProteinAvcısı); pressing on against a store
// platform that has started refusing would spend that IP's standing for
// all of them.
const stopAfter = 3

// breaker counts refusals in a row; any other outcome resets it.
type breaker struct{ inARow int }

// record notes one store's result and reports whether the run must stop.
func (b *breaker) record(err error) bool {
	if errors.Is(err, shopify.ErrRateLimited) || errors.Is(err, shopify.ErrChallenged) {
		b.inARow++
	} else {
		b.inARow = 0
	}
	return b.inARow >= stopAfter
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
		return err
	}
	if err := r.Q.UpdateRoasterMeta(ctx, db.UpdateRoasterMetaParams{
		Slug: slug, Country: nonEmpty(meta.Country), Currency: nonEmpty(meta.Currency), ShipsTo: meta.ShipsTo,
		ShopID: meta.ID,
	}); err != nil {
		return err
	}
	read, check, skipped := shopify.MarketsFor(meta, markets)
	for _, m := range check {
		cur, err := r.presented(ctx, slug, baseURL, m, rules)
		switch {
		case err != nil:
			// A refusal counts for the breaker; anything else only skips
			// this market for today.
			if errors.Is(err, shopify.ErrRateLimited) || errors.Is(err, shopify.ErrChallenged) {
				return fmt.Errorf("%s currency check: %w", m.Code, err)
			}
			skipped = append(skipped, fmt.Sprintf("%s: currency check failed: %v", m.Code, err))
		case cur != m.Currency:
			skipped = append(skipped, fmt.Sprintf("%s: shows %s to %s, market needs %s", m.Code, cur, m.Country, m.Currency))
		default:
			read = append(read, m)
		}
	}
	for _, why := range skipped {
		r.Log.Info("market skipped", "roaster", slug, "why", why)
	}
	for _, m := range read {
		products, err := r.Client.FetchProducts(ctx, baseURL, m)
		if errors.Is(err, shopify.ErrPageCap) {
			r.Log.Warn("catalog cut at the page cap; raise MaxPages", "roaster", slug, "market", m.Code, "products", len(products))
		} else if err != nil {
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

// currencyCheckDays is how long a store's presented currency is trusted.
// Stores rarely change their markets, and the check reads a whole home page.
const currencyCheckDays = 7

// presented returns the currency the store shows in m's country: from the
// last week's check if there is one, otherwise by reading its home page as a
// buyer there (when robots.txt allows) and keeping the answer.
func (r *Runner) presented(ctx context.Context, slug, baseURL string, m market.Market, rules robots.Rules) (string, error) {
	cur, err := r.Q.PresentedCurrency(ctx, db.PresentedCurrencyParams{Slug: slug, Market: m.Code, MaxAgeDays: currencyCheckDays})
	if err == nil {
		return cur, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if path := shopify.HomePath(m); !rules.Allowed(path) {
		return "", fmt.Errorf("robots.txt disallows %s", path)
	}
	cur, err = r.Client.PresentedCurrency(ctx, baseURL, m)
	if err != nil {
		return "", err
	}
	if err := r.Q.SavePresentedCurrency(ctx, db.SavePresentedCurrencyParams{Slug: slug, Market: m.Code, Currency: cur}); err != nil {
		return "", err
	}
	r.Log.Info("currency checked", "roaster", slug, "market", m.Code, "presents", cur)
	return cur, nil
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// A missing robots.txt (404) allows everything; any other failure is an
// error, because "couldn't check" is not the same as "allowed".
// It goes through the client like every store request: refusals map to the
// errors the breaker counts, and a 5xx gets its one retry.
func (r *Runner) robots(ctx context.Context, baseURL string) (robots.Rules, error) {
	body, err := r.Client.Get(ctx, baseURL+"/robots.txt", 512<<10)
	if errors.Is(err, shopify.ErrNotShopify) { // 404: no robots.txt
		return robots.Rules{}, nil
	}
	if err != nil {
		return robots.Rules{}, err
	}
	return robots.Parse(string(body), BotToken), nil
}
