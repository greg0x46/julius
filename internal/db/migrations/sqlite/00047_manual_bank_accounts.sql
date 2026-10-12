-- +goose NO TRANSACTION
-- +goose Up
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TABLE financial_accounts_new (
    id TEXT PRIMARY KEY,
    source_id TEXT REFERENCES data_sources (id),
    external_id TEXT,
    institution TEXT,
    name TEXT,
    number TEXT,
    account_type TEXT,
    account_subtype TEXT,
    balance TEXT,
    credit_limit TEXT,
    currency_code TEXT,
    provider_updated_at TEXT,
    current_raw_import_id TEXT REFERENCES raw_imports (id),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    available_credit_limit TEXT,
    balance_close_date TEXT,
    balance_due_date TEXT,
    manual_closing_day INTEGER CHECK (manual_closing_day BETWEEN 1 AND 31),
    balance_as_of TEXT,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    opening_balance TEXT,
    archived_at TEXT,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL AND opening_balance IS NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL
            AND account_type IS NOT NULL AND account_type = 'BANK' AND currency_code IS NOT NULL
            AND opening_balance IS NOT NULL AND balance IS NULL)
    )
);
INSERT INTO financial_accounts_new (id, source_id, external_id, institution, name, number, account_type, account_subtype, balance, credit_limit, currency_code, provider_updated_at, current_raw_import_id, normalized_hash, created_at, updated_at, available_credit_limit, balance_close_date, balance_due_date, manual_closing_day, balance_as_of) SELECT id, source_id, external_id, institution, name, number, account_type, account_subtype, balance, credit_limit, currency_code, provider_updated_at, current_raw_import_id, normalized_hash, created_at, updated_at, available_credit_limit, balance_close_date, balance_due_date, manual_closing_day, balance_as_of FROM financial_accounts;
DROP TABLE financial_accounts;
ALTER TABLE financial_accounts_new RENAME TO financial_accounts;
CREATE TABLE account_balance_adjustments (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
    amount TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX ix_account_balance_adjustments_account ON account_balance_adjustments(account_id);
COMMIT;
PRAGMA foreign_keys = ON;

-- +goose Down
PRAGMA foreign_keys = OFF;
BEGIN;
CREATE TEMP TABLE manual_accounts_rollback_guard (n INTEGER CHECK(n = 0));
INSERT INTO manual_accounts_rollback_guard SELECT COUNT(*) FROM financial_accounts WHERE origin = 'manual';
DROP TABLE manual_accounts_rollback_guard;
DROP TABLE account_balance_adjustments;
CREATE TABLE financial_accounts_old (
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
    available_credit_limit TEXT,
    balance_close_date TEXT,
    balance_due_date TEXT,
    manual_closing_day INTEGER CHECK (manual_closing_day BETWEEN 1 AND 31),
    balance_as_of TEXT,
    UNIQUE (source_id, external_id));
INSERT INTO financial_accounts_old (id, source_id, external_id, institution, name, number, account_type, account_subtype, balance, credit_limit, currency_code, provider_updated_at, current_raw_import_id, normalized_hash, created_at, updated_at, available_credit_limit, balance_close_date, balance_due_date, manual_closing_day, balance_as_of) SELECT id, source_id, external_id, institution, name, number, account_type, account_subtype, balance, credit_limit, currency_code, provider_updated_at, current_raw_import_id, normalized_hash, created_at, updated_at, available_credit_limit, balance_close_date, balance_due_date, manual_closing_day, balance_as_of FROM financial_accounts;
DROP TABLE financial_accounts;
ALTER TABLE financial_accounts_old RENAME TO financial_accounts;
COMMIT;
PRAGMA foreign_keys = ON;
