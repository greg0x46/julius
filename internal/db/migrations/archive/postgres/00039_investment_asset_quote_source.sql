-- Manual positions (assets no Open Banking connection ever reports, such as
-- Bitcoin, or B3 tickers held outside a synced connection) have no provider
-- quote to fall back on. quote_source/quote_symbol optionally point a manual
-- asset at a market (b3, crypto) and the symbol it trades under there, so a
-- scheduled job (internal/quotes, through internal/marketdata) can price it
-- automatically; both stay NULL for an asset priced only through manual
-- valuations, which remains fully supported.
--
-- No CHECK pairing the two columns, to stay identical to the SQLite side of
-- this migration. "Both set or both empty" is enforced in Go instead, in
-- internal/investments/assets.go's normalizeAssetInput.

-- +goose Up

ALTER TABLE investment_assets ADD COLUMN quote_source TEXT;
ALTER TABLE investment_assets ADD COLUMN quote_symbol TEXT;

-- +goose Down

ALTER TABLE investment_assets DROP COLUMN quote_symbol;
ALTER TABLE investment_assets DROP COLUMN quote_source;
