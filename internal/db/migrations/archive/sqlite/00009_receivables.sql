-- +goose Up

-- Mirrors 00005_debts.sql: a Receivable is money owed *to* the user (a
-- loan made, a reimbursement pending, ...), tracked the same way a Debt
-- is — total_amount plus a starting_received_amount snapshot, with
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

-- scenarios/scenario_transaction_realizations need rebuilding (SQLite can't
-- ALTER a CHECK constraint or a column's nullability in place) to let a
-- scenario attach to a receivable instead of a debt, and a realization
-- allocate to a receivable_transaction_links row instead of a
-- debt_transaction_links one — mutually exclusive with the debt case in
-- both tables, enforced by the rebuilt CHECK constraints below.

CREATE TABLE scenarios_new (
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
INSERT INTO scenarios_new (id, kind, name, debt_id, receivable_id, created_at, updated_at)
    SELECT id, kind, name, debt_id, NULL, created_at, updated_at FROM scenarios;
DROP TABLE scenarios;
ALTER TABLE scenarios_new RENAME TO scenarios;

CREATE TABLE scenario_transactions_new (
    id           TEXT PRIMARY KEY,
    scenario_id  TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       TEXT NOT NULL,
    projected_at TEXT NOT NULL,
    category     TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
INSERT INTO scenario_transactions_new SELECT * FROM scenario_transactions;
DROP TABLE scenario_transactions;
ALTER TABLE scenario_transactions_new RENAME TO scenario_transactions;
CREATE INDEX idx_scenario_transactions_scenario_id ON scenario_transactions (scenario_id);

CREATE TABLE scenario_transaction_realizations_new (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    debt_link_id             TEXT REFERENCES debt_transaction_links (id) ON DELETE CASCADE,
    receivable_link_id       TEXT REFERENCES receivable_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL,
    CHECK ((debt_link_id IS NOT NULL) != (receivable_link_id IS NOT NULL))
);
INSERT INTO scenario_transaction_realizations_new (id, scenario_transaction_id, debt_link_id, receivable_link_id, allocated_amount, created_at)
    SELECT id, scenario_transaction_id, debt_link_id, NULL, allocated_amount, created_at FROM scenario_transaction_realizations;
DROP TABLE scenario_transaction_realizations;
ALTER TABLE scenario_transaction_realizations_new RENAME TO scenario_transaction_realizations;
CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);
CREATE INDEX idx_str_receivable_link_id ON scenario_transaction_realizations (receivable_link_id);

-- +goose Down

DROP INDEX idx_str_receivable_link_id;
DROP INDEX idx_str_debt_link_id;
DROP INDEX idx_str_scenario_transaction_id;

CREATE TABLE scenario_transaction_realizations_old (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    debt_link_id             TEXT NOT NULL REFERENCES debt_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL
);
INSERT INTO scenario_transaction_realizations_old (id, scenario_transaction_id, debt_link_id, allocated_amount, created_at)
    SELECT id, scenario_transaction_id, debt_link_id, allocated_amount, created_at FROM scenario_transaction_realizations WHERE debt_link_id IS NOT NULL;
DROP TABLE scenario_transaction_realizations;
ALTER TABLE scenario_transaction_realizations_old RENAME TO scenario_transaction_realizations;
CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);

DROP INDEX idx_scenario_transactions_scenario_id;
CREATE TABLE scenario_transactions_old (
    id           TEXT PRIMARY KEY,
    scenario_id  TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       TEXT NOT NULL,
    projected_at TEXT NOT NULL,
    category     TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
INSERT INTO scenario_transactions_old SELECT * FROM scenario_transactions;
DROP TABLE scenario_transactions;
ALTER TABLE scenario_transactions_old RENAME TO scenario_transactions;
CREATE INDEX idx_scenario_transactions_scenario_id ON scenario_transactions (scenario_id);

CREATE TABLE scenarios_old (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    debt_id    TEXT REFERENCES debts (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (kind != 'debt_plan' OR debt_id IS NOT NULL)
);
INSERT INTO scenarios_old (id, kind, name, debt_id, created_at, updated_at)
    SELECT id, kind, name, debt_id, created_at, updated_at FROM scenarios WHERE receivable_id IS NULL;
DROP TABLE scenarios;
ALTER TABLE scenarios_old RENAME TO scenarios;

DROP TABLE receivable_transaction_links;
DROP TABLE receivables;
