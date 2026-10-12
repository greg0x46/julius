-- +goose Up

-- recurring_commitments materializes a manually-registered recurring cash
-- flow (salary, rent, subscriptions) that Pluggy has no concept of ahead of
-- time — see .specs/relatorio-financeiro/m1-recorrencias.md. It's
-- deliberately independent from financial_transactions (same isolation
-- principle as scenario_transactions): reconciliation against real
-- transactions happens at read time via internal/rules, never by storing a
-- link.
CREATE TABLE recurring_commitments (
    id                        TEXT PRIMARY KEY,
    name                      TEXT NOT NULL CHECK (trim(name) <> ''),
    kind                      TEXT NOT NULL CHECK (kind IN ('income', 'expense')),
    amount                    TEXT NOT NULL,
    amount_tolerance_percent  TEXT NOT NULL DEFAULT '10',
    category_id               TEXT NOT NULL REFERENCES categories (id),
    account_id                TEXT REFERENCES financial_accounts (id),
    cadence                   TEXT NOT NULL CHECK (cadence IN ('monthly', 'annual')),
    day_of_month              INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31),
    month_of_year             INTEGER CHECK (month_of_year BETWEEN 1 AND 12),
    start_date                TEXT NOT NULL,
    end_date                  TEXT,
    is_active                 INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at                TEXT NOT NULL,
    updated_at                TEXT NOT NULL,
    CHECK (cadence != 'annual' OR month_of_year IS NOT NULL)
);

CREATE INDEX idx_recurring_commitments_category_id ON recurring_commitments (category_id);
CREATE INDEX idx_recurring_commitments_account_id ON recurring_commitments (account_id);

-- +goose Down

DROP TABLE recurring_commitments;
