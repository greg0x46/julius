-- +goose NO TRANSACTION
-- +goose Up

-- Unifies debts and receivables into a single Payable concept: a debt is
-- money the user owes, a receivable is money owed to the user, and beyond
-- that one directional difference (checked via the `kind` column, and via
-- money.Outflow/Inflow classification at the Go layer) they were tracked
-- identically. See internal/payables.
--
-- NO TRANSACTION plus toggling foreign_keys off around the table rebuild
-- below is required: PRAGMA foreign_keys can't be changed inside a
-- transaction, and with it left on, `DROP TABLE scenarios` while
-- scenario_transactions/scenario_transaction_realizations still hold
-- ON DELETE CASCADE references to it does not just drop the table — SQLite
-- treats dropping a referenced parent table as implicitly deleting every
-- row in it first, cascading away the very data this migration is trying
-- to preserve.
PRAGMA foreign_keys = OFF;

CREATE TABLE payables (
    id                      TEXT PRIMARY KEY,
    kind                    TEXT NOT NULL CHECK (kind IN ('debt', 'receivable')),
    name                    TEXT NOT NULL,
    total_amount            TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_settled_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_settled_amount AS REAL) >= 0),
    created_at              TEXT NOT NULL,
    updated_at              TEXT NOT NULL
);

CREATE TABLE payable_transaction_links (
    id             TEXT PRIMARY KEY,
    payable_id     TEXT NOT NULL REFERENCES payables (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount  TEXT NOT NULL,
    linked_at      TEXT NOT NULL
);

INSERT INTO payables (id, kind, name, total_amount, starting_settled_amount, created_at, updated_at)
    SELECT id, 'debt', name, total_amount, starting_paid_amount, created_at, updated_at FROM debts;
INSERT INTO payables (id, kind, name, total_amount, starting_settled_amount, created_at, updated_at)
    SELECT id, 'receivable', name, total_amount, starting_received_amount, created_at, updated_at FROM receivables;

INSERT INTO payable_transaction_links (id, payable_id, transaction_id, linked_amount, linked_at)
    SELECT id, debt_id, transaction_id, linked_amount, linked_at FROM debt_transaction_links;
INSERT INTO payable_transaction_links (id, payable_id, transaction_id, linked_amount, linked_at)
    SELECT id, receivable_id, transaction_id, linked_amount, linked_at FROM receivable_transaction_links;

-- SQLite can't ALTER a CHECK constraint or a column's nullability in place
-- (same limitation 00009_receivables.sql hit) — rebuild scenarios and
-- scenario_transaction_realizations to point at a single payable_id /
-- payable_link_id instead of the debt_id/receivable_id and
-- debt_link_id/receivable_link_id pairs.

CREATE TABLE scenarios_new (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    payable_id TEXT REFERENCES payables (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (payable_id IS NOT NULL)
);
INSERT INTO scenarios_new (id, kind, name, payable_id, created_at, updated_at)
    SELECT id, kind, name, COALESCE(debt_id, receivable_id), created_at, updated_at FROM scenarios;
DROP TABLE scenarios;
ALTER TABLE scenarios_new RENAME TO scenarios;

CREATE TABLE scenario_transaction_realizations_new (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    payable_link_id          TEXT REFERENCES payable_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL,
    CHECK (payable_link_id IS NOT NULL)
);
INSERT INTO scenario_transaction_realizations_new (id, scenario_transaction_id, payable_link_id, allocated_amount, created_at)
    SELECT id, scenario_transaction_id, COALESCE(debt_link_id, receivable_link_id), allocated_amount, created_at FROM scenario_transaction_realizations;
DROP TABLE scenario_transaction_realizations;
ALTER TABLE scenario_transaction_realizations_new RENAME TO scenario_transaction_realizations;
CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_payable_link_id ON scenario_transaction_realizations (payable_link_id);

DROP TABLE debt_transaction_links;
DROP TABLE receivable_transaction_links;
DROP TABLE debts;
DROP TABLE receivables;

PRAGMA foreign_keys = ON;

-- +goose Down

PRAGMA foreign_keys = OFF;

CREATE TABLE debts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    total_amount TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_paid_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_paid_amount AS REAL) >= 0),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE receivables (
    id                       TEXT PRIMARY KEY,
    name                     TEXT NOT NULL,
    total_amount             TEXT NOT NULL CHECK (CAST(total_amount AS REAL) > 0),
    starting_received_amount TEXT NOT NULL DEFAULT '0' CHECK (CAST(starting_received_amount AS REAL) >= 0),
    created_at               TEXT NOT NULL,
    updated_at               TEXT NOT NULL
);
INSERT INTO debts (id, name, total_amount, starting_paid_amount, created_at, updated_at)
    SELECT id, name, total_amount, starting_settled_amount, created_at, updated_at FROM payables WHERE kind = 'debt';
INSERT INTO receivables (id, name, total_amount, starting_received_amount, created_at, updated_at)
    SELECT id, name, total_amount, starting_settled_amount, created_at, updated_at FROM payables WHERE kind = 'receivable';

CREATE TABLE debt_transaction_links (
    id TEXT PRIMARY KEY,
    debt_id TEXT NOT NULL REFERENCES debts (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount TEXT NOT NULL,
    linked_at TEXT NOT NULL
);
CREATE TABLE receivable_transaction_links (
    id             TEXT PRIMARY KEY,
    receivable_id  TEXT NOT NULL REFERENCES receivables (id) ON DELETE CASCADE,
    transaction_id TEXT NOT NULL UNIQUE REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    linked_amount  TEXT NOT NULL,
    linked_at      TEXT NOT NULL
);
INSERT INTO debt_transaction_links (id, debt_id, transaction_id, linked_amount, linked_at)
    SELECT l.id, l.payable_id, l.transaction_id, l.linked_amount, l.linked_at
    FROM payable_transaction_links l JOIN payables p ON p.id = l.payable_id WHERE p.kind = 'debt';
INSERT INTO receivable_transaction_links (id, receivable_id, transaction_id, linked_amount, linked_at)
    SELECT l.id, l.payable_id, l.transaction_id, l.linked_amount, l.linked_at
    FROM payable_transaction_links l JOIN payables p ON p.id = l.payable_id WHERE p.kind = 'receivable';

CREATE TABLE scenario_transaction_realizations_old (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    debt_link_id             TEXT REFERENCES debt_transaction_links (id) ON DELETE CASCADE,
    receivable_link_id       TEXT REFERENCES receivable_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL,
    CHECK ((debt_link_id IS NOT NULL) != (receivable_link_id IS NOT NULL))
);
INSERT INTO scenario_transaction_realizations_old (id, scenario_transaction_id, debt_link_id, receivable_link_id, allocated_amount, created_at)
    SELECT str.id, str.scenario_transaction_id,
           CASE WHEN p.kind = 'debt' THEN str.payable_link_id END,
           CASE WHEN p.kind = 'receivable' THEN str.payable_link_id END,
           str.allocated_amount, str.created_at
    FROM scenario_transaction_realizations str
    JOIN payable_transaction_links l ON l.id = str.payable_link_id
    JOIN payables p ON p.id = l.payable_id;
DROP TABLE scenario_transaction_realizations;
ALTER TABLE scenario_transaction_realizations_old RENAME TO scenario_transaction_realizations;
CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);
CREATE INDEX idx_str_receivable_link_id ON scenario_transaction_realizations (receivable_link_id);

CREATE TABLE scenarios_old (
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
INSERT INTO scenarios_old (id, kind, name, debt_id, receivable_id, created_at, updated_at)
    SELECT s.id, s.kind, s.name,
           CASE WHEN p.kind = 'debt' THEN s.payable_id END,
           CASE WHEN p.kind = 'receivable' THEN s.payable_id END,
           s.created_at, s.updated_at
    FROM scenarios s JOIN payables p ON p.id = s.payable_id;
DROP TABLE scenarios;
ALTER TABLE scenarios_old RENAME TO scenarios;

DROP TABLE payable_transaction_links;
DROP TABLE payables;

PRAGMA foreign_keys = ON;
