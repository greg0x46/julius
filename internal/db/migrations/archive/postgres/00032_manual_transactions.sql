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
ALTER TABLE financial_transactions ADD COLUMN origin TEXT NOT NULL DEFAULT 'synced';

-- Soft-delete marker for a manual row only: transaction_category_events and
-- transaction_inclusion_events are append-only (see their reject_delete
-- triggers) and RESTRICT the deletion of the financial_transactions row they
-- point at, so a manual row that was ever categorized or ignored/restored —
-- essentially every one a user bothers to keep — can never be hard-deleted.
-- deleted_at lets DeleteManual remove one from view without touching that
-- audit trail. internal/transactions.viewSelect filters it out of every read
-- this package serves (the ledger, totals, GetItem); nothing else needs to
-- know about it because deleting a manual row also detaches any payable/
-- recurring link it held (see httpapi.onIgnoredHook), which is the only
-- other place a "gone" transaction could still influence a real total.
ALTER TABLE financial_transactions ADD COLUMN deleted_at TEXT;

ALTER TABLE financial_transactions ALTER COLUMN source_id DROP NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN external_id DROP NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN current_raw_import_id DROP NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN normalized_hash DROP NOT NULL;

ALTER TABLE financial_transactions ADD CONSTRAINT chk_financial_transactions_origin CHECK (
    (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
        AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL) OR
    (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
        AND current_raw_import_id IS NULL AND normalized_hash IS NULL)
);

-- +goose Down

-- Lossy on purpose if any manual row exists by now (same precedent as
-- 00031_data_source_connections.sql's down): a manual row has no source/
-- external id/raw import to restore, so it cannot survive a rollback to the
-- old all-synced shape.
DELETE FROM financial_transactions WHERE origin = 'manual';

ALTER TABLE financial_transactions DROP CONSTRAINT chk_financial_transactions_origin;

ALTER TABLE financial_transactions ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN external_id SET NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN current_raw_import_id SET NOT NULL;
ALTER TABLE financial_transactions ALTER COLUMN normalized_hash SET NOT NULL;

ALTER TABLE financial_transactions DROP COLUMN deleted_at;
ALTER TABLE financial_transactions DROP COLUMN origin;
