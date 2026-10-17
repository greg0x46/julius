-- +goose Up

CREATE TABLE automation_rules (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (trim(name) <> ''),
    is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    logic_operator TEXT NOT NULL CHECK (logic_operator IN ('and', 'or')),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE automation_rule_conditions (
    id TEXT PRIMARY KEY,
    rule_id TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    field TEXT NOT NULL CHECK (field IN ('description', 'card', 'account')),
    operator TEXT NOT NULL CHECK (operator IN ('contains', 'equals')),
    value TEXT NOT NULL CHECK (trim(value) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position)
);

-- +goose Down

DROP TABLE automation_rule_conditions;
DROP TABLE automation_rules;
