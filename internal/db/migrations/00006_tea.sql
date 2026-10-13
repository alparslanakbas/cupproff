-- +goose Up
-- Traditional tea next to coffee. A store sells one kind and every product
-- it lists is read as that kind (shopify.Kind); the tables keep their coffee
-- names on purpose: renaming them would touch some 270 references for no
-- visible change, so "coffees" holds teas too and the store's kind tells
-- them apart.
ALTER TABLE roasters
    ADD COLUMN kind text NOT NULL DEFAULT 'coffee' CHECK (kind IN ('coffee', 'tea')),
    -- A regular expression a size's text must match, for a store whose
    -- catalog no title or type check can sort (shopify.Store). NULL = none.
    ADD COLUMN variant_must_match text;

-- US tea houses confirmed as Shopify on 13 Oct 2026, one /products.json
-- request each. Left out: Tea Forte (3 of 250 products state a weight, the
-- rest are sachet counts, so no per-100 g price), Sugimoto (404), Verdant
-- (not Shopify).
-- Harney: every tea size reads "Loose / 2 oz. Tin"; its canned drinks, jerky
-- and coffee state weights but never "Loose" (84 of 84 against 0 of 38).
INSERT INTO roasters (slug, name, base_url, kind, variant_must_match) VALUES
    ('harney', 'Harney & Sons', 'https://harney.com', 'tea', '(?i)\bloose\b');

INSERT INTO roasters (slug, name, base_url, kind) VALUES
    ('rishi', 'Rishi Tea & Botanicals', 'https://www.rishi-tea.com', 'tea'),
    ('song-tea', 'Song Tea & Ceramics', 'https://songtea.com', 'tea'),
    ('yunnan-sourcing', 'Yunnan Sourcing', 'https://yunnansourcing.com', 'tea'),
    ('ippodo', 'Ippodo Tea', 'https://ippodotea.com', 'tea'),
    ('spirit-tea', 'Spirit Tea', 'https://spirittea.co', 'tea'),
    ('smith-teamaker', 'Smith Teamaker', 'https://smithtea.com', 'tea'),
    ('in-pursuit-of-tea', 'In Pursuit of Tea', 'https://inpursuitoftea.com', 'tea'),
    ('bellocq', 'Bellocq', 'https://bellocq.com', 'tea'),
    ('red-blossom', 'Red Blossom Tea Company', 'https://redblossomtea.com', 'tea'),
    ('floating-leaves', 'Floating Leaves Tea', 'https://floatingleavestea.com', 'tea'),
    ('te-company', 'Té Company', 'https://tecompanytea.com', 'tea');

-- +goose Down
DELETE FROM roasters WHERE kind = 'tea';
ALTER TABLE roasters DROP COLUMN kind, DROP COLUMN variant_must_match;
