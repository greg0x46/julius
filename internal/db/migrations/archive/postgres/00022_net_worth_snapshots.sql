-- +goose Up

-- One row per calendar day the app was opened, capturing the point-in-time
-- net worth computed by internal/networth.Compute. There is no historical
-- reconstruction: financial_accounts.balance and financial_investments are
-- overwritten on every sync, so the series can only grow forward from
-- whenever this table starts being written. See internal/networth.
CREATE TABLE net_worth_snapshots (
    id                    TEXT PRIMARY KEY,
    captured_at           TEXT NOT NULL UNIQUE,
    total_assets          TEXT NOT NULL,
    total_liabilities     TEXT NOT NULL,
    net_worth             TEXT NOT NULL,
    cash_balance          TEXT NOT NULL,
    investment_balance    TEXT NOT NULL,
    credit_card_balance   TEXT NOT NULL,
    payables_debt         TEXT NOT NULL,
    created_at            TEXT NOT NULL,
    updated_at            TEXT NOT NULL
);

-- +goose Down

DROP TABLE net_worth_snapshots;
