-- +goose Up

-- automation_rule set_category actions (00021_automation_rule_set_category_action.sql)
-- write category decisions with origin 'rule' (see
-- internal/categories.OriginRule) — widen both tables' origin CHECKs to
-- accept it, alongside the existing 'manual'/'automatic'. SQLite can't
-- ALTER a CHECK constraint in place, so rebuild the tables (same pattern as
-- 00018_widen_automation_conditions_drop_commitment_conditions.sql).
CREATE TABLE transaction_category_decisions_new (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule'))
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
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic', 'rule')),
    UNIQUE (transaction_id, revision)
);
INSERT INTO transaction_category_events_new SELECT * FROM transaction_category_events;
DROP TABLE transaction_category_events;
ALTER TABLE transaction_category_events_new RENAME TO transaction_category_events;

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd

-- +goose Down

CREATE TABLE transaction_category_decisions_old (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    category_id TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic'))
);
INSERT INTO transaction_category_decisions_old
    SELECT * FROM transaction_category_decisions WHERE origin IN ('manual', 'automatic');
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
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'automatic')),
    UNIQUE (transaction_id, revision)
);
INSERT INTO transaction_category_events_old
    SELECT * FROM transaction_category_events WHERE origin IN ('manual', 'automatic');
DROP TABLE transaction_category_events;
ALTER TABLE transaction_category_events_old RENAME TO transaction_category_events;

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_update
BEFORE UPDATE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_category_events_reject_delete
BEFORE DELETE ON transaction_category_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_category_events is append-only');
END;
-- +goose StatementEnd
