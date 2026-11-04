-- +goose Up
-- A price a store shows in another currency than its own (Shopify Markets
-- converts it at the day's rate) moves with the exchange rate when the store
-- changes nothing. Measured 4 Nov 2026: of 1,703 converted prices under their
-- usual price, 1,689 had no drop in the store's own currency (Bocca's 1 kg
-- bag: EUR 39.40 every day, USD 45 and 46 by turns). own_price records, for
-- such an offer, whether the same size's price in the store's own currency
-- dropped too ('dropped'), didn't ('no-drop') or can't be compared
-- ('unknown': not read in that currency, or too new); '' for an offer in the
-- store's own currency. Only '' and 'dropped' may show a drop. The summary
-- refresh fills it; rows written before it runs default to 'unknown', so an
-- unchecked converted price never shows a drop.
ALTER TABLE offer_summaries
    ADD COLUMN own_price text NOT NULL DEFAULT 'unknown'
        CHECK (own_price IN ('', 'dropped', 'no-drop', 'unknown'));

-- +goose Down
ALTER TABLE offer_summaries DROP COLUMN own_price;
