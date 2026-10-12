-- +goose Up

-- categories.ApplyLearned (internal/categories/learned.go) writes category
-- decisions with origin 'learned' (a category inferred from a past manual
-- decision on a similar transaction) — widen both tables' origin CHECKs to
-- accept it. Same copy-swap rebuild as 00020_category_decision_rule_origin.sql.
CREATE TABLE transaction_category_decisions_new (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned'))
);
INSERT INTO transaction_category_decisions_new SELECT * FROM transaction_category_decisions;
DROP TABLE transaction_category_decisions;
ALTER TABLE transaction_category_decisions_new RENAME TO transaction_category_decisions;

CREATE INDEX ix_transaction_category_decisions_category_id ON transaction_category_decisions (category_id);

CREATE TABLE transaction_category_events_new (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_category_id TEXT,
    resulting_category_id TEXT NOT NULL,
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule', 'learned')),
    UNIQUE (transaction_id, revision)
);
INSERT INTO transaction_category_events_new SELECT * FROM transaction_category_events;
DROP TABLE transaction_category_events;
ALTER TABLE transaction_category_events_new RENAME TO transaction_category_events;

CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

-- +goose Down

CREATE TABLE transaction_category_decisions_old (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule'))
);
INSERT INTO transaction_category_decisions_old
    SELECT * FROM transaction_category_decisions WHERE origin IN ('manual', 'automatic', 'rule');
DROP TABLE transaction_category_decisions;
ALTER TABLE transaction_category_decisions_old RENAME TO transaction_category_decisions;

CREATE INDEX ix_transaction_category_decisions_category_id ON transaction_category_decisions (category_id);

CREATE TABLE transaction_category_events_old (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_category_id TEXT,
    resulting_category_id TEXT NOT NULL,
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule')),
    UNIQUE (transaction_id, revision)
);
INSERT INTO transaction_category_events_old
    SELECT * FROM transaction_category_events WHERE origin IN ('manual', 'automatic', 'rule');
DROP TABLE transaction_category_events;
ALTER TABLE transaction_category_events_old RENAME TO transaction_category_events;

CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();

CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
FOR EACH ROW EXECUTE FUNCTION reject_append_only_write();
