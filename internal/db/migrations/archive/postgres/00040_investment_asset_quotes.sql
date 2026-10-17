-- A dated price series per asset, so rendimento can be derived for any period
-- (V(to) − V(from) − net flows) instead of only from a provider-stated profit
-- or a complete movement history. financial_investments.value is overwritten
-- on every sync, so without this table a synced holding has no past price.
--
-- One price per asset per day, last writer wins; source records who wrote it
-- ('pluggy', 'issue' — the PU a fixed income title was bought at — or a
-- market data provider name such as 'brapi'). Which writer may replace which
-- is decided by origin, added in a later migration. Writers and the backfill
-- from raw_imports live in internal/investments/quotes.go.
--
-- issue_date/purchase_date/issuer_cnpj let a fixed income title without a
-- provider code get a synthetic one (investments.FixedIncomeCode): holdings
-- with the same issuer, rate, issue and due date share the same PU, so they
-- are one asset rather than every CDB of an issuer collapsing into one.

-- +goose Up

ALTER TABLE financial_investments ADD COLUMN issue_date TEXT;
ALTER TABLE financial_investments ADD COLUMN purchase_date TEXT;
ALTER TABLE financial_investments ADD COLUMN issuer_cnpj TEXT;

CREATE TABLE investment_asset_quotes (
    asset_id TEXT NOT NULL REFERENCES investment_assets (id) ON DELETE CASCADE,
    quoted_on TEXT NOT NULL,
    price TEXT NOT NULL,
    source TEXT NOT NULL,
    raw_import_id TEXT REFERENCES raw_imports (id),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (asset_id, quoted_on)
);

-- +goose Down

DROP TABLE investment_asset_quotes;
ALTER TABLE financial_investments DROP COLUMN issuer_cnpj;
ALTER TABLE financial_investments DROP COLUMN purchase_date;
ALTER TABLE financial_investments DROP COLUMN issue_date;
