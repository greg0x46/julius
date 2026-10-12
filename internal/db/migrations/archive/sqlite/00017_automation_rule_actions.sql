-- +goose Up

-- automation_rule_actions replaces the hardcoded "every rule ignores" with
-- an explicit action a rule performs when its conditions match: 'ignore' (no
-- target) today, or 'reconcile' (targets a recurring_commitments row).
-- Future action types (e.g. 'set_category') will add their own target
-- column here rather than a new table.
CREATE TABLE automation_rule_actions (
    id                      TEXT PRIMARY KEY,
    rule_id                 TEXT NOT NULL REFERENCES automation_rules (id) ON DELETE CASCADE,
    action_type             TEXT NOT NULL CHECK (action_type IN ('ignore', 'reconcile')),
    recurring_commitment_id TEXT REFERENCES recurring_commitments (id) ON DELETE RESTRICT,
    position                INTEGER NOT NULL CHECK (position >= 0),
    UNIQUE (rule_id, position),
    CHECK (
        (action_type = 'ignore' AND recurring_commitment_id IS NULL) OR
        (action_type = 'reconcile' AND recurring_commitment_id IS NOT NULL)
    )
);

CREATE INDEX idx_automation_rule_actions_recurring_commitment_id
    ON automation_rule_actions (recurring_commitment_id);

-- At most one reconcile action may target a given commitment at a time —
-- internal/timeline reads "the" reconcile rule for a commitment as a 1:1
-- lookup (see internal/automation.ListActiveReconcileTargets).
CREATE UNIQUE INDEX idx_automation_rule_actions_one_reconcile_per_commitment
    ON automation_rule_actions (recurring_commitment_id)
    WHERE action_type = 'reconcile';

-- Backfill: every existing rule's implicit behavior (always ignore) becomes
-- an explicit 'ignore' action, so pre-refactor rules keep working unchanged.
INSERT INTO automation_rule_actions (id, rule_id, action_type, recurring_commitment_id, position)
SELECT lower(hex(randomblob(16))), id, 'ignore', NULL, 0
FROM automation_rules;

-- +goose Down

DROP TABLE automation_rule_actions;
