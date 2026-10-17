-- Pluggy's Account.creditData carries more than the credit limit already
-- stored: how much of that limit is still free, and when the currently open
-- fatura closes and falls due. The open fatura never reaches financial_bills
-- (Pluggy only exposes closed/overdue bills), so these two dates are the only
-- way to say when the balance on a card is actually due.

-- +goose Up

ALTER TABLE financial_accounts ADD COLUMN available_credit_limit TEXT;
ALTER TABLE financial_accounts ADD COLUMN balance_close_date TEXT;
ALTER TABLE financial_accounts ADD COLUMN balance_due_date TEXT;

-- +goose Down

ALTER TABLE financial_accounts DROP COLUMN balance_due_date;
ALTER TABLE financial_accounts DROP COLUMN balance_close_date;
ALTER TABLE financial_accounts DROP COLUMN available_credit_limit;
