-- Which writer may replace which price in investment_asset_quotes was decided
-- by listing source names in SQL ('pluggy', 'issue') and in Go
-- (marketdata.IsProvider), so adding a writer meant finding every list. origin
-- is the kind of statement a price makes, and precedence is stated on it:
--
--   sync    the Open Finance provider reported it on a sync;
--   issue   derived from a fixed income title's purchase (the PU it was
--           bought at), not observed;
--   market  a market data provider's spot or close.
--
-- A market price replaces only another market price, never one the provider
-- observed or one derived from the title; the provider's replaces any.
-- source stays the name of who wrote the row ('pluggy', 'issue', a provider
-- such as 'yahoo'), for display and audit only.
--
-- Existing rows are classified by their source. Anything that is neither
-- Pluggy's nor an issue price (a provider name, or the 'manual' that the
-- table's own migration lists but nothing writes) is a market price.

-- +goose Up

ALTER TABLE investment_asset_quotes ADD COLUMN origin TEXT NOT NULL DEFAULT 'market'
    CHECK (origin IN ('sync', 'issue', 'market'));

UPDATE investment_asset_quotes SET origin = CASE source
    WHEN 'pluggy' THEN 'sync'
    WHEN 'issue' THEN 'issue'
    ELSE 'market'
END;

-- +goose Down

ALTER TABLE investment_asset_quotes DROP COLUMN origin;
