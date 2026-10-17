-- +goose Up

-- Credit card bills (faturas), mirroring financial_investments: same
-- current_raw_import_id + normalized_hash change-detection pattern. Only
-- synced for accounts with account_type = 'CREDIT' (see
-- syncsvc.processBills) — Pluggy only exposes closed/overdue bills, not the
-- currently open one.
CREATE TABLE financial_bills (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    external_id TEXT NOT NULL,
    due_date TEXT,
    closing_date TEXT,
    total_amount TEXT,
    currency_code TEXT,
    minimum_payment_amount TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE INDEX ix_financial_bills_account_id ON financial_bills (account_id);

ALTER TABLE sync_failures ADD COLUMN bill_id TEXT;
ALTER TABLE sync_failures ADD COLUMN external_bill_id TEXT;
ALTER TABLE sync_failures
    ADD CONSTRAINT sync_failures_bill_id_fkey
    FOREIGN KEY (bill_id) REFERENCES financial_bills (id);

ALTER TABLE normalization_events ADD COLUMN bill_id TEXT;
ALTER TABLE normalization_events
    ADD CONSTRAINT normalization_events_bill_id_fkey
    FOREIGN KEY (bill_id) REFERENCES financial_bills (id);

-- Widen the CHECKs that gate scope/stage/entity_type to cover bills. Postgres
-- auto-names an inline CHECK "<table>_<column>_check", the same name
-- DROP/ADD CONSTRAINT below targets.
ALTER TABLE raw_imports DROP CONSTRAINT raw_imports_scope_check;
ALTER TABLE raw_imports ADD CONSTRAINT raw_imports_scope_check
    CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills'));

ALTER TABLE sync_failures DROP CONSTRAINT sync_failures_stage_check;
ALTER TABLE sync_failures ADD CONSTRAINT sync_failures_stage_check
    CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'investments', 'investment', 'investment_transactions',
        'bills', 'normalize', 'interrupted', 'worker_unavailable'
    ));

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_entity_type_check;
ALTER TABLE normalization_events ADD CONSTRAINT normalization_events_entity_type_check
    CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction', 'bill'));

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_check;
ALTER TABLE normalization_events ADD CONSTRAINT normalization_events_check
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL AND bill_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL)::int + (transaction_id IS NOT NULL)::int
            + (investment_id IS NOT NULL)::int + (investment_transaction_id IS NOT NULL)::int
            + (bill_id IS NOT NULL)::int) = 1)
    );

ALTER TABLE sync_runs ADD COLUMN bills_processed INTEGER NOT NULL DEFAULT 0 CHECK (bills_processed >= 0);

-- +goose Down

ALTER TABLE sync_runs DROP COLUMN bills_processed;

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_check;
ALTER TABLE normalization_events ADD CONSTRAINT normalization_events_check
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL)::int + (transaction_id IS NOT NULL)::int
            + (investment_id IS NOT NULL)::int + (investment_transaction_id IS NOT NULL)::int) = 1)
    );

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_entity_type_check;
ALTER TABLE normalization_events ADD CONSTRAINT normalization_events_entity_type_check
    CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction'));

ALTER TABLE sync_failures DROP CONSTRAINT sync_failures_stage_check;
ALTER TABLE sync_failures ADD CONSTRAINT sync_failures_stage_check
    CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'investments', 'investment', 'investment_transactions',
        'normalize', 'interrupted', 'worker_unavailable'
    ));

ALTER TABLE raw_imports DROP CONSTRAINT raw_imports_scope_check;
ALTER TABLE raw_imports ADD CONSTRAINT raw_imports_scope_check
    CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions'));

ALTER TABLE normalization_events DROP CONSTRAINT normalization_events_bill_id_fkey;
ALTER TABLE normalization_events DROP COLUMN bill_id;

ALTER TABLE sync_failures DROP CONSTRAINT sync_failures_bill_id_fkey;
ALTER TABLE sync_failures DROP COLUMN external_bill_id;
ALTER TABLE sync_failures DROP COLUMN bill_id;

DROP TABLE financial_bills;
