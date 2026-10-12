-- +goose Up

-- A recurring commitment is now nothing but a Scenario of kind 'recurring'
-- plus its scenario_recurring_schedules row. Every reader and writer — the
-- projector, the automation reconcile target, the occurrence decisions, the
-- HTTP API — is addressed by the scenario id, so the legacy table, its
-- mapping and the duplicated automation target column have nothing left
-- pointing at them.
--
-- Anything created through the old write path since the unification
-- migration already has its scenario; a commitment without one would be
-- invisible to every reader, so it is materialized here rather than lost.
-- The commitment's own id becomes that scenario's id: it is already a UUID,
-- so a scenario already holding it would be a UUID collision, and reusing it
-- means no row ever has to reference a scenario that does not exist yet —
-- the same ordering the SQLite migration relies on.
INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at)
SELECT r.id, 'recurring', r.name, NULL, r.is_active, 0, r.created_at, r.updated_at
FROM recurring_commitments r
WHERE NOT EXISTS (
    SELECT 1 FROM recurring_commitment_scenario_map m WHERE m.recurring_commitment_id = r.id
)
AND NOT EXISTS (SELECT 1 FROM scenarios s WHERE s.id = r.id);

INSERT INTO recurring_commitment_scenario_map (recurring_commitment_id, scenario_id)
SELECT r.id, r.id
FROM recurring_commitments r
WHERE NOT EXISTS (
    SELECT 1 FROM recurring_commitment_scenario_map m WHERE m.recurring_commitment_id = r.id
);

INSERT INTO scenario_recurring_schedules (
    scenario_id, cashflow_kind, amount, category_id, account_id, cadence,
    day_of_month, month_of_year, start_date, end_date
)
SELECT m.scenario_id, r.kind, r.amount, r.category_id, r.account_id, r.cadence,
       r.day_of_month, r.month_of_year, r.start_date, r.end_date
FROM recurring_commitments r
JOIN recurring_commitment_scenario_map m ON m.recurring_commitment_id = r.id
WHERE NOT EXISTS (
    SELECT 1 FROM scenario_recurring_schedules s WHERE s.scenario_id = m.scenario_id
);

-- A reconcile action written before scenario_id existed still carries only
-- its commitment; resolve it before the column disappears. Every commitment
-- has a scenario by now, and the column is a FK, so this resolves all of
-- them — the DELETE below is a floor, not an expected path.
UPDATE automation_rule_actions a
SET scenario_id = m.scenario_id
FROM recurring_commitment_scenario_map m
WHERE m.recurring_commitment_id = a.recurring_commitment_id
  AND a.action_type = 'reconcile' AND a.scenario_id IS NULL;

-- A reconcile action with no scenario left to point at cannot be kept: the
-- new CHECK forbids it, and there is nothing for the resolver to key on.
DELETE FROM automation_rule_actions WHERE action_type = 'reconcile' AND scenario_id IS NULL;

-- Dropping an action must not leave its rule behind action-less: a rule with
-- no actions is a state automation.Write.Validate rejects, so it would be
-- listed by the API and impossible to save. A rule that kept at least one
-- other action stays exactly as it was.
DELETE FROM automation_rules
WHERE NOT EXISTS (
    SELECT 1 FROM automation_rule_actions a WHERE a.rule_id = automation_rules.id
);

-- DROP COLUMN takes the table constraints involving that column with it, so
-- the old three-way CHECK (which named recurring_commitment_id in every
-- branch) goes away here and is replaced by the two-target form below.
ALTER TABLE automation_rule_actions DROP COLUMN recurring_commitment_id;
ALTER TABLE automation_rule_actions ADD CONSTRAINT automation_rule_actions_target_check CHECK (
    (action_type = 'ignore' AND scenario_id IS NULL AND category_id IS NULL) OR
    (action_type = 'reconcile' AND scenario_id IS NOT NULL AND category_id IS NULL) OR
    (action_type = 'set_category' AND scenario_id IS NULL AND category_id IS NOT NULL)
);

DROP TABLE recurring_commitment_scenario_map;
DROP TABLE recurring_commitments;

-- +goose Down

CREATE TABLE recurring_commitments (
    id            TEXT PRIMARY KEY,
    name          TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('income', 'expense')),
    amount        TEXT NOT NULL CHECK (CAST(amount AS NUMERIC) > 0),
    category_id   TEXT NOT NULL REFERENCES categories (id) ON DELETE RESTRICT,
    account_id    TEXT REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    cadence       TEXT NOT NULL CHECK (cadence IN ('monthly', 'annual')),
    day_of_month  INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31),
    month_of_year INTEGER CHECK (month_of_year BETWEEN 1 AND 12),
    start_date    TEXT NOT NULL,
    end_date      TEXT,
    is_active     INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1)),
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL,
    CHECK (cadence != 'annual' OR month_of_year IS NOT NULL)
);

CREATE TABLE recurring_commitment_scenario_map (
    recurring_commitment_id TEXT PRIMARY KEY REFERENCES recurring_commitments (id) ON DELETE CASCADE,
    scenario_id             TEXT UNIQUE NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE
);

-- LOSSY, deliberately: recurring_commitments.category_id is NOT NULL, while
-- the schedule that replaced it allows a recurring scenario with no
-- category. Those scenarios have no representation in the old shape, so
-- rolling back drops them (their scenario row and schedule survive — only
-- the legacy alias is missing). Check for schedules with a NULL category_id
-- before relying on this Down.
INSERT INTO recurring_commitments (
    id, name, kind, amount, category_id, account_id, cadence, day_of_month,
    month_of_year, start_date, end_date, is_active, created_at, updated_at
)
SELECT s.id, s.name, r.cashflow_kind, r.amount, r.category_id, r.account_id,
       r.cadence, r.day_of_month, r.month_of_year, r.start_date, r.end_date,
       s.is_active, s.created_at, s.updated_at
FROM scenarios s
JOIN scenario_recurring_schedules r ON r.scenario_id = s.id
WHERE s.kind = 'recurring' AND r.category_id IS NOT NULL;

INSERT INTO recurring_commitment_scenario_map (recurring_commitment_id, scenario_id)
SELECT id, id FROM recurring_commitments;

ALTER TABLE automation_rule_actions DROP CONSTRAINT automation_rule_actions_target_check;
ALTER TABLE automation_rule_actions ADD COLUMN recurring_commitment_id TEXT REFERENCES recurring_commitments (id) ON DELETE RESTRICT;
UPDATE automation_rule_actions a
SET recurring_commitment_id = m.recurring_commitment_id
FROM recurring_commitment_scenario_map m
WHERE m.scenario_id = a.scenario_id AND a.action_type = 'reconcile';
ALTER TABLE automation_rule_actions ADD CONSTRAINT automation_rule_actions_target_check CHECK (
    (action_type = 'ignore' AND scenario_id IS NULL AND recurring_commitment_id IS NULL AND category_id IS NULL) OR
    (action_type = 'reconcile' AND (scenario_id IS NOT NULL OR recurring_commitment_id IS NOT NULL) AND category_id IS NULL) OR
    (action_type = 'set_category' AND scenario_id IS NULL AND recurring_commitment_id IS NULL AND category_id IS NOT NULL)
);
CREATE INDEX idx_automation_rule_actions_recurring_commitment_id
    ON automation_rule_actions (recurring_commitment_id);
