-- +goose Up
-- When the scraper last read a store without an error. The health check
-- used the newest offer instead, and some stores never have one by design:
-- Coffee Collective and Fuglen show no market's currency, Paper & Tea and
-- Joe's Tea state no weights (14 Oct 2026). They stayed "stale" forever and
-- would have kept the uptime monitor red; a failed read is what the check
-- is for.
ALTER TABLE roasters ADD COLUMN last_read_at timestamptz;

-- Stores read before this column existed: their newest offer is when they
-- were last read. The others get a value on their next read.
UPDATE roasters r SET last_read_at = x.seen
FROM (SELECT c.roaster_id, max(o.last_seen_at) AS seen
      FROM coffees c JOIN offers o ON o.coffee_id = c.id GROUP BY c.roaster_id) x
WHERE x.roaster_id = r.id;

-- +goose Down
ALTER TABLE roasters DROP COLUMN last_read_at;
