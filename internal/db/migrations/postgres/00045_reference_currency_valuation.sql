-- +goose Up
CREATE TABLE fx_rates (
    currency_code TEXT NOT NULL,
    reference_currency TEXT NOT NULL,
    observed_at TEXT NOT NULL,
    rate TEXT NOT NULL,
    source TEXT NOT NULL,
    PRIMARY KEY (currency_code, reference_currency, observed_at)
);

ALTER TABLE net_worth_snapshots ADD COLUMN reference_currency TEXT;
ALTER TABLE net_worth_snapshots ADD COLUMN valued_at TEXT;
ALTER TABLE net_worth_snapshots ADD COLUMN rate_policy TEXT;
ALTER TABLE net_worth_snapshots ADD COLUMN fx_rates TEXT NOT NULL DEFAULT '{}';
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_version INTEGER NOT NULL DEFAULT 0;
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_status TEXT NOT NULL DEFAULT 'legacy_unknown';

-- +goose Down
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_status;
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_version;
ALTER TABLE net_worth_snapshots DROP COLUMN fx_rates;
ALTER TABLE net_worth_snapshots DROP COLUMN rate_policy;
ALTER TABLE net_worth_snapshots DROP COLUMN valued_at;
ALTER TABLE net_worth_snapshots DROP COLUMN reference_currency;
DROP TABLE fx_rates;
