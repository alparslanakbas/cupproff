-- +goose Up
-- EU roasters and tea houses for the en-eu market, checked on 14 Oct 2026
-- with one /meta.json request each from the server: Shopify, ships to
-- Germany. Most sell in EUR. The Nordic ones sell in NOK, DKK or SEK; the
-- scraper reads them for the EU only when their home page shows a German
-- buyer EUR (shopify.PresentedCurrency), so adding them costs nothing when
-- they don't.
--
-- Left out (not Shopify, or no answer): Coffee Circle (HTTP 500), Blank
-- Roast, Quijote, Wildkaffee, Supremo, Machhörndl, Friedhats, Manhattan,
-- Lot Sixty One, Stooker, Terres de Café, Gardelli (shop subdomain, no
-- meta.json), April, Lov Organic.
INSERT INTO roasters (slug, name, base_url) VALUES
    ('the-barn', 'The Barn Coffee Roasters', 'https://thebarn.de'),
    ('bonanza', 'Bonanza Coffee Roasters', 'https://bonanzacoffee.de'),
    ('five-elephant', 'Five Elephant', 'https://fiveelephant.com'),
    ('hoppenworth-ploch', 'Hoppenworth & Ploch', 'https://hoppenworth-ploch.de'),
    ('19grams', '19grams', 'https://19grams.coffee'),
    ('elbgold', 'Elbgold', 'https://elbgold.com'),
    ('man-vs-machine', 'Man Versus Machine Coffee Roasters', 'https://mvsm.coffee'),
    ('bocca', 'Bocca Coffee', 'https://bocca.nl'),
    ('belleville', 'Belleville Brûlerie', 'https://cafesbelleville.com'),
    ('coutume', 'Coutume Café', 'https://coutumecafe.com'),
    ('ditta-artigianale', 'Ditta Artigianale', 'https://dittaartigianale.it'),
    ('nomad', 'Nomad Coffee', 'https://nomadcoffee.es'),
    ('right-side', 'Right Side Coffee', 'https://rightsidecoffee.com'),
    ('hola', 'Hola Coffee Roasters', 'https://hola.coffee'),
    ('syra', 'Syra Coffee', 'https://syra.coffee'),
    ('3fe', '3fe', 'https://3fe.com'),
    ('kaffa', 'Kaffa Roastery', 'https://kaffaroastery.fi'),
    ('good-life', 'Good Life Coffee', 'https://goodlifecoffee.fi'),
    ('koppi', 'Koppi Roasters', 'https://koppi.se'),
    ('tim-wendelboe', 'Tim Wendelboe', 'https://timwendelboe.no'),
    ('la-cabra', 'La Cabra', 'https://lacabra.dk'),
    ('coffee-collective', 'Coffee Collective', 'https://coffeecollective.dk'),
    ('drop', 'Drop Coffee Roasters', 'https://dropcoffee.com'),
    ('prolog', 'Prolog Coffee', 'https://prologcoffee.com'),
    ('fuglen', 'Fuglen Coffee Roasters', 'https://fuglencoffee.no'),
    ('morgon', 'Morgon Coffee Roasters', 'https://morgoncoffeeroasters.com');

INSERT INTO roasters (slug, name, base_url, kind) VALUES
    ('paper-and-tea', 'Paper & Tea', 'https://paperandtea.com', 'tea'),
    ('kusmi', 'Kusmi Tea', 'https://kusmitea.com', 'tea'),
    ('nannuoshan', 'Nannuoshan', 'https://nannuoshan.org', 'tea');

-- +goose Down
DELETE FROM roasters WHERE slug IN ('the-barn', 'bonanza', 'five-elephant', 'hoppenworth-ploch', '19grams',
    'elbgold', 'man-vs-machine', 'bocca', 'belleville', 'coutume', 'ditta-artigianale', 'nomad', 'right-side',
    'hola', 'syra', '3fe', 'kaffa', 'good-life', 'koppi', 'tim-wendelboe', 'la-cabra', 'coffee-collective', 'drop',
    'prolog', 'fuglen', 'morgon', 'paper-and-tea', 'kusmi', 'nannuoshan');
