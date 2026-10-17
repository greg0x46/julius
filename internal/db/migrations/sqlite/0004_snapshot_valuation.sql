-- +goose Up
-- Existing snapshots keep version 0 / 'legacy': their totals only ever covered
-- BRL accounts and holdings, with foreign balances left out.
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_version INTEGER NOT NULL DEFAULT 0 CHECK (valuation_version >= 0);
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_status TEXT NOT NULL DEFAULT 'legacy' CHECK (valuation_status IN ('legacy', 'complete', 'stale', 'reconstructed', 'unavailable'));
ALTER TABLE net_worth_snapshots ADD COLUMN valued_at TEXT;
ALTER TABLE net_worth_snapshots ADD COLUMN rate_policy TEXT;
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_rates TEXT NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_rates;
ALTER TABLE net_worth_snapshots DROP COLUMN rate_policy;
ALTER TABLE net_worth_snapshots DROP COLUMN valued_at;
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_status;
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_version;
