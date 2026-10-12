-- +goose Up

-- Postgres baseline for what the sqlite dialect reaches through
-- sqlite/00007_scenarios.sql, sqlite/00008_scenario_transaction_realizations.sql
-- and sqlite/00009_receivables.sql combined: scenarios/scenario_transactions
-- started as debt-only, then 00009 widened them (plus
-- scenario_transaction_realizations) to also support receivables by
-- rebuilding the tables (SQLite can't ALTER a CHECK constraint or a column's
-- nullability in place). Written here directly in the final, receivable-aware
-- shape since Postgres never needs that rebuild.

-- Mirrors debts: a Receivable is money owed *to* the user (a loan made, a
-- reimbursement pending, ...), tracked the same way a Debt is —
-- total_amount plus a starting_received_amount snapshot, with
-- received/remaining amounts and status always recomputed from the linked
-- transactions, never stored. See internal/receivables.
CREATE TABLE receivables (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL,
    total_amount             TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_received_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_received_amount AS REAL) >= 0),
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);

CREATE TABLE receivable_transaction_links (
    id             TEXT PRIMARY KEY,
    receivable_id  TEXT NOT NULL REFERENCES receivables (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount  TEXT NOT NULL,
    linked_at      TEXT NOT NULL
);

-- scenarios / scenario_transactions materialize a hypothetical, editable
-- projection layered on top of real data — see
-- .specs/plano-pagamento-e-cenarios-projecao.md. They are purely additive:
-- nothing about financial_transactions, debts', or receivables' own
-- paid/remaining computation changes because these tables exist. A scenario
-- attaches to exactly one of a debt or a receivable, never both.
CREATE TABLE scenarios (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    name          TEXT NOT NULL,
    debt_id       TEXT REFERENCES debts (id) ON DELETE CASCADE,
    receivable_id TEXT REFERENCES receivables (id) ON DELETE CASCADE,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CHECK (kind != 'debt_plan' OR (debt_id IS NOT NULL AND receivable_id IS NULL)),
    CHECK (kind != 'receivable_plan' OR (receivable_id IS NOT NULL AND debt_id IS NULL))
);

CREATE TABLE scenario_transactions (
    id           TEXT PRIMARY KEY,
    scenario_id  TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       TEXT NOT NULL,
    projected_at TEXT NOT NULL,
    category     TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX idx_scenario_transactions_scenario_id ON scenario_transactions (scenario_id);

-- Allocates (part of) a real debt_transaction_links or
-- receivable_transaction_links row to a planned scenario_transaction. Many-
-- to-many on purpose: a single real link can be split across several planned
-- installments (a lump-sum payment covering two months at once), and a
-- single installment can receive allocations from more than one link (a
-- payment split across two real transactions). Allocates to exactly one of a
-- debt link or a receivable link, never both.
CREATE TABLE scenario_transaction_realizations (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    debt_link_id             TEXT REFERENCES debt_transaction_links (id) ON DELETE CASCADE,
    receivable_link_id       TEXT REFERENCES receivable_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL,
    CHECK ((debt_link_id IS NOT NULL) != (receivable_link_id IS NOT NULL))
);

CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);
CREATE INDEX idx_str_receivable_link_id ON scenario_transaction_realizations (receivable_link_id);

-- +goose Down

DROP TABLE scenario_transaction_realizations;
DROP TABLE scenario_transactions;
DROP TABLE scenarios;
DROP TABLE receivable_transaction_links;
DROP TABLE receivables;
