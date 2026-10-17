-- +goose Up

CREATE TABLE debts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    total_amount TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_paid_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_paid_amount AS REAL) >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE debt_transaction_links (
    id TEXT PRIMARY KEY,
    debt_id TEXT NOT NULL REFERENCES debts (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount TEXT NOT NULL,
    linked_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE debt_transaction_links;
DROP TABLE debts;
