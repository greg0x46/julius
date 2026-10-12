-- The closing day of a credit card is not reliably available from the
-- provider: creditData.balanceCloseDate is only sent by some connectors, and
-- closed bills only exist once at least one invoice has closed. This column
-- lets the user state the day themselves, taking precedence over both.
--
-- It lives on financial_accounts even though the sync pipeline owns that
-- table, because the pipeline writes an explicit column list (see
-- syncsvc.upsertAccount) and never touches this one. The "manual_" prefix is
-- the guard: it marks the column as user-owned for anyone extending that
-- INSERT/UPDATE later.

-- +goose Up

ALTER TABLE financial_accounts ADD COLUMN manual_closing_day INTEGER
    CHECK (manual_closing_day BETWEEN 1 AND 31);

-- +goose Down

ALTER TABLE financial_accounts DROP COLUMN manual_closing_day;
