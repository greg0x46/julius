-- +goose Up

-- recurring_commitment_conditions replaces the old fixed
-- amount_tolerance_percent + implicit day-of-month reconciliation rule with
-- a user-composed condition list, the same shape automation_rule_conditions
-- already uses (see .specs/relatorio-financeiro/m1-recorrencias.md). Unlike
-- automation, the amount/day_of_month fields here store only a tolerance —
-- the reference (expected amount/day) is always the occurrence being
-- resolved, filled in at match time.
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

-- Backfill: every existing commitment's implicit rule (amount tolerance +
-- a fixed 3-day window) becomes its explicit condition list, so behavior is
-- unchanged for commitments registered before this migration.
INSERT INTO recurring_commitment_conditions (id, recurring_commitment_id, field, operator, value, position)
SELECT gen_random_uuid()::text, id, 'amount', 'within_percent', amount_tolerance_percent, 0
FROM recurring_commitments;

INSERT INTO recurring_commitment_conditions (id, recurring_commitment_id, field, operator, value, position)
SELECT gen_random_uuid()::text, id, 'day_of_month', 'near_day', '3', 1
FROM recurring_commitments;

ALTER TABLE recurring_commitments DROP COLUMN amount_tolerance_percent;

-- +goose Down

ALTER TABLE recurring_commitments ADD COLUMN amount_tolerance_percent TEXT NOT NULL DEFAULT '10';

UPDATE recurring_commitments
SET amount_tolerance_percent = (
    SELECT value FROM recurring_commitment_conditions
    WHERE recurring_commitment_id = recurring_commitments.id AND field = 'amount'
    ORDER BY position LIMIT 1
)
WHERE EXISTS (
    SELECT 1 FROM recurring_commitment_conditions
    WHERE recurring_commitment_id = recurring_commitments.id AND field = 'amount'
);

DROP TABLE recurring_commitment_conditions;

ALTER TABLE recurring_commitments DROP COLUMN logic_operator;
