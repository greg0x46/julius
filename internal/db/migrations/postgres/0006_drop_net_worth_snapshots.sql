-- +goose Up
-- Wealth is derived on read from the ledgers and price series; a stored
-- snapshot could only go stale. The history is rebuilt, not migrated.
DROP TABLE net_worth_snapshots;

-- +goose Down
-- Restores the table as migrations 0003 and 0004 left it, so their own Down
-- steps still apply.
CREATE TABLE net_worth_snapshots (
    id                    UUID NOT NULL PRIMARY KEY,
    captured_at           DATE NOT NULL UNIQUE,
    total_assets          NUMERIC NOT NULL,
    total_liabilities     NUMERIC NOT NULL,
    net_worth             NUMERIC NOT NULL,
    cash_balance          NUMERIC NOT NULL,
    investment_balance    NUMERIC NOT NULL,
    credit_card_balance   NUMERIC NOT NULL,
    payables_debt         NUMERIC NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL,
    updated_at            TIMESTAMPTZ NOT NULL
, is_backfilled BOOLEAN NOT NULL DEFAULT FALSE,
    valuation_currency    TEXT NOT NULL DEFAULT 'BRL' CHECK (valuation_currency = 'BRL'),
    valuation_version     INTEGER NOT NULL DEFAULT 0 CHECK (valuation_version >= 0),
    valuation_status      TEXT NOT NULL DEFAULT 'legacy' CHECK (valuation_status IN ('legacy', 'complete', 'stale', 'reconstructed', 'unavailable')),
    valued_at             TIMESTAMPTZ,
    rate_policy           TEXT,
    valuation_rates       TEXT NOT NULL DEFAULT '{}',
    CHECK (total_assets NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (total_liabilities NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (net_worth NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (cash_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (investment_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (credit_card_balance NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (payables_debt NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);
