-- +goose Up
-- Derived data, refreshed after every scrape by RefreshOfferSummaries:
-- price_points stay the only source of truth, and this table can always be
-- rebuilt from them. It exists so a listing can show and filter by verdicts
-- (real drop, 30-day low, lowest since) without running the window rules
-- for every card, and so the rules live in ONE query, not in each page's.
CREATE TABLE offer_summaries (
    offer_id          bigint PRIMARY KEY REFERENCES offers (id) ON DELETE CASCADE,
    latest_price      bigint NOT NULL,
    latest_compare_at bigint,
    latest_available  boolean NOT NULL,
    latest_seen_at    timestamptz NOT NULL,
    low30             bigint NOT NULL, -- 0 = none
    usual             bigint NOT NULL, -- 0 = none yet
    tracked_since     timestamptz NOT NULL,
    lowest_tracked    bigint NOT NULL,
    last_available    timestamptz,     -- NULL = never seen in stock
    refreshed_at      timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE offer_summaries;
