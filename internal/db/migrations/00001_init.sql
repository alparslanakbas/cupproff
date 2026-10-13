-- +goose Up

-- A roaster is a store we read. Platform says which collector reads it.
CREATE TABLE roasters (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug       text NOT NULL UNIQUE,
    name       text NOT NULL,
    base_url   text NOT NULL,
    platform   text NOT NULL DEFAULT 'shopify',
    active     boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- A coffee is one product of a roaster; market-independent.
CREATE TABLE coffees (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    roaster_id   bigint NOT NULL REFERENCES roasters (id),
    handle       text NOT NULL,
    slug         text NOT NULL UNIQUE,
    title        text NOT NULL,
    product_type text NOT NULL DEFAULT '',
    image_url    text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (roaster_id, handle)
);

-- An offer is one bag size of a coffee in one market. Price is per market
-- because stores price per country (rounded local prices, VAT), never
-- converted with our own rate.
CREATE TABLE offers (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    coffee_id     bigint NOT NULL REFERENCES coffees (id),
    market        text NOT NULL,
    weight_grams  integer NOT NULL CHECK (weight_grams > 0),
    size_label    text NOT NULL,
    currency      text NOT NULL,
    variant_id    bigint NOT NULL,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (coffee_id, market, weight_grams)
);

-- One row per offer per ingest. Money is in minor units (cents), never float.
-- The primary key doubles as the "latest price" index: (offer_id, seen_at DESC)
-- lookups stay on one offer instead of windowing over all history.
CREATE TABLE price_points (
    offer_id         bigint NOT NULL REFERENCES offers (id),
    seen_at          timestamptz NOT NULL,
    price_minor      bigint NOT NULL CHECK (price_minor > 0),
    compare_at_minor bigint,
    available        boolean NOT NULL,
    PRIMARY KEY (offer_id, seen_at)
);

-- +goose Down
DROP TABLE price_points;
DROP TABLE offers;
DROP TABLE coffees;
DROP TABLE roasters;
