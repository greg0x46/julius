-- +goose Up
CREATE TABLE manual_investment_movements (
    id TEXT NOT NULL PRIMARY KEY,
    investment_id TEXT NOT NULL REFERENCES financial_investments (id) ON DELETE RESTRICT,
    movement_type TEXT NOT NULL CHECK (movement_type IN ('BUY', 'SELL', 'INTEREST')),
    direction TEXT NOT NULL CHECK (direction IN ('inflow', 'outflow')),
    trade_date TEXT NOT NULL,
    quantity TEXT,
    unit_price TEXT,
    amount TEXT NOT NULL,
    notes TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX ix_manual_investment_movements_investment_id ON manual_investment_movements (investment_id);

-- +goose Down
DROP TABLE manual_investment_movements;
