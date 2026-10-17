-- Which days of an asset's price series have already been asked of a market data
-- provider. Without it a gap in investment_asset_quotes is ambiguous: a
-- weekend or a holiday has no price, and so does a day nobody ever asked
-- about, and the two need opposite handling (leave it alone vs. fetch it).
--
-- One contiguous interval per asset: backfill fetches history up to the day
-- before today and the daily run extends it, so the covered range only ever
-- grows at its two ends. source/symbol record what the interval was fetched
-- with; changing either on the asset makes the stored interval stale, and
-- internal/quotes then ignores it and fetches the history again.
--
-- Written only by internal/quotes' backfill (via internal/investments).

-- +goose Up

CREATE TABLE investment_asset_quote_coverage (
    asset_id TEXT PRIMARY KEY REFERENCES investment_assets (id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    symbol TEXT NOT NULL,
    covered_from TEXT NOT NULL,
    covered_to TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE investment_asset_quote_coverage;
