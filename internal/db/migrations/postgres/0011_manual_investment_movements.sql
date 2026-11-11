-- +goose Up
CREATE TABLE manual_investment_movements (
    id UUID NOT NULL PRIMARY KEY,
    investment_id UUID NOT NULL REFERENCES financial_investments (id) ON DELETE RESTRICT,
    movement_type TEXT NOT NULL CHECK (movement_type IN ('BUY', 'SELL', 'INTEREST')),
    direction TEXT NOT NULL CHECK (direction IN ('inflow', 'outflow')),
    trade_date DATE NOT NULL,
    quantity NUMERIC,
    unit_price NUMERIC,
    amount NUMERIC NOT NULL,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    CHECK (quantity NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (unit_price NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric)),
    CHECK (amount NOT IN ('NaN'::numeric, 'Infinity'::numeric, '-Infinity'::numeric))
);

CREATE INDEX ix_manual_investment_movements_investment_id ON manual_investment_movements (investment_id);

-- +goose Down
DROP TABLE manual_investment_movements;
