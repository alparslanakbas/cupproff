-- +goose Up
-- What each store says about itself in Shopify's /meta.json, read before its
-- catalog on every run: where it ships and its default currency. A market
-- is read only when the store ships there (design §15). The empty-array
-- default is deliberate and explicit: "not read yet" means no market is
-- read, never "ships everywhere".
ALTER TABLE roasters
    ADD COLUMN country text,
    ADD COLUMN currency text,
    ADD COLUMN ships_to text[] NOT NULL DEFAULT '{}',
    ADD COLUMN meta_checked_at timestamptz;

-- +goose Down
ALTER TABLE roasters
    DROP COLUMN country,
    DROP COLUMN currency,
    DROP COLUMN ships_to,
    DROP COLUMN meta_checked_at;
