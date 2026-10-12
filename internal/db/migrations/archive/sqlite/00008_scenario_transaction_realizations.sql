-- +goose Up

-- Allocates (part of) a real debt_transaction_links row to a planned
-- scenario_transaction — see .specs/plano-pagamento-e-cenarios-projecao.md.
-- Many-to-many on purpose: a single real link can be split across several
-- planned installments (a lump-sum payment covering two months at once),
-- and a single installment can receive allocations from more than one link
-- (a payment split across two real transactions).
CREATE TABLE scenario_transaction_realizations (
    id                       TEXT PRIMARY KEY,
    scenario_transaction_id  TEXT NOT NULL REFERENCES scenario_transactions (id) ON DELETE CASCADE,
    debt_link_id             TEXT NOT NULL REFERENCES debt_transaction_links (id) ON DELETE CASCADE,
    allocated_amount         TEXT NOT NULL CHECK (CAST(allocated_amount AS REAL) > 0),
    created_at               TEXT NOT NULL
);

CREATE INDEX idx_str_scenario_transaction_id ON scenario_transaction_realizations (scenario_transaction_id);
CREATE INDEX idx_str_debt_link_id ON scenario_transaction_realizations (debt_link_id);

-- +goose Down

DROP TABLE scenario_transaction_realizations;
