-- +goose Up
-- Which currency a store shows a buyer in a market's country, when its own
-- default currency is another one (Shopify Markets: PERC, a US store, shows
-- EUR to Germany; Klatch shows USD). products.json prices carry no
-- currency, so a catalog is read for such a market only when this says the
-- store presents the market's currency. Checked at most weekly (one
-- /cart.js request per store and market), not on every daily run.
CREATE TABLE roaster_markets (
    roaster_id bigint NOT NULL REFERENCES roasters (id) ON DELETE CASCADE,
    market     text NOT NULL,
    currency   text NOT NULL,
    checked_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (roaster_id, market)
);

-- +goose Down
DROP TABLE roaster_markets;
