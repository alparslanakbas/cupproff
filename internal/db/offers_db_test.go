package db_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alparslanakbas/cupproof/internal/db"
	"github.com/alparslanakbas/cupproof/internal/pricing"
)

// testPool gives a freshly migrated database, or skips the test when
// TEST_DB is not set (it shows as skipped, never as passed). The database
// name must contain "test": the schema is dropped first, and a wrong
// connection string must not take real data with it.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DB")
	if url == "" {
		t.Skip("TEST_DB not set; database test skipped")
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.ConnConfig.Database, "test") {
		t.Fatalf("TEST_DB points at %q; its name must contain \"test\"", cfg.ConnConfig.Database)
	}
	ctx := context.Background()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestOffersForCoffeeAppliesTheGapRuleAndTracking(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := db.New(pool)
	roaster, err := q.RoasterBySlug(ctx, "perc") // seeded by migration 00002
	if err != nil {
		t.Fatal(err)
	}
	coffeeID, err := q.UpsertCoffee(ctx, db.UpsertCoffeeParams{RoasterID: roaster.ID, Handle: "test-coffee", Slug: "perc-test-coffee", Title: "Test Coffee"})
	if err != nil {
		t.Fatal(err)
	}
	offer := func(grams int32) int64 {
		id, err := q.UpsertOffer(ctx, db.UpsertOfferParams{CoffeeID: coffeeID, Market: "en-us", WeightGrams: grams, SizeLabel: "x", Currency: "USD", VariantID: 1})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	now := time.Now().UTC().Add(-time.Minute)
	add := func(offerID int64, daysAgo int, price int64) {
		at := pgtype.Timestamptz{Time: now.AddDate(0, 0, -daysAgo), Valid: true}
		if err := q.InsertPricePoint(ctx, db.InsertPricePointParams{OfferID: offerID, SeenAt: at, PriceMinor: price, Available: true}); err != nil {
			t.Fatal(err)
		}
	}

	// 9 days at $30, a 6-day gap, then 7 days at $20. Without the gap rule
	// the usual price would be $30 and the page would claim a 33% drop.
	gap := offer(340)
	for d := 20; d >= 12; d-- {
		add(gap, d, 3000)
	}
	for d := 6; d >= 0; d-- {
		add(gap, d, 2000)
	}
	// 40 days at $25, then $20 today: lowest since tracking began.
	tracked := offer(907)
	for d := 40; d >= 1; d-- {
		add(tracked, d, 2500)
	}
	add(tracked, 0, 2000)

	// The rules live in the summary refresh; pages only read its rows.
	if n, err := q.RefreshOfferSummaries(ctx); err != nil || n != 2 {
		t.Fatalf("refresh: %d rows, err %v; want 2", n, err)
	}
	rows, err := q.OffersForCoffee(ctx, db.OffersForCoffeeParams{CoffeeID: coffeeID, Market: "en-us"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	close := func(a, b time.Time) bool { d := a.Sub(b); return d > -time.Minute && d < time.Minute }

	g := rows[0]
	if g.Usual != 2000 || g.Low30 != 2000 || g.LowestTracked != 2000 {
		t.Errorf("gap offer: usual=%d low30=%d lowestTracked=%d; want 2000 each (only readings after the gap count)", g.Usual, g.Low30, g.LowestTracked)
	}
	if !close(g.TrackedSince.Time, now.AddDate(0, 0, -6)) {
		t.Errorf("gap offer tracked since %v, want the first reading after the gap (%v)", g.TrackedSince.Time, now.AddDate(0, 0, -6))
	}
	gs := pricing.Summary{Latest: g.PriceMinor, Low30: g.Low30, Usual: g.Usual, TrackedSince: g.TrackedSince.Time, LowestTracked: g.LowestTracked}
	if gs.DropPercent() != 0 {
		t.Errorf("gap offer shows a %d%% drop; a coffee back after a gap must not be measured against its old price", gs.DropPercent())
	}

	tr := rows[1]
	if tr.Usual != 2500 || tr.Low30 != 2000 || tr.LowestTracked != 2000 || !close(tr.TrackedSince.Time, now.AddDate(0, 0, -40)) {
		t.Errorf("tracked offer: usual=%d low30=%d lowestTracked=%d since=%v", tr.Usual, tr.Low30, tr.LowestTracked, tr.TrackedSince.Time)
	}
	ts := pricing.Summary{Latest: tr.PriceMinor, Low30: tr.Low30, Usual: tr.Usual, TrackedSince: tr.TrackedSince.Time, LowestTracked: tr.LowestTracked}
	if ts.DropPercent() != 20 || !ts.LowestIn30Days() {
		t.Errorf("tracked offer: drop=%d low30=%v; want 20%% and the 30-day low", ts.DropPercent(), ts.LowestIn30Days())
	}
	if _, ok := ts.LowestSince(time.Now()); !ok {
		t.Error("tracked offer: want 'lowest since' after 40 days at a higher price")
	}
}

// A store's price in another currency than its own follows the exchange
// rate (Bocca, 4 Nov 2026: EUR 39.40 every day, USD 45 and 46 by turns). The
// refresh compares the same size in the store's own currency, and only a drop
// there lets the converted price count as one.
func TestConvertedPriceDropsOnlyWithTheStoresOwnPrice(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := db.New(pool)
	roaster, err := q.RoasterBySlug(ctx, "perc") // seeded by migration 00002
	if err != nil {
		t.Fatal(err)
	}
	// A euro store selling in the US through Shopify Markets.
	if _, err := pool.Exec(ctx, "UPDATE roasters SET currency = 'EUR' WHERE id = $1", roaster.ID); err != nil {
		t.Fatal(err)
	}
	coffee := func(handle string) int64 {
		id, err := q.UpsertCoffee(ctx, db.UpsertCoffeeParams{RoasterID: roaster.ID, Handle: handle, Slug: "perc-" + handle, Title: handle})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	offer := func(coffeeID int64, market, currency string) int64 {
		id, err := q.UpsertOffer(ctx, db.UpsertOfferParams{CoffeeID: coffeeID, Market: market, WeightGrams: 1000, SizeLabel: "1 kg", Currency: currency, VariantID: 1})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	now := time.Now().UTC().Add(-time.Minute)
	prices := func(offerID, before, today int64) { // ten days at one price, today at another
		for d := 10; d >= 0; d-- {
			p := before
			if d == 0 {
				p = today
			}
			at := pgtype.Timestamptz{Time: now.AddDate(0, 0, -d), Valid: true}
			if err := q.InsertPricePoint(ctx, db.InsertPricePointParams{OfferID: offerID, SeenAt: at, PriceMinor: p, Available: true}); err != nil {
				t.Fatal(err)
			}
		}
	}
	rate := coffee("rate-only") // the euro price never moved
	rateEU, rateUS := offer(rate, "en-eu", "EUR"), offer(rate, "en-us", "USD")
	prices(rateEU, 3940, 3940)
	prices(rateUS, 4600, 4500)
	cut := coffee("real-cut") // the store cut its euro price; the dollar one follows
	cutEU, cutUS := offer(cut, "en-eu", "EUR"), offer(cut, "en-us", "USD")
	prices(cutEU, 4000, 3600)
	prices(cutUS, 4700, 4200)
	lone := coffee("no-euro-price") // never read in euros: nothing to compare with
	loneUS := offer(lone, "en-us", "USD")
	prices(loneUS, 4600, 4500)

	if n, err := q.RefreshOfferSummaries(ctx); err != nil || n != 5 {
		t.Fatalf("refresh: %d rows, err %v; want 5", n, err)
	}
	want := map[int64]string{
		rateEU: pricing.OwnCurrency, rateUS: pricing.OwnNoDrop,
		cutEU: pricing.OwnCurrency, cutUS: pricing.OwnDropped,
		loneUS: pricing.OwnUnknown,
	}
	for id, w := range want {
		var got string
		if err := pool.QueryRow(ctx, "SELECT own_price FROM offer_summaries WHERE offer_id = $1", id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != w {
			t.Errorf("offer %d: own_price %q, want %q", id, got, w)
		}
	}

	// The listing's "Real drops" count and rows apply the same rule.
	for market, slug := range map[string]string{"en-us": "perc-real-cut", "en-eu": "perc-real-cut"} {
		n, err := q.CountCoffees(ctx, db.CountCoffeesParams{Market: market, Kind: "coffee", Show: "drops"})
		if err != nil {
			t.Fatal(err)
		}
		rows, err := q.ListCoffees(ctx, db.ListCoffeesParams{Market: market, Kind: "coffee", Show: "drops", PageSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if n != 1 || len(rows) != 1 || rows[0].Slug != slug {
			t.Errorf("%s drops: count %d, %d rows %v; want only %s", market, n, len(rows), rows, slug)
		}
	}
	// And the coffee page: the dollar offer of the rate-only coffee shows no drop.
	rows, err := q.OffersForCoffee(ctx, db.OffersForCoffeeParams{CoffeeID: rate, Market: "en-us"})
	if err != nil || len(rows) != 1 {
		t.Fatalf("offers: %v, %d rows", err, len(rows))
	}
	r := rows[0]
	s := pricing.Summary{Latest: r.PriceMinor, Low30: r.Low30, Usual: r.Usual, TrackedSince: r.TrackedSince.Time, LowestTracked: r.LowestTracked, OwnPrice: r.OwnPrice}
	if s.DropPercent() != 0 || s.LowestIn30Days() {
		t.Errorf("rate-only offer shows drop %d%%, low30 %v", s.DropPercent(), s.LowestIn30Days())
	}
}

// One Shopify shop on two domains (Has Bean and Ozone, 13 Oct 2026) must
// not be stored twice: the second domain's meta update fails.
func TestSameShopOnTwoDomainsIsRefused(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	q := db.New(pool)
	meta := func(slug string) error {
		return q.UpdateRoasterMeta(ctx, db.UpdateRoasterMetaParams{Slug: slug, ShipsTo: []string{"GB"}, ShopID: 231572})
	}
	if err := meta("ozone"); err != nil { // seeded by migration 00008
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO roasters (slug, name, base_url) VALUES ('hasbean', 'Has Bean', 'https://hasbean.co.uk')"); err != nil {
		t.Fatal(err)
	}
	if err := meta("hasbean"); err == nil || !strings.Contains(err.Error(), "roasters_shop_id_key") {
		t.Fatalf("second domain of the same shop: err %v, want the roasters_shop_id_key violation", err)
	}
	// A meta.json without an id stores NULL, and NULLs never collide.
	for _, slug := range []string{"perc", "onyx"} {
		if err := q.UpdateRoasterMeta(ctx, db.UpdateRoasterMetaParams{Slug: slug, ShipsTo: []string{"US"}}); err != nil {
			t.Fatalf("%s without a shop id: %v", slug, err)
		}
	}
}
