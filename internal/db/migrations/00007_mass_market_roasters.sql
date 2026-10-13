-- +goose Up
-- The two mass-market brands left out of 00003 as a product decision,
-- added on 13 Oct 2026: their names are what many US buyers search for.
-- Both are Shopify, US/USD and ship to the US (/meta.json, one request
-- each); Black Rifle answers on www only (the bare host redirects).
INSERT INTO roasters (slug, name, base_url) VALUES
    ('community-coffee', 'Community Coffee', 'https://communitycoffee.com'),
    ('black-rifle', 'Black Rifle Coffee Company', 'https://www.blackriflecoffee.com');

-- +goose Down
DELETE FROM roasters WHERE slug IN ('community-coffee', 'black-rifle');
