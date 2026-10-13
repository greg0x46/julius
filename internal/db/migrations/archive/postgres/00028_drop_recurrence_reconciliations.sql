-- +goose Up

-- scenario_realizations (relation_type = 'reconciliation') is now the only
-- store for a manual occurrence decision: the occurrence list, the candidate
-- filter, the transaction drawer and the Timeline projector all read it, and
-- both write paths — the compatibility occurrence endpoint and the generic
-- event endpoint — insert there. Carry over anything that only ever reached
-- the legacy table before dropping it.
INSERT INTO scenario_realizations (
    id, scenario_id, occurrence_date, transaction_id, relation_type,
    state, origin, created_at
)
SELECT rr.id, rr.scenario_id, rr.occurrence_date, rr.transaction_id,
       'reconciliation', rr.state, 'manual', rr.created_at
FROM recurrence_reconciliations rr
WHERE rr.scenario_id IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM scenario_realizations sr
      WHERE sr.relation_type = 'reconciliation'
        AND sr.scenario_id = rr.scenario_id
        AND sr.occurrence_date = rr.occurrence_date
  );

DROP TABLE recurrence_reconciliations;

-- +goose Down

CREATE TABLE recurrence_reconciliations (
    id                      TEXT PRIMARY KEY,
    recurring_commitment_id TEXT NOT NULL REFERENCES recurring_commitments (id) ON DELETE CASCADE,
    scenario_id             TEXT REFERENCES scenarios (id) ON DELETE CASCADE,
    occurrence_date         TEXT NOT NULL,
    state                   TEXT NOT NULL CHECK (state IN ('linked', 'detached')),
    transaction_id          TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    created_at              TEXT NOT NULL,
    UNIQUE (recurring_commitment_id, occurrence_date),
    UNIQUE (transaction_id),
    CHECK ((state = 'linked' AND transaction_id IS NOT NULL) OR
           (state = 'detached' AND transaction_id IS NULL))
);

CREATE INDEX idx_recurrence_reconciliations_commitment
    ON recurrence_reconciliations (recurring_commitment_id, occurrence_date);
CREATE INDEX idx_recurrence_reconciliations_scenario
    ON recurrence_reconciliations (scenario_id, occurrence_date);

INSERT INTO recurrence_reconciliations (
    id, recurring_commitment_id, scenario_id, occurrence_date, state, transaction_id, created_at
)
SELECT sr.id, m.recurring_commitment_id, sr.scenario_id, sr.occurrence_date,
       sr.state, sr.transaction_id, sr.created_at
FROM scenario_realizations sr
JOIN recurring_commitment_scenario_map m ON m.scenario_id = sr.scenario_id
WHERE sr.relation_type = 'reconciliation';
