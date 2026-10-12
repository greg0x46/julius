-- +goose Up

-- Unifies debts and receivables into a single Payable concept: a debt is
-- money the user owes, a receivable is money owed to the user, and beyond
-- that one directional difference (checked via the `kind` column, and via
-- money.Outflow/Inflow classification at the Go layer) they were tracked
-- identically. See internal/payables.
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

-- Repoint scenarios at a single payable_id (kind already says debt_plan vs
-- receivable_plan; the payable it points to now carries its own kind too,
-- consistency between the two is validated at the HTTP layer since a CHECK
-- can't reach across tables).
-- Dropping debt_id/receivable_id below also drops (automatically, no
-- CASCADE needed for same-table constraints) the two kind CHECKs, since
-- both reference both columns.
ALTER TABLE scenarios ADD COLUMN payable_id TEXT REFERENCES payables (id) ON DELETE CASCADE;
UPDATE scenarios SET payable_id = COALESCE(debt_id, receivable_id);
ALTER TABLE scenarios DROP COLUMN debt_id;
ALTER TABLE scenarios DROP COLUMN receivable_id;
ALTER TABLE scenarios ADD CONSTRAINT scenarios_payable_id_check CHECK (payable_id IS NOT NULL);

-- Same automatic-drop reasoning: dropping debt_link_id removes the
-- (debt_link_id IS NOT NULL) != (receivable_link_id IS NOT NULL) CHECK,
-- since it references debt_link_id. The unrelated allocated_amount CHECK
-- is untouched.
ALTER TABLE scenario_transaction_realizations ADD COLUMN payable_link_id TEXT REFERENCES payable_transaction_links (id) ON DELETE CASCADE;
UPDATE scenario_transaction_realizations SET payable_link_id = COALESCE(debt_link_id, receivable_link_id);
ALTER TABLE scenario_transaction_realizations DROP COLUMN debt_link_id;
ALTER TABLE scenario_transaction_realizations DROP COLUMN receivable_link_id;
ALTER TABLE scenario_transaction_realizations ADD CONSTRAINT scenario_transaction_realizations_payable_link_id_check CHECK (payable_link_id IS NOT NULL);

CREATE INDEX idx_str_payable_link_id ON scenario_transaction_realizations (payable_link_id);

DROP TABLE debt_transaction_links;
DROP TABLE receivable_transaction_links;
DROP TABLE debts;
DROP TABLE receivables;

-- +goose Down

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

DROP INDEX idx_str_payable_link_id;

-- Dropping payable_id below also drops its CHECK automatically.
ALTER TABLE scenarios ADD COLUMN debt_id TEXT REFERENCES debts (id) ON DELETE CASCADE;
ALTER TABLE scenarios ADD COLUMN receivable_id TEXT REFERENCES receivables (id) ON DELETE CASCADE;
UPDATE scenarios SET debt_id = payable_id WHERE kind = 'debt_plan';
UPDATE scenarios SET receivable_id = payable_id WHERE kind = 'receivable_plan';
ALTER TABLE scenarios DROP COLUMN payable_id;
ALTER TABLE scenarios ADD CONSTRAINT scenarios_kind_check1 CHECK (kind != 'debt_plan' OR (debt_id IS NOT NULL AND receivable_id IS NULL));
ALTER TABLE scenarios ADD CONSTRAINT scenarios_kind_check2 CHECK (kind != 'receivable_plan' OR (receivable_id IS NOT NULL AND debt_id IS NULL));

ALTER TABLE scenario_transaction_realizations ADD COLUMN debt_link_id TEXT REFERENCES debt_transaction_links (id) ON DELETE CASCADE;
ALTER TABLE scenario_transaction_realizations ADD COLUMN receivable_link_id TEXT REFERENCES receivable_transaction_links (id) ON DELETE CASCADE;
UPDATE scenario_transaction_realizations str SET debt_link_id = str.payable_link_id
    WHERE EXISTS (SELECT 1 FROM debt_transaction_links dl WHERE dl.id = str.payable_link_id);
UPDATE scenario_transaction_realizations str SET receivable_link_id = str.payable_link_id
    WHERE EXISTS (SELECT 1 FROM receivable_transaction_links rl WHERE rl.id = str.payable_link_id);
ALTER TABLE scenario_transaction_realizations DROP COLUMN payable_link_id;
ALTER TABLE scenario_transaction_realizations ADD CONSTRAINT scenario_transaction_realizations_check CHECK ((debt_link_id IS NOT NULL) != (receivable_link_id IS NOT NULL));

CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);
CREATE INDEX idx_str_receivable_link_id ON scenario_transaction_realizations (receivable_link_id);

DROP TABLE payable_transaction_links;
DROP TABLE payables;
