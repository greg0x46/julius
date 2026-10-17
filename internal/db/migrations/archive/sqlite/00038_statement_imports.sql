-- +goose NO TRANSACTION
-- +goose Up

-- SQLite cannot widen the scope CHECK in place. Existing raw imports keep
-- their IDs so every account, transaction and event still references them.
PRAGMA foreign_keys = OFF;
ALTER TABLE sync_runs ADD COLUMN run_type TEXT NOT NULL DEFAULT 'sync'
    CHECK (run_type IN ('sync', 'file_import'));
ALTER TABLE financial_accounts ADD COLUMN balance_as_of TEXT;

DROP TRIGGER raw_imports_reject_update;
DROP TRIGGER raw_imports_reject_delete;
CREATE TABLE raw_imports_new (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills', 'file')),
    external_account_id TEXT,
    page_sequence INTEGER NOT NULL CHECK (page_sequence >= 1),
    request_attempt INTEGER NOT NULL CHECK (request_attempt >= 1),
    request_method TEXT,
    request_path TEXT,
    http_status INTEGER,
    response_headers TEXT,
    payload BLOB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    received_at TEXT NOT NULL,
    CHECK (scope = 'file' OR
        (request_method IS NOT NULL AND request_path IS NOT NULL
         AND http_status IS NOT NULL AND response_headers IS NOT NULL))
);
INSERT INTO raw_imports_new SELECT * FROM raw_imports;
DROP TABLE raw_imports;
ALTER TABLE raw_imports_new RENAME TO raw_imports;
CREATE UNIQUE INDEX uq_raw_imports_identity ON raw_imports (
    sync_run_id, scope, COALESCE(external_account_id, ''), page_sequence, request_attempt
);
-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_update BEFORE UPDATE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_delete BEFORE DELETE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;
-- +goose StatementEnd

CREATE TABLE statement_imports (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL UNIQUE REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    format TEXT NOT NULL,
    adapter_version TEXT NOT NULL,
    filename TEXT NOT NULL,
    file_sha256 TEXT NOT NULL,
    rows_total INTEGER NOT NULL CHECK (rows_total >= 0),
    rows_invalid INTEGER NOT NULL DEFAULT 0 CHECK (rows_invalid >= 0),
    rows_duplicate INTEGER NOT NULL DEFAULT 0 CHECK (rows_duplicate >= 0),
    created_at TEXT NOT NULL
);
CREATE INDEX ix_statement_imports_account_created ON statement_imports (account_id, created_at DESC);
PRAGMA foreign_keys = ON;

-- +goose Down
-- Refuse to discard imported accounts or leave dangling raw-import references.
DROP TABLE IF EXISTS temp.statement_imports_down_guard;
CREATE TEMP TABLE statement_imports_down_guard (file_rows INTEGER CHECK (file_rows = 0));
INSERT INTO statement_imports_down_guard SELECT COUNT(*) FROM raw_imports WHERE scope = 'file';
DROP TABLE statement_imports_down_guard;
PRAGMA foreign_keys = OFF;
DROP TABLE statement_imports;
DROP TRIGGER raw_imports_reject_update;
DROP TRIGGER raw_imports_reject_delete;
CREATE TABLE raw_imports_old (
    id TEXT PRIMARY KEY,
    sync_run_id TEXT NOT NULL REFERENCES sync_runs (id),
    source_id TEXT NOT NULL REFERENCES data_sources (id),
    scope TEXT NOT NULL CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills')),
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
INSERT INTO raw_imports_old SELECT id, sync_run_id, source_id, scope, external_account_id,
    page_sequence, request_attempt, request_method, request_path, http_status,
    response_headers, payload, payload_sha256, received_at
    FROM raw_imports WHERE scope <> 'file';
DROP TABLE raw_imports;
ALTER TABLE raw_imports_old RENAME TO raw_imports;
CREATE UNIQUE INDEX uq_raw_imports_identity ON raw_imports (
    sync_run_id, scope, COALESCE(external_account_id, ''), page_sequence, request_attempt
);
-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_update BEFORE UPDATE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE TRIGGER raw_imports_reject_delete BEFORE DELETE ON raw_imports
BEGIN SELECT RAISE(ABORT, 'raw_imports is append-only'); END;
-- +goose StatementEnd
ALTER TABLE financial_accounts DROP COLUMN balance_as_of;
ALTER TABLE sync_runs DROP COLUMN run_type;
PRAGMA foreign_keys = ON;
