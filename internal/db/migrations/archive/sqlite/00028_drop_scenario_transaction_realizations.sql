-- +goose Up

-- scenario_realizations (relation_type = 'allocation') is now the only
-- allocation store: every reader — the plan summary, the readjustment
-- selector and the Timeline projector — goes through it, and the compatibility
-- endpoint writes there directly. Any allocation that only ever existed in the
-- legacy table (created between the unification migration and this one through
-- the old write path) is carried over before the table goes away.
INSERT INTO scenario_realizations (
    id, scenario_id, scenario_transaction_id, transaction_id, relation_type,
    state, origin, allocated_amount, created_at
)
SELECT
    str.id, st.scenario_id, str.scenario_transaction_id, l.transaction_id,
    'allocation', 'linked', 'manual', str.allocated_amount, str.created_at
FROM scenario_transaction_realizations str
JOIN scenario_transactions st ON st.id = str.scenario_transaction_id
JOIN payable_transaction_links l ON l.id = str.payable_link_id
WHERE NOT EXISTS (
    SELECT 1 FROM scenario_realizations sr
    WHERE sr.relation_type = 'allocation'
      AND sr.scenario_transaction_id = str.scenario_transaction_id
      AND sr.transaction_id = l.transaction_id
);

DROP TABLE scenario_transaction_realizations;

-- +goose Down

CREATE TABLE scenario_transaction_realizations (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    payable_link_id          TEXT REFERENCES payable_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL,
    CHECK (payable_link_id IS NOT NULL)
);

CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_payable_link_id ON scenario_transaction_realizations (payable_link_id);

INSERT INTO scenario_transaction_realizations (
    id, scenario_transaction_id, payable_link_id, allocated_amount, created_at
)
SELECT sr.id, sr.scenario_transaction_id, l.id, sr.allocated_amount, sr.created_at
FROM scenario_realizations sr
JOIN scenarios s ON s.id = sr.scenario_id
JOIN payable_transaction_links l
  ON l.payable_id = s.payable_id AND l.transaction_id = sr.transaction_id
WHERE sr.relation_type = 'allocation';
