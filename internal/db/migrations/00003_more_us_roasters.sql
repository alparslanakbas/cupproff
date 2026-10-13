-- +goose Up
-- US specialty roasters confirmed as Shopify on 13 Oct 2026: one
-- /products.json request each, with the vendor field matching the roaster.
-- Not Shopify (404): Ritual, Coava. Left out for now (mass-market brands, a
-- product decision): Community Coffee, Black Rifle.
INSERT INTO roasters (slug, name, base_url) VALUES
    ('onyx', 'Onyx Coffee Lab', 'https://onyxcoffeelab.com'),
    ('verve', 'Verve Coffee Roasters', 'https://vervecoffee.com'),
    ('stumptown', 'Stumptown Coffee Roasters', 'https://stumptowncoffee.com'),
    ('la-colombe', 'La Colombe', 'https://lacolombe.com'),
    ('sey', 'SEY Coffee', 'https://seycoffee.com'),
    ('heart', 'Heart Coffee Roasters', 'https://heartroasters.com'),
    ('equator', 'Equator Coffees', 'https://equatorcoffees.com'),
    ('madcap', 'Madcap Coffee', 'https://madcapcoffee.com'),
    ('passenger', 'Passenger Coffee', 'https://passengercoffee.com'),
    ('peace', 'Peace Coffee', 'https://peacecoffee.com'),
    ('big-island', 'Big Island Coffee Roasters', 'https://bigislandcoffeeroasters.com'),
    ('brooklyn-roasting', 'Brooklyn Roasting Company', 'https://brooklynroasting.com'),
    ('nguyen', 'Nguyen Coffee Supply', 'https://nguyencoffeesupply.com'),
    ('methodical', 'Methodical Coffee', 'https://methodicalcoffee.com'),
    ('george-howell', 'George Howell Coffee', 'https://georgehowellcoffee.com'),
    ('sightglass', 'Sightglass Coffee', 'https://sightglasscoffee.com');

-- +goose Down
DELETE FROM roasters WHERE slug IN ('onyx', 'verve', 'stumptown', 'la-colombe', 'sey', 'heart', 'equator',
    'madcap', 'passenger', 'peace', 'big-island', 'brooklyn-roasting', 'nguyen', 'methodical', 'george-howell', 'sightglass');
