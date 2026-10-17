-- +goose Up
ALTER TABLE sync_runs ADD COLUMN run_type TEXT NOT NULL DEFAULT 'sync'
    CHECK (run_type IN ('sync', 'file_import'));
ALTER TABLE financial_accounts ADD COLUMN balance_as_of TEXT;
ALTER TABLE raw_imports DROP CONSTRAINT raw_imports_scope_check;
ALTER TABLE raw_imports ADD CONSTRAINT raw_imports_scope_check
    CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills', 'file'));
ALTER TABLE raw_imports ALTER COLUMN request_method DROP NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN request_path DROP NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN http_status DROP NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN response_headers DROP NOT NULL;
ALTER TABLE raw_imports ADD CONSTRAINT raw_imports_http_fields_check
    CHECK (scope = 'file' OR
        (request_method IS NOT NULL AND request_path IS NOT NULL
         AND http_status IS NOT NULL AND response_headers IS NOT NULL));
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

-- +goose Down
DROP TABLE statement_imports;
ALTER TABLE raw_imports DROP CONSTRAINT raw_imports_http_fields_check;
ALTER TABLE raw_imports ALTER COLUMN request_method SET NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN request_path SET NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN http_status SET NOT NULL;
ALTER TABLE raw_imports ALTER COLUMN response_headers SET NOT NULL;
ALTER TABLE raw_imports DROP CONSTRAINT raw_imports_scope_check;
ALTER TABLE raw_imports ADD CONSTRAINT raw_imports_scope_check
    CHECK (scope IN ('item', 'accounts', 'transactions', 'investments', 'investment_transactions', 'bills'));
ALTER TABLE financial_accounts DROP COLUMN balance_as_of;
ALTER TABLE sync_runs DROP COLUMN run_type;
