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
       x.price_minor, x.compare_at_minor, x.low30, x.usual, x.tracked_since, x.lowest_tracked
FROM (
    SELECT DISTINCT ON (c.id)
        c.slug, c.title, c.image_url, r.name AS roaster_name,
        o.size_label, o.weight_grams, o.currency,
        s.latest_price AS price_minor, s.latest_compare_at AS compare_at_minor,
        s.low30, s.usual, s.tracked_since, s.lowest_tracked
    FROM coffees c
    JOIN roasters r ON r.id = c.roaster_id
    JOIN offers o ON o.coffee_id = c.id AND o.market = @market AND o.last_seen_at > now() - interval '48 hours'
    JOIN offer_summaries s ON s.offer_id = o.id
    WHERE s.latest_available AND r.kind = @kind
    ORDER BY c.id, o.weight_grams
) x
ORDER BY x.title, x.slug
LIMIT @page_size OFFSET @skip;

-- name: CountCoffees :one
SELECT count(DISTINCT o.coffee_id)
FROM offers o
JOIN offer_summaries s ON s.offer_id = o.id
JOIN coffees c ON c.id = o.coffee_id
JOIN roasters r ON r.id = c.roaster_id
WHERE o.market = @market AND o.last_seen_at > now() - interval '48 hours' AND s.latest_available AND r.kind = @kind;

-- name: CoffeeBySlug :one
SELECT c.id, c.slug, c.title, c.product_type, c.image_url, c.handle,
       r.name AS roaster_name, r.base_url AS roaster_url, r.kind
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
WHERE c.slug = $1;

-- Offers of one coffee in one market with their summaries.
-- name: OffersForCoffee :many
SELECT o.id, o.size_label, o.weight_grams, o.currency, o.variant_id,
       s.latest_price AS price_minor, s.latest_compare_at AS compare_at_minor,
       s.latest_available AS available, s.latest_seen_at AS seen_at,
       s.low30, s.usual, s.tracked_since, s.lowest_tracked, s.last_available
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
-- Prices are > 0 by CHECK, so 0 always means "none".
-- name: RefreshOfferSummaries :execrows
WITH o AS (
    SELECT id FROM offers WHERE last_seen_at > now() - interval '48 hours'
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
)
INSERT INTO offer_summaries (offer_id, latest_price, latest_compare_at, latest_available, latest_seen_at,
                             low30, usual, tracked_since, lowest_tracked, last_available, refreshed_at)
SELECT o.id, lp.price_minor, lp.compare_at_minor, lp.available, lp.seen_at,
       COALESCE(low.low30, 0), COALESCE(usual.usual, 0), COALESCE(seg.restart, alltime.first),
       COALESCE(CASE WHEN seg.restart IS NULL THEN alltime.low ELSE low.low30 END, 0),
       alltime.last_available, now()
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

-- When each active roaster was last read, for the health endpoint. A roaster
-- with no offers at all has a NULL last_seen.
-- name: RoasterFreshness :many
SELECT r.slug, max(o.last_seen_at)::timestamptz AS last_seen
FROM roasters r
LEFT JOIN coffees c ON c.roaster_id = r.id
LEFT JOIN offers o ON o.coffee_id = c.id
WHERE r.active
GROUP BY r.slug
ORDER BY r.slug;

-- name: UpdateRoasterMeta :exec
UPDATE roasters
SET country = @country, currency = @currency, ships_to = @ships_to::text[], meta_checked_at = now()
WHERE slug = @slug;
