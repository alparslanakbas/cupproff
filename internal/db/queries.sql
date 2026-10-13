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

-- Market listing: one card per coffee, showing its smallest bag that was
-- seen in the last 48 hours (older offers are stale, not current prices).
-- name: ListCoffees :many
SELECT DISTINCT ON (c.id)
    c.slug, c.title, c.image_url, r.name AS roaster_name,
    o.size_label, o.weight_grams, o.currency,
    lp.price_minor, lp.compare_at_minor, lp.available
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
JOIN offers o ON o.coffee_id = c.id AND o.market = $1 AND o.last_seen_at > now() - interval '48 hours'
JOIN LATERAL (
    SELECT p.price_minor, p.compare_at_minor, p.available
    FROM price_points p
    WHERE p.offer_id = o.id
    ORDER BY p.seen_at DESC
    LIMIT 1
) lp ON true
ORDER BY c.id, o.weight_grams;

-- name: CoffeeBySlug :one
SELECT c.id, c.slug, c.title, c.product_type, c.image_url, c.handle,
       r.name AS roaster_name, r.base_url AS roaster_url
FROM coffees c
JOIN roasters r ON r.id = c.roaster_id
WHERE c.slug = $1;

-- Offers of one coffee in one market with their latest price, the lowest
-- price of the last 30 days, and the usual price: the highest price seen on
-- at least 7 different UTC days in the last 30 days (the rule both sister
-- sites use; a store's own "compare at" price is shown, never trusted).
-- name: OffersForCoffee :many
SELECT o.id, o.size_label, o.weight_grams, o.currency, o.variant_id,
       lp.price_minor, lp.compare_at_minor, lp.available, lp.seen_at,
       -- 0 means "none yet": prices are > 0 by CHECK, so 0 is never a real price.
       COALESCE(s.low30, 0)::bigint AS low30,
       COALESCE(u.usual, 0)::bigint AS usual
FROM offers o
JOIN LATERAL (
    SELECT p.price_minor, p.compare_at_minor, p.available, p.seen_at
    FROM price_points p
    WHERE p.offer_id = o.id
    ORDER BY p.seen_at DESC
    LIMIT 1
) lp ON true
JOIN LATERAL (
    SELECT min(p.price_minor) AS low30
    FROM price_points p
    WHERE p.offer_id = o.id AND p.seen_at > now() - interval '30 days'
) s ON true
LEFT JOIN LATERAL (
    SELECT max(d.price_minor) AS usual
    FROM (
        SELECT p.price_minor
        FROM price_points p
        WHERE p.offer_id = o.id AND p.seen_at > now() - interval '30 days'
        GROUP BY p.price_minor
        HAVING count(DISTINCT (p.seen_at AT TIME ZONE 'UTC')::date) >= 7
    ) d
) u ON true
WHERE o.coffee_id = $1 AND o.market = $2 AND o.last_seen_at > now() - interval '48 hours'
ORDER BY o.weight_grams;

-- name: SitemapCoffees :many
SELECT c.slug, max(o.last_seen_at)::timestamptz AS last_seen
FROM coffees c
JOIN offers o ON o.coffee_id = c.id AND o.market = $1 AND o.last_seen_at > now() - interval '48 hours'
GROUP BY c.slug
ORDER BY c.slug;
