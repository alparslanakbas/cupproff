-- name: ListActiveRoasters :many
SELECT id, slug, name, base_url, platform
FROM roasters
WHERE active
ORDER BY slug;

-- name: RoasterBySlug :one
SELECT id, slug, name, base_url, platform
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
-- stale and never shown.
-- name: ListCoffees :many
SELECT x.slug, x.title, x.image_url, x.roaster_name, x.size_label, x.weight_grams,
       x.currency, x.price_minor, x.compare_at_minor, x.available
FROM (
    SELECT DISTINCT ON (c.id)
        c.slug, c.title, c.image_url, r.name AS roaster_name,
        o.size_label, o.weight_grams, o.currency,
        lp.price_minor, lp.compare_at_minor, lp.available
    FROM coffees c
    JOIN roasters r ON r.id = c.roaster_id
    JOIN offers o ON o.coffee_id = c.id AND o.market = @market AND o.last_seen_at > now() - interval '48 hours'
    JOIN LATERAL (
        SELECT p.price_minor, p.compare_at_minor, p.available
        FROM price_points p
        WHERE p.offer_id = o.id
        ORDER BY p.seen_at DESC
        LIMIT 1
    ) lp ON true
    WHERE lp.available
    ORDER BY c.id, o.weight_grams
) x
ORDER BY x.title, x.slug
LIMIT @page_size OFFSET @skip;

-- name: CountCoffees :one
SELECT count(DISTINCT o.coffee_id)
FROM offers o
JOIN LATERAL (
    SELECT p.available
    FROM price_points p
    WHERE p.offer_id = o.id
    ORDER BY p.seen_at DESC
    LIMIT 1
) lp ON true
WHERE o.market = $1 AND o.last_seen_at > now() - interval '48 hours' AND lp.available;

-- name: CoffeeBySlug :one
SELECT c.id, c.slug, c.title, c.product_type, c.image_url, c.handle,
       r.name AS roaster_name, r.base_url AS roaster_url
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
WHERE c.slug = $1;

-- Offers of one coffee in one market with everything the verdict needs.
-- The rules are the sister sites' (TR PriceSummaryRefresher, TakipDibi):
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
-- name: OffersForCoffee :many
WITH o AS (
    SELECT id, size_label, weight_grams, currency, variant_id
    FROM offers
    WHERE coffee_id = @coffee_id AND market = @market AND last_seen_at > now() - interval '48 hours'
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
    SELECT offer_id, min(price_minor) AS low30
    FROM win
    GROUP BY offer_id
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
SELECT o.id, o.size_label, o.weight_grams, o.currency, o.variant_id,
       lp.price_minor, lp.compare_at_minor, lp.available, lp.seen_at,
       COALESCE(low.low30, 0)::bigint AS low30,
       COALESCE(usual.usual, 0)::bigint AS usual,
       COALESCE(seg.restart, alltime.first)::timestamptz AS tracked_since,
       COALESCE(CASE WHEN seg.restart IS NULL THEN alltime.low ELSE low.low30 END, 0)::bigint AS lowest_tracked,
       alltime.last_available::timestamptz AS last_available
FROM o
JOIN LATERAL (
    SELECT p.price_minor, p.compare_at_minor, p.available, p.seen_at
    FROM price_points p
    WHERE p.offer_id = o.id
    ORDER BY p.seen_at DESC
    LIMIT 1
) lp ON true
LEFT JOIN seg ON seg.offer_id = o.id
LEFT JOIN low ON low.offer_id = o.id
LEFT JOIN usual ON usual.offer_id = o.id
LEFT JOIN alltime ON alltime.offer_id = o.id
ORDER BY o.weight_grams;

-- A finished lot leaves the sitemap 30 days after it was last in stock
-- (design decision K10); its page stays, with noindex, for anyone who
-- bookmarked it, and comes back if the roaster restocks it.
-- name: SitemapCoffees :many
SELECT c.slug, max(o.last_seen_at)::timestamptz AS last_seen
FROM coffees c
JOIN offers o ON o.coffee_id = c.id AND o.market = $1 AND o.last_seen_at > now() - interval '48 hours'
WHERE EXISTS (
    SELECT 1 FROM price_points p
    WHERE p.offer_id = o.id AND p.available AND p.seen_at > now() - interval '30 days'
)
GROUP BY c.slug
ORDER BY c.slug;

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
