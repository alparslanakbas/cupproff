// Package ingest stores one collector delivery: a store's catalog for one
// market, turned into coffee offers and price points.
package ingest

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/market"
	"github.com/alparslanakbas/cupproof/internal/shopify"
)

// ErrEmptyCatalog refuses a delivery with no products. An empty catalog is
// far more likely a blocked or broken read than a store that sells nothing,
// and storing it would make every coffee of the store look gone.
var ErrEmptyCatalog = errors.New("empty catalog refused")

// Report summarises one delivery.
type Report struct {
	Products int
	Offers   int
	Skipped  []shopify.Skipped
}

// Service stores deliveries.
type Service struct {
	Pool *pgxpool.Pool
}

// Shopify stores a Shopify catalog read by the collector. Everything is
// written in one transaction: a half-stored catalog would show some sizes
// with today's price and others with yesterday's.
func (s *Service) Shopify(ctx context.Context, roasterSlug string, m market.Market, products []shopify.Product, seenAt time.Time) (Report, error) {
	if len(products) == 0 {
		return Report{}, ErrEmptyCatalog
	}
	roaster, err := db.New(s.Pool).RoasterBySlug(ctx, roasterSlug)
	if err != nil {
		return Report{}, fmt.Errorf("roaster %q: %w", roasterSlug, err)
	}
	store := shopify.Store{Kind: shopify.Kind(roaster.Kind)}
	if roaster.VariantMustMatch != nil {
		// A broken rule fails the store rather than reading it unfiltered.
		if store.VariantMustMatch, err = regexp.Compile(*roaster.VariantMustMatch); err != nil {
			return Report{}, fmt.Errorf("roaster %q variant rule: %w", roasterSlug, err)
		}
	}
	offers, skipped := shopify.ToOffers(products, store)
	rep := Report{Products: len(products), Offers: len(offers), Skipped: skipped}

	err = pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		q := db.New(tx)
		var err error
		at := seenAt.UTC().Truncate(time.Second)
		coffeeIDs := map[string]int64{}
		for _, o := range offers {
			coffeeID, ok := coffeeIDs[o.Handle]
			if !ok {
				coffeeID, err = q.UpsertCoffee(ctx, db.UpsertCoffeeParams{
					RoasterID: roaster.ID, Handle: o.Handle, Slug: roaster.Slug + "-" + o.Handle,
					Title: o.Title, ProductType: o.ProductType, ImageUrl: nonEmpty(o.ImageURL),
				})
				if err != nil {
					return fmt.Errorf("coffee %q: %w", o.Handle, err)
				}
				coffeeIDs[o.Handle] = coffeeID
			}
			offerID, err := q.UpsertOffer(ctx, db.UpsertOfferParams{
				CoffeeID: coffeeID, Market: m.Code, WeightGrams: int32(o.WeightGrams),
				SizeLabel: o.SizeLabel, Currency: m.Currency, VariantID: o.VariantID,
			})
			if err != nil {
				return fmt.Errorf("offer %q %dg: %w", o.Handle, o.WeightGrams, err)
			}
			var compareAt *int64
			if o.CompareAtMinor > 0 {
				compareAt = &o.CompareAtMinor
			}
			if err := q.InsertPricePoint(ctx, db.InsertPricePointParams{
				OfferID: offerID, SeenAt: timestamptz(at), PriceMinor: o.PriceMinor,
				CompareAtMinor: compareAt, Available: o.Available,
			}); err != nil {
				return fmt.Errorf("price %q %dg: %w", o.Handle, o.WeightGrams, err)
			}
		}
		return nil
	})
	return rep, err
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
