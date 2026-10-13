-- +goose Up

ALTER TABLE scenarios ADD COLUMN is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1));
ALTER TABLE scenarios ADD COLUMN is_accounting_source INTEGER NOT NULL DEFAULT 0 CHECK (is_accounting_source IN (0, 1));
ALTER TABLE scenarios DROP CONSTRAINT scenarios_payable_id_check;
ALTER TABLE scenarios ADD CONSTRAINT scenarios_kind_check CHECK (
    kind IN ('debt_plan', 'receivable_plan', 'standalone', 'recurring')
);
ALTER TABLE scenarios ADD CONSTRAINT scenarios_payable_id_check CHECK (
    (kind IN ('standalone', 'recurring') AND payable_id IS NULL) OR
    (kind IN ('debt_plan', 'receivable_plan') AND payable_id IS NOT NULL)
);
ALTER TABLE scenarios ADD CONSTRAINT scenarios_accounting_source_check CHECK (
    is_accounting_source = 0 OR kind IN ('debt_plan', 'receivable_plan')
);

UPDATE scenarios SET is_active = CASE WHEN kind = 'standalone' THEN 0 ELSE 1 END;
UPDATE scenarios s SET is_accounting_source = 1
WHERE s.kind IN ('debt_plan', 'receivable_plan')
  AND s.id = (
      SELECT older.id FROM scenarios older
      WHERE older.payable_id = s.payable_id
        AND older.kind IN ('debt_plan', 'receivable_plan')
      ORDER BY older.created_at, older.id LIMIT 1
  );

CREATE INDEX idx_scenarios_payable_id ON scenarios (payable_id);
CREATE UNIQUE INDEX uq_scenarios_accounting_source
    ON scenarios (payable_id) WHERE is_accounting_source = 1;

INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at)
SELECT
    gen_random_uuid()::text,
    CASE WHEN p.kind = 'debt' THEN 'debt_plan' ELSE 'receivable_plan' END,
    p.name, p.id, 1, 1, p.created_at, p.updated_at
FROM payables p
WHERE NOT EXISTS (
    SELECT 1 FROM scenarios s
    WHERE s.payable_id = p.id AND s.is_accounting_source = 1
);

CREATE TABLE scenario_recurring_schedules (
    scenario_id    TEXT PRIMARY KEY REFERENCES scenarios (id) ON DELETE CASCADE,
    cashflow_kind  TEXT NOT NULL CHECK (cashflow_kind IN ('income', 'expense')),
    amount         TEXT NOT NULL CHECK (CAST(amount AS NUMERIC) > 0),
    category_id    TEXT REFERENCES categories (id) ON DELETE RESTRICT,
    account_id     TEXT REFERENCES financial_accounts (id) ON DELETE RESTRICT,
    cadence        TEXT NOT NULL CHECK (cadence IN ('monthly', 'annual')),
    day_of_month   INTEGER NOT NULL CHECK (day_of_month BETWEEN 1 AND 31),
    month_of_year  INTEGER CHECK (month_of_year BETWEEN 1 AND 12),
    start_date     TEXT NOT NULL,
    end_date       TEXT,
    CHECK (cadence != 'annual' OR month_of_year IS NOT NULL)
);
CREATE INDEX idx_scenario_recurring_schedules_category_id ON scenario_recurring_schedules (category_id);
CREATE INDEX idx_scenario_recurring_schedules_account_id ON scenario_recurring_schedules (account_id);

CREATE TABLE recurring_commitment_scenario_map (
    recurring_commitment_id TEXT PRIMARY KEY REFERENCES recurring_commitments (id) ON DELETE CASCADE,
    scenario_id             TEXT UNIQUE NOT NULL
);
INSERT INTO recurring_commitment_scenario_map (recurring_commitment_id, scenario_id)
SELECT r.id, gen_random_uuid()::text
FROM recurring_commitments r;

INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at)
SELECT m.scenario_id, 'recurring', r.name, NULL, r.is_active, 0, r.created_at, r.updated_at
FROM recurring_commitments r JOIN recurring_commitment_scenario_map m ON m.recurring_commitment_id = r.id;

INSERT INTO scenario_recurring_schedules (
    scenario_id, cashflow_kind, amount, category_id, account_id, cadence,
    day_of_month, month_of_year, start_date, end_date
)
SELECT m.scenario_id, r.kind, r.amount, r.category_id, r.account_id, r.cadence,
       r.day_of_month, r.month_of_year, r.start_date, r.end_date
FROM recurring_commitments r JOIN recurring_commitment_scenario_map m ON m.recurring_commitment_id = r.id;

ALTER TABLE recurring_commitment_scenario_map
    ADD CONSTRAINT recurring_commitment_scenario_map_scenario_fk
    FOREIGN KEY (scenario_id) REFERENCES scenarios (id) ON DELETE CASCADE;

ALTER TABLE recurrence_reconciliations ADD COLUMN scenario_id TEXT REFERENCES scenarios (id) ON DELETE CASCADE;
UPDATE recurrence_reconciliations rr SET scenario_id = m.scenario_id
FROM recurring_commitment_scenario_map m
WHERE m.recurring_commitment_id = rr.recurring_commitment_id;
CREATE INDEX idx_recurrence_reconciliations_scenario ON recurrence_reconciliations (scenario_id, occurrence_date);

-- Rebuild actions so new rules can target a recurring Scenario without
-- manufacturing a legacy recurring_commitment_id. Existing targets are
-- copied to both columns for the compatibility API.
CREATE TABLE automation_rule_actions_new (
    id                      TEXT PRIMARY KEY,
    rule_id                 TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    action_type             TEXT NOT NULL CHECK (action_type IN ('ignore', 'reconcile', 'set_category')),
    scenario_id             TEXT REFERENCES scenarios (id) ON DELETE RESTRICT,
    recurring_commitment_id TEXT REFERENCES recurring_commitments (id) ON DELETE RESTRICT,
    category_id             TEXT REFERENCES categories (id) ON DELETE RESTRICT,
    position                INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position),
    CHECK (
        (action_type = 'ignore' AND scenario_id IS NULL AND recurring_commitment_id IS NULL AND category_id IS NULL) OR
        (action_type = 'reconcile' AND (scenario_id IS NOT NULL OR recurring_commitment_id IS NOT NULL) AND category_id IS NULL) OR
        (action_type = 'set_category' AND scenario_id IS NULL AND recurring_commitment_id IS NULL AND category_id IS NOT NULL)
    )
);
INSERT INTO automation_rule_actions_new (
    id, rule_id, action_type, scenario_id, recurring_commitment_id, category_id, position
)
SELECT
    a.id, a.rule_id, a.action_type,
    CASE WHEN a.action_type = 'reconcile' THEN m.scenario_id END,
    a.recurring_commitment_id, a.category_id, a.position
FROM automation_rule_actions a
LEFT JOIN recurring_commitment_scenario_map m
  ON m.recurring_commitment_id = a.recurring_commitment_id;
DROP TABLE automation_rule_actions;
ALTER TABLE automation_rule_actions_new RENAME TO automation_rule_actions;
CREATE INDEX idx_automation_rule_actions_scenario_id ON automation_rule_actions (scenario_id);
CREATE INDEX idx_automation_rule_actions_recurring_commitment_id ON automation_rule_actions (recurring_commitment_id);
CREATE INDEX idx_automation_rule_actions_category_id ON automation_rule_actions (category_id);
CREATE UNIQUE INDEX uq_automation_rule_actions_one_reconcile_per_scenario
    ON automation_rule_actions (scenario_id)
    WHERE action_type = 'reconcile' AND scenario_id IS NOT NULL;

CREATE TABLE scenario_realizations (
    id                      TEXT PRIMARY KEY,
    scenario_id             TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    scenario_transaction_id TEXT REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    occurrence_date         TEXT,
    transaction_id          TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    relation_type           TEXT NOT NULL CHECK (relation_type IN ('settlement', 'allocation', 'reconciliation')),
    state                   TEXT NOT NULL CHECK (state IN ('linked', 'detached')),
    origin                  TEXT NOT NULL,
    allocated_amount        TEXT,
    linked_amount           TEXT,
    created_at              TEXT NOT NULL,
    CHECK ((state = 'linked' AND transaction_id IS NOT NULL) OR
           (state = 'detached' AND transaction_id IS NULL)),
    CHECK (
        (relation_type = 'settlement' AND scenario_transaction_id IS NULL AND occurrence_date IS NULL AND state = 'linked' AND linked_amount IS NOT NULL) OR
        (relation_type = 'allocation' AND scenario_transaction_id IS NOT NULL AND occurrence_date IS NULL AND state = 'linked' AND allocated_amount IS NOT NULL) OR
        (relation_type = 'reconciliation' AND scenario_transaction_id IS NULL AND occurrence_date IS NOT NULL)
    )
);
CREATE INDEX idx_scenario_realizations_scenario_occurrence ON scenario_realizations (scenario_id, occurrence_date);
CREATE INDEX idx_scenario_realizations_transaction_event ON scenario_realizations (scenario_transaction_id, transaction_id);
CREATE INDEX idx_scenario_realizations_transaction_settlement_reconciliation
    ON scenario_realizations (transaction_id) WHERE relation_type IN ('settlement', 'reconciliation');
CREATE UNIQUE INDEX uq_scenario_realizations_reconciliation_event
    ON scenario_realizations (scenario_id, occurrence_date) WHERE relation_type = 'reconciliation';
CREATE UNIQUE INDEX uq_scenario_realizations_one_complete_event_transaction
    ON scenario_realizations (transaction_id)
    WHERE transaction_id IS NOT NULL AND relation_type IN ('settlement', 'reconciliation') AND state = 'linked';

INSERT INTO scenario_realizations (id, scenario_id, transaction_id, relation_type, state, origin, linked_amount, created_at)
SELECT gen_random_uuid()::text, s.id, l.transaction_id,
       'settlement', 'linked', 'manual', l.linked_amount, l.linked_at
FROM payable_transaction_links l
JOIN scenarios s ON s.payable_id = l.payable_id AND s.is_accounting_source = 1;

INSERT INTO scenario_realizations (
    id, scenario_id, scenario_transaction_id, transaction_id, relation_type,
    state, origin, allocated_amount, created_at
)
SELECT gen_random_uuid()::text, st.scenario_id,
       str.scenario_transaction_id, l.transaction_id, 'allocation', 'linked',
       'manual', str.allocated_amount, str.created_at
FROM scenario_transaction_realizations str
JOIN scenario_transactions st ON st.id = str.scenario_transaction_id
JOIN payable_transaction_links l ON l.id = str.payable_link_id;

INSERT INTO scenario_realizations (id, scenario_id, occurrence_date, transaction_id, relation_type, state, origin, created_at)
SELECT gen_random_uuid()::text, rr.scenario_id, rr.occurrence_date,
       rr.transaction_id, 'reconciliation', rr.state, 'manual', rr.created_at
FROM recurrence_reconciliations rr
WHERE rr.scenario_id IS NOT NULL;

-- +goose Down

DROP TABLE scenario_realizations;
DROP INDEX uq_automation_rule_actions_one_reconcile_per_scenario;
DROP INDEX idx_automation_rule_actions_scenario_id;
DROP INDEX idx_recurrence_reconciliations_scenario;
DROP TABLE scenario_recurring_schedules;
DROP TABLE recurring_commitment_scenario_map;
