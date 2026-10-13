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
