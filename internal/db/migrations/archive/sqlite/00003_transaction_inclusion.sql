-- +goose Up

CREATE TABLE transaction_inclusion_decisions (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    state TEXT NOT NULL CHECK (state IN ('considered', 'ignored')),
    revision INTEGER NOT NULL CHECK (revision >= 1),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'rule')),
    rule_id TEXT REFERENCES automation_rules (id) ON DELETE SET NULL,
    rule_name TEXT,
    CHECK (origin = 'rule' OR (rule_id IS NULL AND rule_name IS NULL))
);

CREATE TABLE transaction_inclusion_events (
    id TEXT PRIMARY KEY,
    transaction_id TEXT NOT NULL REFERENCES financial_transactions (id) ON DELETE RESTRICT,
    revision INTEGER NOT NULL CHECK (revision >= 1),
    previous_state TEXT NOT NULL CHECK (previous_state IN ('considered', 'ignored')),
    resulting_state TEXT NOT NULL CHECK (resulting_state IN ('considered', 'ignored')),
    changed_at TEXT NOT NULL,
    origin TEXT NOT NULL CHECK (origin IN ('manual', 'rule')),
    -- No FK on rule_id: this table is append-only (see triggers below), and a
    -- deleted rule must not force an UPDATE of historical rows to null it out.
    -- rule_name is the durable audit snapshot; rule_id is a best-effort reference.
    rule_id TEXT,
    rule_name TEXT,
    CHECK (previous_state <> resulting_state),
    CHECK (origin = 'rule' OR (rule_id IS NULL AND rule_name IS NULL)),
    UNIQUE (transaction_id, revision)
);

-- +goose StatementBegin
CREATE TRIGGER transaction_inclusion_events_reject_update
BEFORE UPDATE ON transaction_inclusion_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_inclusion_events is append-only');
END;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TRIGGER transaction_inclusion_events_reject_delete
BEFORE DELETE ON transaction_inclusion_events
BEGIN
    SELECT RAISE(ABORT, 'transaction_inclusion_events is append-only');
END;
-- +goose StatementEnd

-- +goose Down

DROP TRIGGER transaction_inclusion_events_reject_delete;
DROP TRIGGER transaction_inclusion_events_reject_update;
DROP TABLE transaction_inclusion_events;
DROP TABLE transaction_inclusion_decisions;
