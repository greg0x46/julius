-- +goose Up
ALTER TABLE financial_accounts ALTER COLUMN source_id DROP NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN external_id DROP NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN current_raw_import_id DROP NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN normalized_hash DROP NOT NULL;
ALTER TABLE financial_accounts ADD COLUMN origin TEXT NOT NULL DEFAULT 'synced' CHECK (origin IN ('synced', 'manual'));
ALTER TABLE financial_accounts ADD COLUMN opening_balance TEXT;
ALTER TABLE financial_accounts ADD COLUMN archived_at TEXT;
ALTER TABLE financial_accounts ADD CONSTRAINT financial_accounts_origin_identity CHECK (
    (origin = 'synced' AND source_id IS NOT NULL AND external_id IS NOT NULL
        AND current_raw_import_id IS NOT NULL AND normalized_hash IS NOT NULL AND opening_balance IS NULL) OR
    (origin = 'manual' AND source_id IS NULL AND external_id IS NULL
        AND current_raw_import_id IS NULL AND normalized_hash IS NULL
        AND account_type = 'BANK' AND currency_code IS NOT NULL
        AND opening_balance IS NOT NULL AND balance IS NULL)
);
CREATE TABLE account_balance_adjustments (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES financial_accounts(id) ON DELETE RESTRICT,
    amount TEXT NOT NULL,
    description TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE INDEX ix_account_balance_adjustments_account ON account_balance_adjustments(account_id);

-- +goose Down
-- +goose StatementBegin
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM financial_accounts WHERE origin = 'manual') THEN
  RAISE EXCEPTION 'Cannot roll back with manual accounts';
 END IF;
END $$;
-- +goose StatementEnd
DROP TABLE account_balance_adjustments;
ALTER TABLE financial_accounts DROP CONSTRAINT financial_accounts_origin_identity;
ALTER TABLE financial_accounts DROP COLUMN archived_at;
ALTER TABLE financial_accounts DROP COLUMN opening_balance;
ALTER TABLE financial_accounts DROP COLUMN origin;
ALTER TABLE financial_accounts ALTER COLUMN source_id SET NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN external_id SET NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN current_raw_import_id SET NOT NULL;
ALTER TABLE financial_accounts ALTER COLUMN normalized_hash SET NOT NULL;
