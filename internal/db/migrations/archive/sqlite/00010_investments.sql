-- +goose NO TRANSACTION
-- +goose Up

-- Rebuilding raw_imports/normalization_events/sync_failures below drops and
-- recreates tables that financial_accounts/financial_transactions/etc. still
-- hold live foreign keys into on a database with real synced data (unlike an
-- empty dev DB, where the DROP TABLE never has any referencing rows to
-- violate) — SQLite refuses that DROP under enforcement, so FK checking is
-- suspended for exactly this rebuild block and restored before the file
-- ends. PRAGMA foreign_keys is also a no-op inside a transaction, hence NO
-- TRANSACTION above; each statement here runs and commits individually.
PRAGMA foreign_keys = OFF;

-- Investments (holdings) and their transaction history, mirroring
-- financial_accounts/financial_transactions: same current_raw_import_id +
-- normalized_hash change-detection pattern.
CREATE TABLE financial_investments (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    external_id TEXT NOT NULL,
    investment_type TEXT,
    subtype TEXT,
    name TEXT,
    balance TEXT,
    currency_code TEXT,
    number TEXT,
    owner TEXT,
    tax_number TEXT,
    due_date TEXT,
    issuer TEXT,
    issuer_code TEXT,
    rate TEXT,
    rate_type TEXT,
    fixed_annual_rate TEXT,
    annual_rate TEXT,
    last_twelve_months_rate TEXT,
    quantity TEXT,
    value TEXT,
    amount TEXT,
    amount_profit TEXT,
    amount_withdrawal TEXT,
    as_of_date TEXT,
    provider_updated_at TEXT,
    isin TEXT,
    code TEXT,
    provider_status TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE TABLE financial_investment_transactions (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    investment_id TEXT NOT NULL REFERENCES financial_investments (id),
    external_id TEXT NOT NULL,
    movement_type TEXT,
    quantity TEXT,
    value TEXT,
    amount TEXT,
    occurred_at TEXT,
    trade_date TEXT,
    current_raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    normalized_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (source_id, external_id)
);

CREATE INDEX ix_financial_investment_transactions_investment_id ON financial_investment_transactions (investment_id);
CREATE INDEX ix_financial_investment_transactions_occurred_at_id_desc ON financial_investment_transactions (
    occurred_at DESC, id DESC
);

-- sync_failures, raw_imports, and normalization_events all CHECK-constrain a
-- column that must widen to cover investments; SQLite can't ALTER a CHECK in
-- place, so each is rebuilt (create _new with the wider constraint, copy,
-- drop, rename), same as 00009_receivables.sql's scenarios rebuild.

CREATE TABLE sync_failures_new (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT REFERENCES raw_imports (id),
    account_id TEXT REFERENCES financial_accounts (id),
    external_account_id TEXT,
    external_transaction_id TEXT,
    investment_id TEXT REFERENCES financial_investments (id),
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
INSERT INTO sync_failures_new (
    id, sync_run_id, raw_import_id, account_id, external_account_id,
    external_transaction_id, stage, error_code, safe_message, created_at
)
    SELECT id, sync_run_id, raw_import_id, account_id, external_account_id,
           external_transaction_id, stage, error_code, safe_message, created_at
    FROM sync_failures;
DROP TABLE sync_failures;
ALTER TABLE sync_failures_new RENAME TO sync_failures;

-- +goose StatementBegin
DROP TRIGGER raw_imports_reject_update;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER raw_imports_reject_delete;
-- +goose StatementEnd

CREATE TABLE raw_imports_new (
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
    payload BLOB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TEXT NOT NULL
);
INSERT INTO raw_imports_new SELECT * FROM raw_imports;
DROP TABLE raw_imports;
ALTER TABLE raw_imports_new RENAME TO raw_imports;

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

-- +goose StatementBegin
DROP TRIGGER normalization_events_reject_update;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER normalization_events_reject_delete;
-- +goose StatementEnd

CREATE TABLE normalization_events_new (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    raw_import_id TEXT NOT NULL REFERENCES raw_imports (id),
    entity_type TEXT NOT NULL CHECK (entity_type IN ('account', 'transaction', 'investment', 'investment_transaction')),
    account_id TEXT REFERENCES financial_accounts (id),
    transaction_id TEXT REFERENCES financial_transactions (id),
    investment_id TEXT REFERENCES financial_investments (id),
    investment_transaction_id TEXT REFERENCES financial_investment_transactions (id),
    external_id TEXT,
    outcome TEXT NOT NULL CHECK (outcome IN ('inserted', 'updated', 'unchanged', 'rejected')),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    CHECK (
        (outcome = 'rejected' AND account_id IS NULL AND transaction_id IS NULL
            AND investment_id IS NULL AND investment_transaction_id IS NULL) OR
        (outcome <> 'rejected' AND ((account_id IS NOT NULL) + (transaction_id IS NOT NULL)
            + (investment_id IS NOT NULL) + (investment_transaction_id IS NOT NULL)) = 1)
    )
);
INSERT INTO normalization_events_new (
    id, sync_run_id, raw_import_id, entity_type, account_id, transaction_id,
    external_id, outcome, normalized_hash, created_at
)
    SELECT id, sync_run_id, raw_import_id, entity_type, account_id, transaction_id,
           external_id, outcome, normalized_hash, created_at
    FROM normalization_events;
DROP TABLE normalization_events;
ALTER TABLE normalization_events_new RENAME TO normalization_events;

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

ALTER TABLE sync_runs ADD COLUMN investments_processed INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_runs ADD COLUMN investment_transactions_inserted INTEGER NOT NULL DEFAULT 0;
ALTER TABLE sync_runs ADD COLUMN investment_transactions_updated INTEGER NOT NULL DEFAULT 0;

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

ALTER TABLE sync_runs DROP COLUMN investment_transactions_updated;
ALTER TABLE sync_runs DROP COLUMN investment_transactions_inserted;
ALTER TABLE sync_runs DROP COLUMN investments_processed;

-- +goose StatementBegin
DROP TRIGGER normalization_events_reject_update;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER normalization_events_reject_delete;
-- +goose StatementEnd

CREATE TABLE normalization_events_old (
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
INSERT INTO normalization_events_old (
    id, sync_run_id, raw_import_id, entity_type, account_id, transaction_id,
    external_id, outcome, normalized_hash, created_at
)
    SELECT id, sync_run_id, raw_import_id, entity_type, account_id, transaction_id,
           external_id, outcome, normalized_hash, created_at
    FROM normalization_events
    WHERE entity_type IN ('account', 'transaction');
DROP TABLE normalization_events;
ALTER TABLE normalization_events_old RENAME TO normalization_events;

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

-- +goose StatementBegin
DROP TRIGGER raw_imports_reject_update;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TRIGGER raw_imports_reject_delete;
-- +goose StatementEnd

CREATE TABLE raw_imports_old (
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
INSERT INTO raw_imports_old SELECT * FROM raw_imports WHERE scope IN ('item', 'accounts', 'transactions');
DROP TABLE raw_imports;
ALTER TABLE raw_imports_old RENAME TO raw_imports;

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

CREATE TABLE sync_failures_old (
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
INSERT INTO sync_failures_old (
    id, sync_run_id, raw_import_id, account_id, external_account_id,
    external_transaction_id, stage, error_code, safe_message, created_at
)
    SELECT id, sync_run_id, raw_import_id, account_id, external_account_id,
           external_transaction_id, stage, error_code, safe_message, created_at
    FROM sync_failures
    WHERE stage NOT IN ('investments', 'investment', 'investment_transactions');
DROP TABLE sync_failures;
ALTER TABLE sync_failures_old RENAME TO sync_failures;

DROP TABLE financial_investment_transactions;
DROP TABLE financial_investments;

PRAGMA foreign_keys = ON;
