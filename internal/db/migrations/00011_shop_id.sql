-- +goose Up
-- The Shopify shop id from /meta.json. One shop can answer on two domains:
-- hasbean.co.uk and ozonecoffee.co.uk are the same shop (id 231572, found
-- on 13 Oct 2026 by hand), and adding both would list every coffee twice.
-- The unique index makes the second domain's meta update fail on its first
-- read ("duplicate key ... roasters_shop_id_key"), so the run's log and exit
-- code show it and its products are never stored.
ALTER TABLE roasters ADD COLUMN shop_id bigint;
CREATE UNIQUE INDEX roasters_shop_id_key ON roasters (shop_id) WHERE shop_id IS NOT NULL;

-- +goose Down
DROP INDEX roasters_shop_id_key;
ALTER TABLE roasters DROP COLUMN shop_id;
