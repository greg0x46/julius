-- +goose Up

-- Provider holdings remain immutable snapshots in financial_investments.
-- These tables hold the user's custody structure and a separate manual
-- ledger, so a local correction can never manufacture a source/raw-import
-- identity or overwrite a balance supplied by the provider.
CREATE TABLE investment_accounts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL CHECK (kind IN ('manual', 'integrated')),
    currency_code TEXT,
    source_id TEXT UNIQUE REFERENCES data_sources (id) ON DELETE CASCADE,
    financial_account_id TEXT UNIQUE REFERENCES financial_accounts (id) ON DELETE SET NULL,
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (kind = 'manual' AND source_id IS NULL AND currency_code = 'BRL') OR
        (kind = 'integrated' AND source_id IS NOT NULL)
    )
);

-- One persisted local account per already-known connection. New connections
-- are materialized lazily by investments.EnsureIntegratedAccounts; the id is
-- deterministic only for the local grouping row, never an imported id.
INSERT INTO investment_accounts (
    id, name, kind, currency_code, source_id, is_active, created_at, updated_at
)
SELECT
    'integrated:' || ds.id,
    COALESCE(NULLIF(ds.label, ''), NULLIF(ds.display_name, ''), ds.external_item_id),
    'integrated', NULL, ds.id, 1, MIN(fi.created_at), MAX(fi.updated_at)
FROM data_sources ds
JOIN financial_investments fi ON fi.source_id = ds.id
GROUP BY ds.id, ds.label, ds.display_name, ds.external_item_id;

CREATE TABLE investment_portfolios (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    target_amount TEXT,
    target_date TEXT,
    notes TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE investment_assets (
    id TEXT PRIMARY KEY,
    canonical_key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    ticker TEXT,
    asset_type TEXT NOT NULL,
    currency_code TEXT NOT NULL DEFAULT 'BRL',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- A manual position has no row in a provider table. Its quantities and cost
-- are replayed from investment_operations on every read.
CREATE TABLE investment_positions (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    asset_id TEXT NOT NULL REFERENCES investment_assets (id) ON DELETE RESTRICT,
    notes TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (account_id, asset_id)
);
CREATE INDEX ix_investment_positions_account_id ON investment_positions (account_id);
CREATE INDEX ix_investment_positions_asset_id ON investment_positions (asset_id);

-- One local portfolio annotation per holding, regardless of whether the
-- holding is manual or provider-synced. The exclusive shape protects the
-- source tables from local edits while still allowing a goal for any holding.
CREATE TABLE investment_position_portfolios (
    manual_position_id TEXT UNIQUE REFERENCES investment_positions (id) ON DELETE CASCADE,
    financial_investment_id TEXT UNIQUE REFERENCES financial_investments (id) ON DELETE CASCADE,
    portfolio_id TEXT NOT NULL REFERENCES investment_portfolios (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (manual_position_id IS NOT NULL AND financial_investment_id IS NULL) OR
        (manual_position_id IS NULL AND financial_investment_id IS NOT NULL)
    )
);
CREATE INDEX ix_investment_position_portfolios_portfolio_id ON investment_position_portfolios (portfolio_id);

CREATE TABLE investment_operations (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES investment_accounts (id) ON DELETE RESTRICT,
    position_id TEXT REFERENCES investment_positions (id) ON DELETE RESTRICT,
    kind TEXT NOT NULL CHECK (kind IN (
        'initial_balance', 'deposit', 'withdrawal', 'buy', 'sell', 'income', 'fee', 'tax', 'valuation',
        'transfer_out', 'transfer_in'
    )),
    occurred_on TEXT NOT NULL,
    amount TEXT NOT NULL,
    quantity TEXT,
    unit_price TEXT,
    fees TEXT NOT NULL DEFAULT '0',
    taxes TEXT NOT NULL DEFAULT '0',
    notes TEXT,
    source TEXT NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'synced')),
    transfer_id TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX ix_investment_operations_account_date ON investment_operations (account_id, occurred_on, created_at, id);
CREATE INDEX ix_investment_operations_position_date ON investment_operations (position_id, occurred_on, created_at, id);
CREATE INDEX ix_investment_operations_transfer_id ON investment_operations (transfer_id);

-- A source transaction can be split between operations up to its available
-- value. Partial uniqueness prevents accidentally entering the same source
-- link twice for one operation while still allowing a deliberate split.
CREATE TABLE investment_reconciliations (
    id TEXT PRIMARY KEY,
    operation_id TEXT NOT NULL REFERENCES investment_operations (id) ON DELETE CASCADE,
    financial_transaction_id TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    financial_investment_transaction_id TEXT REFERENCES financial_investment_transactions (id) ON DELETE CASCADE,
    amount TEXT NOT NULL,
    created_at TEXT NOT NULL,
    CHECK (financial_transaction_id IS NOT NULL OR financial_investment_transaction_id IS NOT NULL)
);
CREATE INDEX ix_investment_reconciliations_operation_id ON investment_reconciliations (operation_id);
CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_transaction
    ON investment_reconciliations (operation_id, financial_transaction_id)
    WHERE financial_transaction_id IS NOT NULL;
CREATE UNIQUE INDEX uq_investment_reconciliation_operation_financial_investment_transaction
    ON investment_reconciliations (operation_id, financial_investment_transaction_id)
    WHERE financial_investment_transaction_id IS NOT NULL;

-- +goose Down

DROP INDEX uq_investment_reconciliation_operation_financial_investment_transaction;
DROP INDEX uq_investment_reconciliation_operation_financial_transaction;
DROP INDEX ix_investment_reconciliations_operation_id;
DROP TABLE investment_reconciliations;
DROP INDEX ix_investment_operations_position_date;
DROP INDEX IF EXISTS ix_investment_operations_transfer_id;
DROP INDEX ix_investment_operations_account_date;
DROP TABLE investment_operations;
DROP INDEX ix_investment_position_portfolios_portfolio_id;
DROP TABLE investment_position_portfolios;
DROP INDEX ix_investment_positions_account_id;
DROP INDEX IF EXISTS ix_investment_positions_asset_id;
DROP TABLE investment_positions;
DROP TABLE IF EXISTS investment_assets;
DROP TABLE investment_portfolios;
DROP TABLE investment_accounts;
