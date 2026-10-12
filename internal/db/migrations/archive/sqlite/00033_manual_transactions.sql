-- +goose NO TRANSACTION
-- +goose Up

-- Lets a lançamento be entered by hand instead of arriving through a Pluggy
-- sync, on any account that already exists. Same entity, same table — the
-- Lançamentos engine is meant to be agnostic of where a row came from (see
-- .specs/motores-de-dominio.md section 1) — distinguished by a new `origin`
-- column. The four columns that only make sense for a synced row
-- (source_id/external_id/current_raw_import_id/normalized_hash) become
-- optional, tied to origin by a CHECK: 'synced' rows keep all four filled,
-- 'manual' rows keep all four NULL. UNIQUE (source_id, external_id) needs no
-- change — every manual row carries NULL in both, and NULL never collides
-- with itself.
--
-- deleted_at is a soft-delete marker for a manual row only:
-- transaction_category_events/transaction_inclusion_events are append-only
-- (reject_delete triggers) and RESTRICT deleting the financial_transactions
-- row they point at, so a manual row that was ever categorized or
-- ignored/restored — essentially every one worth keeping — can never be
-- hard-deleted. internal/transactions.viewSelect filters deleted_at out of
-- every read this package serves; nothing else needs to know about it
-- because deleting a manual row also detaches any payable/recurring link it
-- held (see httpapi.onIgnoredHook), the only other place a "gone"
-- transaction could still influence a real total.
--
-- SQLite can't drop a column's NOT NULL in place, so this is a table
-- rebuild (same shape as 00021_automation_rule_set_category_action.sql).
-- NO TRANSACTION plus toggling foreign_keys off around it is required for
-- the same reason 00013_payables.sql's rebuild needed it: with FK
-- enforcement left on, DROP TABLE financial_transactions while
-- transaction_inclusion_decisions/transaction_category_decisions/
-- payable_transaction_links (ON DELETE RESTRICT) and scenario_realizations
-- (ON DELETE CASCADE) still reference it would either abort on the RESTRICT
-- children or cascade-delete the CASCADE ones — either way losing the data
-- this migration means to preserve.
PRAGMA foreign_keys = OFF;

CREATE TABLE financial_transactions_new (
    id TEXT PRIMARY KEY,
    source_id TEXT REFERENCES data_sources (id),
    account_id TEXT NOT NULL REFERENCES financial_accounts (id),
    external_id TEXT,
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
    current_raw_import_id TEXT REFERENCES raw_imports (id),
    normalized_hash TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual')),
    deleted_at TEXT,
    UNIQUE (source_id, external_id),
    CHECK (
        (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
            AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL) OR
        (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
            AND current_raw_import_id IS NULL AND normalized_hash IS NULL)
    )
);

INSERT INTO financial_transactions_new (
    id, source_id, account_id, external_id, description, description_raw, amount,
    amount_in_account_currency, balance_after, currency_code, occurred_at, provider_status,
    movement_type, source_category, source_category_id, provider_code, payment_data,
    credit_card_metadata, merchant, operation_type, operation_type_additional_info,
    operation_category, provider_id, provider_order, provider_created_at, provider_updated_at,
    current_raw_import_id, normalized_hash, created_at, updated_at, origin
)
SELECT
    id, source_id, account_id, external_id, description, description_raw, amount,
    amount_in_account_currency, balance_after, currency_code, occurred_at, provider_status,
    movement_type, source_category, source_category_id, provider_code, payment_data,
    credit_card_metadata, merchant, operation_type, operation_type_additional_info,
    operation_category, provider_id, provider_order, provider_created_at, provider_updated_at,
    current_raw_import_id, normalized_hash, created_at, updated_at, 'synced'
FROM financial_transactions;

DROP TABLE financial_transactions;
ALTER TABLE financial_transactions_new RENAME TO financial_transactions;

CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC, id DESC
);
CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

PRAGMA foreign_keys = ON;

-- +goose Down

-- Lossy on purpose if any manual row exists by now (same precedent as
-- 00032_data_source_connections.sql's down): a manual row has no source/
-- external id/raw import to restore, so it cannot survive a rollback to the
-- old all-synced shape.
PRAGMA foreign_keys = OFF;

DELETE FROM financial_transactions WHERE origin = 'manual';

CREATE TABLE financial_transactions_old (
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

INSERT INTO financial_transactions_old (
    id, source_id, account_id, external_id, description, description_raw, amount,
    amount_in_account_currency, balance_after, currency_code, occurred_at, provider_status,
    movement_type, source_category, source_category_id, provider_code, payment_data,
    credit_card_metadata, merchant, operation_type, operation_type_additional_info,
    operation_category, provider_id, provider_order, provider_created_at, provider_updated_at,
    current_raw_import_id, normalized_hash, created_at, updated_at
)
SELECT
    id, source_id, account_id, external_id, description, description_raw, amount,
    amount_in_account_currency, balance_after, currency_code, occurred_at, provider_status,
    movement_type, source_category, source_category_id, provider_code, payment_data,
    credit_card_metadata, merchant, operation_type, operation_type_additional_info,
    operation_category, provider_id, provider_order, provider_created_at, provider_updated_at,
    current_raw_import_id, normalized_hash, created_at, updated_at
FROM financial_transactions;

DROP TABLE financial_transactions;
ALTER TABLE financial_transactions_old RENAME TO financial_transactions;

CREATE INDEX ix_financial_transactions_occurred_at_id_desc ON financial_transactions (
    occurred_at DESC, id DESC
);
CREATE INDEX ix_financial_transactions_account_id ON financial_transactions (account_id);

PRAGMA foreign_keys = ON;
