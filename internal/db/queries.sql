-- name: ListActiveRoasters :many
SELECT id, slug, name, base_url, platform, kind
FROM roasters
WHERE active
ORDER BY slug;

-- name: RoasterBySlug :one
SELECT id, slug, name, base_url, platform, kind, variant_must_match
FROM roasters
WHERE slug = $1;

-- name: UpsertCoffee :one
INSERT INTO coffees (roaster_id, handle, slug, title, product_type, image_url)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (roaster_id, handle) DO UPDATE
SET title = EXCLUDED.title,
    product_type = EXCLUDED.product_type,
    image_url = EXCLUDED.image_url,
    updated_at = now()
RETURNING id;

-- name: UpsertOffer :one
INSERT INTO offers (coffee_id, market, weight_grams, size_label, currency, variant_id)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (coffee_id, market, weight_grams) DO UPDATE
SET size_label = EXCLUDED.size_label,
    currency = EXCLUDED.currency,
    variant_id = EXCLUDED.variant_id,
    last_seen_at = now()
RETURNING id;

-- name: InsertPricePoint :exec
INSERT INTO price_points (offer_id, seen_at, price_minor, compare_at_minor, available)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (offer_id, seen_at) DO NOTHING;

-- Market listing: one card per coffee IN STOCK, showing its smallest bag
-- that can be bought now. Most roasters keep sold-out lots published (13 Oct
-- 2026: SEY 9 of 669 coffees in stock, Onyx 167 of 527), and a listing of
-- sold-out coffees is useless to a buyer. Offers not seen for 48 hours are
-- stale and never shown. Verdict figures come from offer_summaries.
-- name: ListCoffees :many
SELECT x.slug, x.title, x.image_url, x.roaster_name, x.size_label, x.weight_grams, x.currency,
       x.price_minor, x.compare_at_minor, x.low30, x.usual, x.tracked_since, x.lowest_tracked, x.offer_id,
       x.handle, x.roaster_url, x.variant_id, x.own_price, x.store_currency, x.roaster_slug, x.last_read
FROM (
    SELECT DISTINCT ON (c.id)
        c.slug, c.title, c.image_url, r.name AS roaster_name, o.id AS offer_id,
        c.handle, r.base_url AS roaster_url, o.variant_id,
        o.size_label, o.weight_grams, o.currency,
        s.latest_price AS price_minor, s.latest_compare_at AS compare_at_minor,
        s.low30, s.usual, s.tracked_since, s.lowest_tracked, s.own_price, r.currency AS store_currency,
        r.slug AS roaster_slug, r.last_read_at AS last_read, c.created_at
    FROM coffees c
    JOIN roasters r ON r.id = c.roaster_id
    JOIN offers o ON o.coffee_id = c.id AND o.market = @market AND o.last_seen_at > now() - interval '48 hours'
    JOIN offer_summaries s ON s.offer_id = o.id
    WHERE s.latest_available AND r.kind = @kind
      AND (@show::text = '' OR (@show = 'drops' AND s.usual > s.latest_price AND s.own_price IN ('', 'dropped')) OR (@show = 'sales' AND s.latest_compare_at IS NOT NULL))
      AND (@roaster::bigint = 0 OR r.id = @roaster) AND (@title::text = '' OR c.title ~* @title)
      AND (@title_not::text = '' OR c.title !~* @title_not)
      AND (@search::text = '' OR (c.title || ' ' || r.name) ~* @search)
    -- The size a row shows: a real drop first, then a store sale, then the
    -- smallest bag, so a coffee whose 1 kg bag dropped isn't listed by its
    -- unchanged 250 g bag and sorted among the full prices.
    ORDER BY c.id, (s.usual > s.latest_price AND s.own_price IN ('', 'dropped')) DESC, (s.latest_compare_at IS NOT NULL) DESC, o.weight_grams
) x
-- @sort picks the order (the sister sites' choices); every order ends with
-- the default, "biggest drop": real drops first, deepest first (the same
-- arithmetic as pricing.DropPercent: below the usual price, rounded down;
-- never a converted price that only followed the exchange rate), then the
-- store's own sales, deepest first, then grouped by store, like a café menu
-- lists its producers. @show narrows the list to real drops or to the
-- store's own sales (empty = everything); @roaster to one store (0 = all);
-- @title and @title_not to titles that match and don't match a regular
-- expression (a category; empty = all); @search to products whose title and
-- store name match a regular expression.
ORDER BY CASE WHEN @sort::text = 'price_asc' THEN x.price_minor END ASC,
         CASE WHEN @sort = 'price_desc' THEN x.price_minor END DESC,
         CASE WHEN @sort = 'unit_asc' THEN x.price_minor::float8 / NULLIF(x.weight_grams, 0) END ASC NULLS LAST,
         CASE WHEN @sort = 'newest' THEN x.created_at END DESC,
         CASE WHEN @sort = 'oldest' THEN x.created_at END ASC,
         CASE WHEN @sort = 'name_asc' THEN x.title END ASC,
         CASE WHEN @sort = 'name_desc' THEN x.title END DESC,
         -- the store sales view: the store's own discount first, as the
         -- sister sites order it
         CASE WHEN @show = 'sales' AND x.compare_at_minor > x.price_minor THEN (x.compare_at_minor - x.price_minor) * 100 / x.compare_at_minor END DESC NULLS LAST,
         (x.usual > x.price_minor AND x.own_price IN ('', 'dropped')) DESC,
         (CASE WHEN x.usual > x.price_minor AND x.own_price IN ('', 'dropped') THEN (x.usual - x.price_minor) * 100 / x.usual ELSE 0 END) DESC,
         (x.compare_at_minor > x.price_minor) DESC NULLS LAST,
         (CASE WHEN x.compare_at_minor > x.price_minor THEN (x.compare_at_minor - x.price_minor) * 100 / x.compare_at_minor ELSE 0 END) DESC,
         x.roaster_name, x.title, x.slug
LIMIT @page_size OFFSET @skip;

-- name: CountCoffees :one
SELECT count(DISTINCT o.coffee_id)
FROM offers o
JOIN offer_summaries s ON s.offer_id = o.id
JOIN coffees c ON c.id = o.coffee_id
JOIN roasters r ON r.id = c.roaster_id
WHERE o.market = @market AND o.last_seen_at > now() - interval '48 hours' AND s.latest_available AND r.kind = @kind
  AND (@show::text = '' OR (@show = 'drops' AND s.usual > s.latest_price AND s.own_price IN ('', 'dropped')) OR (@show = 'sales' AND s.latest_compare_at IS NOT NULL))
  AND (@roaster::bigint = 0 OR r.id = @roaster) AND (@title::text = '' OR c.title ~* @title)
  AND (@title_not::text = '' OR c.title !~* @title_not)
  AND (@search::text = '' OR (c.title || ' ' || r.name) ~* @search);

-- name: CoffeeBySlug :one
SELECT c.id, c.slug, c.title, c.product_type, c.image_url, c.handle,
       r.name AS roaster_name, r.base_url AS roaster_url, r.kind, r.currency AS store_currency
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
WHERE c.slug = $1;

-- Offers of one coffee in one market with their summaries.
-- name: OffersForCoffee :many
SELECT o.id, o.size_label, o.weight_grams, o.currency, o.variant_id,
       s.latest_price AS price_minor, s.latest_compare_at AS compare_at_minor,
       s.latest_available AS available, s.latest_seen_at AS seen_at,
       s.low30, s.usual, s.tracked_since, s.lowest_tracked, s.last_available, s.own_price
FROM offers o
JOIN offer_summaries s ON s.offer_id = o.id
WHERE o.coffee_id = @coffee_id AND o.market = @market AND o.last_seen_at > now() - interval '48 hours'
ORDER BY o.weight_grams;

-- Recomputes every current offer's summary; run after each scrape. One
-- set-based statement, no loop per offer. The rules are the sister sites'
-- (TR PriceSummaryRefresher, TakipDibi):
--   * gap rule: if two readings in the last 30 days are more than 72 hours
--     apart, the offer restarted at the reading after the latest such gap and
--     only readings from there on count. A coffee that vanished and came back
--     (a new lot, a changed bag) is never measured against its old price.
--   * low30: the lowest price in that window.
--   * usual: the highest price seen on at least 7 different UTC days in that
--     window; 0 when there is none yet ("not enough history").
--   * tracked_since / lowest_tracked: where tracking starts (the gap restart,
--     else the first reading ever) and the lowest price since then.
--   * own_price: for an offer shown in another currency than the store's own
--     (Shopify converts it at the day's rate), whether the same size's price
--     in the store's own currency dropped too ('dropped'), didn't ('no-drop')
--     or can't be compared ('unknown': not read in that currency, or too
--     new); '' for an offer in the store's own currency. Only '' and
--     'dropped' may show a drop (pricing.Summary.RateOnly).
-- Prices are > 0 by CHECK, so 0 always means "none".
-- name: RefreshOfferSummaries :execrows
WITH o AS (
    SELECT f.id, f.coffee_id, f.weight_grams, f.currency, r.currency AS store_currency
    FROM offers f
    JOIN coffees c ON c.id = f.coffee_id
    JOIN roasters r ON r.id = c.roaster_id
    WHERE f.last_seen_at > now() - interval '48 hours'
),
seg AS (
    SELECT a.offer_id,
           max(a.seen_at) FILTER (WHERE a.seen_at - a.prev > interval '72 hours') AS restart
    FROM (
        SELECT p.offer_id, p.seen_at,
               lag(p.seen_at) OVER (PARTITION BY p.offer_id ORDER BY p.seen_at) AS prev
        FROM price_points p
        JOIN o ON o.id = p.offer_id
        WHERE p.seen_at >= now() - interval '30 days'
    ) a
    GROUP BY a.offer_id
),
win AS (
    SELECT p.offer_id, p.price_minor, p.seen_at
    FROM price_points p
    JOIN o ON o.id = p.offer_id
    LEFT JOIN seg ON seg.offer_id = p.offer_id
    WHERE p.seen_at >= COALESCE(seg.restart, now() - interval '30 days')
),
low AS (
    SELECT offer_id, min(price_minor) AS low30 FROM win GROUP BY offer_id
),
usual AS (
    SELECT d.offer_id, max(d.price_minor) AS usual
    FROM (
        SELECT offer_id, price_minor
        FROM win
        GROUP BY offer_id, price_minor
        HAVING count(DISTINCT (seen_at AT TIME ZONE 'UTC')::date) >= 7
    ) d
    GROUP BY d.offer_id
),
alltime AS (
    SELECT p.offer_id, min(p.price_minor) AS low, min(p.seen_at) AS first,
           max(p.seen_at) FILTER (WHERE p.available) AS last_available
    FROM price_points p
    JOIN o ON o.id = p.offer_id
    GROUP BY p.offer_id
),
calc AS (
    SELECT o.id, o.coffee_id, o.weight_grams, o.currency, o.store_currency,
           lp.price_minor, lp.compare_at_minor, lp.available, lp.seen_at,
           COALESCE(low.low30, 0) AS low30, COALESCE(usual.usual, 0) AS usual,
           COALESCE(seg.restart, alltime.first) AS tracked_since,
           COALESCE(CASE WHEN seg.restart IS NULL THEN alltime.low ELSE low.low30 END, 0) AS lowest_tracked,
           alltime.last_available
    FROM o
    JOIN LATERAL (
        SELECT p.price_minor, p.compare_at_minor, p.available, p.seen_at
        FROM price_points p
        WHERE p.offer_id = o.id
        ORDER BY p.seen_at DESC
        LIMIT 1
    ) lp ON true
    JOIN alltime ON alltime.offer_id = o.id
    LEFT JOIN seg ON seg.offer_id = o.id
    LEFT JOIN low ON low.offer_id = o.id
    LEFT JOIN usual ON usual.offer_id = o.id
),
-- Each size in the store's own currency. One market per currency makes it
-- one row per size; DISTINCT ON keeps the join below from ever doubling an
-- offer, which the upsert would refuse.
own AS (
    SELECT DISTINCT ON (coffee_id, weight_grams) coffee_id, weight_grams, currency, usual, price_minor
    FROM calc
    WHERE currency = store_currency
    ORDER BY coffee_id, weight_grams, id
)
INSERT INTO offer_summaries (offer_id, latest_price, latest_compare_at, latest_available, latest_seen_at,
                             low30, usual, tracked_since, lowest_tracked, last_available, own_price, refreshed_at)
SELECT x.id, x.price_minor, x.compare_at_minor, x.available, x.seen_at,
       x.low30, x.usual, x.tracked_since, x.lowest_tracked, x.last_available,
       CASE
           WHEN x.currency = x.store_currency THEN ''
           WHEN own.usual IS NULL OR own.usual = 0 THEN 'unknown'
           WHEN own.price_minor < own.usual THEN 'dropped'
           ELSE 'no-drop'
       END,
       now()
FROM calc x
LEFT JOIN own ON own.coffee_id = x.coffee_id AND own.weight_grams = x.weight_grams AND own.currency = x.store_currency
ON CONFLICT (offer_id) DO UPDATE
SET latest_price = EXCLUDED.latest_price,
    latest_compare_at = EXCLUDED.latest_compare_at,
    latest_available = EXCLUDED.latest_available,
    latest_seen_at = EXCLUDED.latest_seen_at,
    low30 = EXCLUDED.low30,
    usual = EXCLUDED.usual,
    tracked_since = EXCLUDED.tracked_since,
    lowest_tracked = EXCLUDED.lowest_tracked,
    last_available = EXCLUDED.last_available,
    own_price = EXCLUDED.own_price,
    refreshed_at = EXCLUDED.refreshed_at;

-- A finished lot leaves the sitemap 30 days after it was last in stock
-- (design decision K10); its page stays, with noindex, for anyone who
-- bookmarked it, and comes back if the roaster restocks it.
-- name: SitemapCoffees :many
SELECT c.slug, r.kind, max(o.last_seen_at)::timestamptz AS last_seen
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
JOIN offers o ON o.coffee_id = c.id AND o.market = $1 AND o.last_seen_at > now() - interval '48 hours'
JOIN offer_summaries s ON s.offer_id = o.id
WHERE s.last_available > now() - interval '30 days'
GROUP BY c.slug, r.kind
ORDER BY r.kind, c.slug;

-- Price history of a coffee's offers, oldest first, for the charts.
-- name: PriceHistory :many
SELECT offer_id, seen_at, price_minor
FROM price_points
WHERE offer_id = ANY(@offer_ids::bigint[]) AND seen_at > now() - interval '180 days'
ORDER BY offer_id, seen_at;

-- When each active roaster was last read without an error, for the health
-- endpoint. A roaster never read has a NULL last_seen.
-- name: RoasterFreshness :many
SELECT slug, last_read_at AS last_seen
FROM roasters
WHERE active
ORDER BY slug;

-- name: MarkRoasterRead :exec
UPDATE roasters SET last_read_at = now() WHERE slug = $1;

-- name: UpdateRoasterMeta :exec
UPDATE roasters
SET country = @country, currency = @currency, ships_to = @ships_to::text[], meta_checked_at = now(),
    shop_id = nullif(@shop_id::bigint, 0)
WHERE slug = @slug;

-- The currency a store presented to a market, if checked recently enough.
-- name: PresentedCurrency :one
SELECT rm.currency
FROM roaster_markets rm
JOIN roasters r ON r.id = rm.roaster_id
WHERE r.slug = @slug AND rm.market = @market AND rm.checked_at > now() - make_interval(days => @max_age_days::int);

-- name: SavePresentedCurrency :exec
INSERT INTO roaster_markets (roaster_id, market, currency, checked_at)
SELECT r.id, @market, @currency, now() FROM roasters r WHERE r.slug = @slug
ON CONFLICT (roaster_id, market) DO UPDATE
SET currency = EXCLUDED.currency, checked_at = EXCLUDED.checked_at;

-- The markets where a coffee has a current offer: its page exists there, so
-- its pages point at each other (hreflang) and the header links them.
-- name: CoffeeMarkets :many
SELECT DISTINCT market FROM offers
WHERE coffee_id = $1 AND last_seen_at > now() - interval '48 hours'
ORDER BY market;

-- Every active store with something in stock in a market today, with what its
-- card shows: coffees in stock, real drops (the listing's rule, own price
-- included), store sales, and the offer with the lowest price per gram (its
-- price and weight, formatted like a listing row). Most real drops first,
-- then the biggest lists.
-- name: ListBrands :many
SELECT r.id, r.slug, r.name, r.kind, r.country,
       count(DISTINCT c.id) AS coffees,
       count(DISTINCT c.id) FILTER (WHERE s.usual > s.latest_price AND s.own_price IN ('', 'dropped')) AS drops,
       count(DISTINCT c.id) FILTER (WHERE s.latest_compare_at IS NOT NULL) AS sales,
       (array_agg(s.latest_price ORDER BY s.latest_price::numeric / o.weight_grams, o.id))[1] AS best_price,
       (array_agg(o.weight_grams ORDER BY s.latest_price::numeric / o.weight_grams, o.id))[1] AS best_grams
FROM roasters r
JOIN coffees c ON c.roaster_id = r.id
JOIN offers o ON o.coffee_id = c.id AND o.market = @market AND o.last_seen_at > now() - interval '48 hours'
JOIN offer_summaries s ON s.offer_id = o.id
WHERE r.active AND s.latest_available
GROUP BY r.id
ORDER BY drops DESC, coffees DESC, r.name;

-- One store for its brand page: what /meta.json told us about it.
-- name: BrandBySlug :one
SELECT id, slug, name, base_url, kind, country, currency, ships_to
FROM roasters
WHERE slug = $1 AND active;

-- The markets where a store has something in stock today: its brand page
-- exists there, so those pages point at each other (hreflang).
-- name: BrandMarkets :many
SELECT DISTINCT o.market
FROM offers o
JOIN coffees c ON c.id = o.coffee_id
JOIN offer_summaries s ON s.offer_id = o.id
WHERE c.roaster_id = $1 AND o.last_seen_at > now() - interval '48 hours' AND s.latest_available
ORDER BY o.market;
