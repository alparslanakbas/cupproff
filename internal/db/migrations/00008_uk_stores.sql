-- +goose Up
-- UK roasters and tea houses for the en-gb market, confirmed on 13 Oct 2026
-- with one /meta.json request each from the server: Shopify, GB, GBP, ships
-- to GB. US stores that ship to the UK are not read there (they sell in USD,
-- and a bag sent from the US costs a UK buyer far more than its price).
--
-- Left out: Has Bean (hasbean.co.uk is the same Shopify shop as Ozone,
-- id 231572; adding both would list every coffee twice), Black Sheep
-- (blacksheepcoffee.com is its US store, USD), and the candidates that are
-- not Shopify or did not answer: Monmouth, Extract, Pact, Notes, Teapigs,
-- Postcard Teas, Whittard, Lalani, Comins, Mei Leaf, Bettys, Kandula.
INSERT INTO roasters (slug, name, base_url) VALUES
    ('square-mile', 'Square Mile Coffee Roasters', 'https://squaremilecoffee.com'),
    ('origin', 'Origin Coffee', 'https://origincoffee.co.uk'),
    ('union', 'Union Coffee', 'https://unionroasted.com'),
    ('colonna', 'Colonna Coffee', 'https://colonnacoffee.com'),
    ('volcano', 'Volcano Coffee Works', 'https://volcanocoffeeworks.com'),
    ('kiss-the-hippo', 'Kiss the Hippo Coffee', 'https://kissthehippo.com'),
    ('assembly', 'Assembly Coffee', 'https://assemblycoffee.co.uk'),
    ('workshop', 'Workshop Coffee', 'https://workshopcoffee.com'),
    ('caravan', 'Caravan Coffee Roasters', 'https://caravancoffeeroasters.co.uk'),
    ('climpson', 'Climpson & Sons', 'https://climpsonandsons.com'),
    ('dark-arts', 'Dark Arts Coffee', 'https://darkartscoffee.co.uk'),
    ('ozone', 'Ozone Coffee', 'https://ozonecoffee.co.uk'),
    ('round-hill', 'Round Hill Roastery', 'https://roundhillroastery.com'),
    ('crankhouse', 'Crankhouse Coffee', 'https://crankhousecoffee.co.uk'),
    ('clifton', 'Clifton Coffee Roasters', 'https://cliftoncoffee.co.uk'),
    ('rave', 'RAVE Coffee', 'https://ravecoffee.co.uk'),
    ('horsham', 'Horsham Coffee Roaster', 'https://horshamcoffeeroaster.co.uk'),
    ('dear-green', 'Dear Green Coffee', 'https://deargreencoffee.com'),
    ('steampunk', 'Steampunk Coffee Roasters', 'https://steampunkcoffee.co.uk'),
    ('artisan-roast', 'Artisan Roast Coffee Roasters', 'https://artisanroast.co.uk'),
    ('coaltown', 'Coaltown Coffee', 'https://coaltowncoffee.co.uk'),
    ('manumit', 'Manumit Coffee Roasters', 'https://manumitcoffee.co.uk'),
    ('rounton', 'Rounton Coffee', 'https://rountoncoffee.co.uk'),
    ('redemption', 'Redemption Roasters', 'https://redemptionroasters.com'),
    ('lost-sheep', 'Lost Sheep Coffee', 'https://lostsheepcoffee.com'),
    ('moon-roast', 'Moon Roast Coffee', 'https://moonroast.co.uk'),
    ('grind', 'Grind', 'https://grind.co.uk'),
    ('200-degrees', '200 Degrees Coffee', 'https://200degs.com'),
    ('quarter-horse', 'Quarter Horse Coffee', 'https://quarterhorsecoffee.com');

INSERT INTO roasters (slug, name, base_url, kind) VALUES
    ('bird-and-blend', 'Bird & Blend Tea Co.', 'https://birdandblendtea.com', 'tea'),
    ('jing', 'Jing Tea', 'https://jingtea.com', 'tea'),
    ('rare-tea', 'Rare Tea Company', 'https://rareteacompany.com', 'tea'),
    ('canton', 'Canton Tea', 'https://cantontea.com', 'tea'),
    ('good-and-proper', 'Good & Proper Tea', 'https://goodandpropertea.com', 'tea'),
    ('brew-tea', 'Brew Tea Company', 'https://brewteacompany.co.uk', 'tea'),
    ('joes-tea', 'Joe''s Tea Co.', 'https://joesteacompany.com', 'tea'),
    ('waterloo-tea', 'Waterloo Tea', 'https://waterlootea.com', 'tea'),
    ('hampstead-tea', 'Hampstead Tea', 'https://hampsteadtea.com', 'tea'),
    ('tregothnan', 'Tregothnan', 'https://tregothnan.co.uk', 'tea');

-- +goose Down
DELETE FROM roasters WHERE slug IN ('square-mile', 'origin', 'union', 'colonna', 'volcano', 'kiss-the-hippo',
    'assembly', 'workshop', 'caravan', 'climpson', 'dark-arts', 'ozone', 'round-hill', 'crankhouse', 'clifton',
    'rave', 'horsham', 'dear-green', 'steampunk', 'artisan-roast', 'coaltown', 'manumit', 'rounton', 'redemption',
    'lost-sheep', 'moon-roast', 'grind', '200-degrees', 'quarter-horse', 'bird-and-blend', 'jing', 'rare-tea',
    'canton', 'good-and-proper', 'brew-tea', 'joes-tea', 'waterloo-tea', 'hampstead-tea', 'tregothnan');
