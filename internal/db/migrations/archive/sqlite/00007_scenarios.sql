-- +goose Up

-- scenarios / scenario_transactions materialize a hypothetical, editable
-- projection layered on top of real data — see
-- .specs/plano-pagamento-e-cenarios-projecao.md. They are purely additive:
-- nothing about financial_transactions or debts' own paid/remaining
-- computation changes because these tables exist.
CREATE TABLE scenarios (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    debt_id    TEXT REFERENCES debts (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (kind != 'debt_plan' OR debt_id IS NOT NULL)
);

CREATE TABLE scenario_transactions (
    id           TEXT PRIMARY KEY,
    scenario_id  TEXT NOT NULL REFERENCES scenarios (id) ON DELETE CASCADE,
    description  TEXT NOT NULL,
    amount       TEXT NOT NULL,
    projected_at TEXT NOT NULL,
    category     TEXT,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

CREATE INDEX idx_scenario_transactions_scenario_id ON scenario_transactions (scenario_id);

-- +goose Down

DROP TABLE scenario_transactions;
DROP TABLE scenarios;
