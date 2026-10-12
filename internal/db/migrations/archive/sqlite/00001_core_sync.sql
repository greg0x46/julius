-- +goose Up

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
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions')),
    external_account_id TEXT,
    page_sequence INTEGER NOT NULL CHECK (page_sequence >= 1),
    request_attempt INTEGER NOT NULL CHECK (request_attempt >= 1),
    request_method TEXT NOT NULL,
    request_path TEXT NOT NULL,
    http_status INTEGER NOT NULL,
    response_headers TEXT NOT NULL,
    payload BLOB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TEXT NOT NULL
);

-- Emulates Postgres' UNIQUE NULLS NOT DISTINCT: COALESCE folds every NULL
-- external_account_id to '' so two rows that differ only by a NULL there
-- still collide, matching the raw_import_identity constraint upstream.
CREATE UNIQUE INDEX uq_raw_imports_identity ON raw_imports (
    sync_run_id, scope, COALESCE(external_account_id, ''), page_sequence, request_attempt
);

-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_update
BEFORE UPDATE ON raw_imports
BEGIN
    SELECT RAISE(ABORT, 'raw_imports is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_delete
BEFORE DELETE ON raw_imports
BEGIN
    SELECT RAISE(ABORT, 'raw_imports is append-only');
END;
-- +goose StatementEnd

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

-- SQLite already sorts NULLs last in DESC order (the opposite of Postgres'
-- default), so no explicit NULLS LAST is needed to match the reference index.
CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC, id DESC
);
CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

CREATE TABLE normalization_events (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'transaction')),
    account_id TEXT REFERENCES financial_accounts (id),
    transaction_id TEXT REFERENCES financial_transactions (id),
    external_id TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('inserted', 'updated', 'unchanged', 'rejected')),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL) + (transaction_id IS NOT NULL)) = 1)
    )
);

-- +goose StatementBegin
CREATE TRIGGER normalization_events_reject_update
BEFORE UPDATE ON normalization_events
BEGIN
    SELECT RAISE(ABORT, 'normalization_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER normalization_events_reject_delete
BEFORE DELETE ON normalization_events
BEGIN
    SELECT RAISE(ABORT, 'normalization_events is append-only');
END;
-- +goose StatementEnd

CREATE TABLE sync_failures (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT REFERENCES raw_imports (id),
    account_id TEXT REFERENCES financial_accounts (id),
    external_account_id TEXT,
    external_transaction_id TEXT,
    stage TEXT NOT NULL CHECK (stage IN (
        'auth', 'item', 'accounts', 'account', 'transactions',
        'normalize', 'interrupted', 'worker_unavailable'
    )),
    error_code TEXT NOT NULL,
    safe_message TEXT NOT NULL,
    created_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE sync_failures;
DROP TRIGGER normalization_events_reject_delete;
DROP TRIGGER normalization_events_reject_update;
DROP TABLE normalization_events;
DROP TABLE financial_transactions;
DROP TABLE financial_accounts;
DROP TRIGGER raw_imports_reject_delete;
DROP TRIGGER raw_imports_reject_update;
DROP TABLE raw_imports;
DROP TABLE sync_runs;
DROP TABLE data_sources;
