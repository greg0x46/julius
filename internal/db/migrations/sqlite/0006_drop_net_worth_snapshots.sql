-- +goose Up
-- Wealth is derived on read from the ledgers and price series; a stored
-- snapshot could only go stale. The history is rebuilt, not migrated.
DROP TABLE net_worth_snapshots;

-- +goose Down
-- Restores the table as migrations 0003 and 0004 left it, so their own Down
-- steps still apply.
CREATE TABLE net_worth_snapshots (
    id                    TEXT NOT NULL PRIMARY KEY,
    captured_at           TEXT NOT NULL UNIQUE,
    total_assets          TEXT NOT NULL,
    total_liabilities     TEXT NOT NULL,
    net_worth             TEXT NOT NULL,
    cash_balance          TEXT NOT NULL,
    investment_balance    TEXT NOT NULL,
    credit_card_balance   TEXT NOT NULL,
    payables_debt         TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
, is_backfilled INTEGER NOT NULL DEFAULT 0 CHECK (is_backfilled IN (0, 1)),
    valuation_currency    TEXT NOT NULL DEFAULT 'BRL' CHECK (valuation_currency = 'BRL'),
    valuation_version     INTEGER NOT NULL DEFAULT 0 CHECK (valuation_version >= 0),
    valuation_status      TEXT NOT NULL DEFAULT 'legacy' CHECK (valuation_status IN ('legacy', 'complete', 'stale', 'reconstructed', 'unavailable')),
    valued_at             TEXT,
    rate_policy           TEXT,
    valuation_rates       TEXT NOT NULL DEFAULT '{}');
