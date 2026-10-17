-- +goose Up

-- Postgres baseline: this mirrors the *final* shape the sqlite dialect
-- reaches after sqlite/00001_core_sync.sql through sqlite/00010_investments.sql
-- (including the investments-related columns 00010 adds to sync_runs,
-- raw_imports, normalization_events and sync_failures), written directly
-- instead of replaying SQLite's intermediate table-rebuild migrations —
-- Postgres has full ALTER TABLE / transactional DDL support, so those
-- rebuilds (needed only to work around SQLite's limited ALTER TABLE) have no
-- equivalent here. See internal/db/migrations/sqlite for the SQLite history.

CREATE TABLE data_sources (
    id TEXT PRIMARY KEY,
    provider TEXT NOT NULL,
    external_item_id TEXT NOT NULL,
    display_name TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (provider, external_item_id)
);

CREATE TABLE sync_runs (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    status TEXT NOT NULL DEFAULT 'in_progress'
        CHECK (status IN ('in_progress', 'completed', 'completed_with_failures', 'failed')),
    started_at TEXT NOT NULL,
    finished_at TEXT,
    heartbeat_at TEXT,
    worker_id TEXT,
    accounts_processed INTEGER NOT NULL DEFAULT 0 CHECK (accounts_processed >= 0),
    transactions_inserted INTEGER NOT NULL DEFAULT 0 CHECK (transactions_inserted >= 0),
    transactions_updated INTEGER NOT NULL DEFAULT 0 CHECK (transactions_updated >= 0),
    investments_processed INTEGER NOT NULL DEFAULT 0,
    investment_transactions_inserted INTEGER NOT NULL DEFAULT 0,
    investment_transactions_updated INTEGER NOT NULL DEFAULT 0,
    general_error_code TEXT,
    general_error_message TEXT,
    CHECK (
        (status = 'in_progress' AND finished_at IS NULL) OR
        (status <> 'in_progress' AND finished_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uq_sync_runs_active_source ON sync_runs (source_id)
    WHERE status = 'in_progress';

CREATE TABLE raw_imports (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions')),
    external_account_id TEXT,
    page_sequence INTEGER NOT NULL CHECK (page_sequence >= 1),
    request_attempt INTEGER NOT NULL CHECK (request_attempt >= 1),
    request_method TEXT NOT NULL,
    request_path TEXT NOT NULL,
    http_status INTEGER NOT NULL,
    response_headers TEXT NOT NULL,
    payload BYTEA NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TEXT NOT NULL
);

-- Postgres has native UNIQUE NULLS NOT DISTINCT (15+), but this keeps the
-- same COALESCE-based expression index the SQLite dialect uses so both
-- dialects share one mental model and neither depends on a Postgres version
-- floor.
CREATE UNIQUE INDEX uq_raw_imports_identity ON raw_imports (
    sync_run_id, scope, COALESCE(external_account_id, ''), page_sequence, request_attempt
);

-- +goose StatementBegin
CREATE FUNCTION reject_append_only_write() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER raw_imports_reject_update
BEFORE UPDATE ON raw_imports
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER raw_imports_reject_delete
BEFORE DELETE ON raw_imports
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TABLE financial_accounts (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    external_id TEXT NOT NULL,
    institution TEXT,
    name TEXT,
    number TEXT,
    account_type TEXT,
    account_subtype TEXT,
    balance TEXT,
    credit_limit TEXT,
    currency_code TEXT,
    provider_updated_at TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE TABLE financial_transactions (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    external_id TEXT NOT NULL,
    description TEXT,
    description_raw TEXT,
    amount TEXT,
    amount_in_account_currency TEXT,
    balance_after TEXT,
    currency_code TEXT,
    occurred_at TEXT,
    provider_status TEXT,
    movement_type TEXT,
    source_category TEXT,
    source_category_id TEXT,
    provider_code TEXT,
    payment_data TEXT,
    credit_card_metadata TEXT,
    merchant TEXT,
    operation_type TEXT,
    operation_type_additional_info TEXT,
    operation_category TEXT,
    provider_id TEXT,
    provider_order INTEGER,
    provider_created_at TEXT,
    provider_updated_at TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

-- Postgres defaults DESC indexes to NULLS FIRST (the opposite of SQLite's
-- always-smallest NULL ordering, which puts NULLs last in DESC) — NULLS LAST
-- here reproduces the SQLite dialect's ordering exactly.
CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC NULLS LAST, id DESC
);
CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

CREATE TABLE normalization_events (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction')),
    account_id TEXT REFERENCES financial_accounts (id),
    transaction_id TEXT REFERENCES financial_transactions (id),
    investment_id TEXT,
    investment_transaction_id TEXT,
    external_id TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('inserted', 'updated', 'unchanged', 'rejected')),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL)::int + (transaction_id IS NOT NULL)::int
            + (investment_id IS NOT NULL)::int + (investment_transaction_id IS NOT NULL)::int) = 1)
    )
);

CREATE TRIGGER normalization_events_reject_update
BEFORE UPDATE ON normalization_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER normalization_events_reject_delete
BEFORE DELETE ON normalization_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TABLE sync_failures (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT REFERENCES raw_imports (id),
    account_id TEXT REFERENCES financial_accounts (id),
    external_account_id TEXT,
    external_transaction_id TEXT,
    investment_id TEXT,
    external_investment_id TEXT,
    stage TEXT NOT NULL CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'investments', 'investment', 'investment_transactions',
        'normalize', 'interrupted', 'worker_unavailable'
    )),
    error_code TEXT NOT NULL,
    safe_message TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE sync_failures;
DROP TRIGGER normalization_events_reject_delete ON normalization_events;
DROP TRIGGER normalization_events_reject_update ON normalization_events;
DROP TABLE normalization_events;
DROP TABLE financial_transactions;
DROP TABLE financial_accounts;
DROP TRIGGER raw_imports_reject_delete ON raw_imports;
DROP TRIGGER raw_imports_reject_update ON raw_imports;
DROP FUNCTION reject_append_only_write();
DROP TABLE raw_imports;
DROP TABLE sync_runs;
DROP TABLE data_sources;
