-- +goose Up
-- The spike's stores: US roasters that ship within the US and were confirmed
-- as Shopify (Shopify /meta.json, 11 Oct 2026).
INSERT INTO roasters (slug, name, base_url) VALUES
    ('counter-culture', 'Counter Culture Coffee', 'https://counterculturecoffee.com'),
    ('intelligentsia', 'Intelligentsia', 'https://www.intelligentsia.com'),
    ('perc', 'PERC Coffee', 'https://perccoffee.com'),
    ('klatch', 'Klatch Coffee', 'https://www.klatchcoffee.com');

-- +goose Down
DELETE FROM roasters WHERE slug IN ('counter-culture', 'intelligentsia', 'perc', 'klatch');
