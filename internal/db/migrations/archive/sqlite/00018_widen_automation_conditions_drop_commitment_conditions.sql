-- +goose Up

-- Reconciliation conditions now live exclusively on automation_rule_conditions
-- (via a rule's 'reconcile' action, see 00017_automation_rule_actions.sql) —
-- widen its field/operator CHECKs to accept amount/day_of_month/
-- within_percent/near_day, which automation_rule_conditions previously
-- forbade (those only made sense for the now-removed
-- recurring_commitment_conditions). SQLite can't ALTER a CHECK constraint in
-- place, so rebuild the table (same pattern as 00013_payables.sql).
CREATE TABLE automation_rule_conditions_new (
    id       TEXT PRIMARY KEY,
    rule_id  TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    field    TEXT NOT NULL CHECK (field IN ('description', 'card', 'account', 'amount', 'day_of_month')),
    operator TEXT NOT NULL CHECK (operator IN ('contains', 'equals', 'within_percent', 'near_day')),
    value    TEXT NOT NULL CHECK (trim(value) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position)
);
INSERT INTO automation_rule_conditions_new SELECT * FROM automation_rule_conditions;
DROP TABLE automation_rule_conditions;
ALTER TABLE automation_rule_conditions_new RENAME TO automation_rule_conditions;

-- recurring_commitment_conditions is gone: reconciliation criteria now live
-- exclusively on automation_rule_conditions via a rule's reconcile action.
-- This drops any locally-configured commitments' reconciliation setups —
-- they must be recreated as automation rules afterward. No backfill target
-- exists (would require inventing rule names), and this repo hasn't gone to
-- production, so that data loss is accepted deliberately.
DROP TABLE recurring_commitment_conditions;
ALTER TABLE recurring_commitments DROP COLUMN logic_operator;

-- +goose Down

ALTER TABLE recurring_commitments ADD COLUMN logic_operator TEXT NOT NULL DEFAULT 'and' CHECK (logic_operator IN ('and', 'or'));

CREATE TABLE recurring_commitment_conditions (
    id                      TEXT PRIMARY KEY,
    recurring_commitment_id TEXT NOT NULL REFERENCES recurring_commitments (id) ON DELETE CASCADE,
    field                   TEXT NOT NULL CHECK (field IN ('description', 'card', 'account', 'amount', 'day_of_month')),
    operator                TEXT NOT NULL CHECK (operator IN ('contains', 'equals', 'within_percent', 'near_day')),
    value                   TEXT NOT NULL CHECK (trim(value) <> ''),
    position                INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (recurring_commitment_id, position)
);

CREATE TABLE automation_rule_conditions_old (
    id       TEXT PRIMARY KEY,
    rule_id  TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    field    TEXT NOT NULL CHECK (field IN ('description', 'card', 'account')),
    operator TEXT NOT NULL CHECK (operator IN ('contains', 'equals')),
    value    TEXT NOT NULL CHECK (trim(value) <> ''),
    position INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position)
);
INSERT INTO automation_rule_conditions_old
    SELECT * FROM automation_rule_conditions WHERE field IN ('description', 'card', 'account');
DROP TABLE automation_rule_conditions;
ALTER TABLE automation_rule_conditions_old RENAME TO automation_rule_conditions;
