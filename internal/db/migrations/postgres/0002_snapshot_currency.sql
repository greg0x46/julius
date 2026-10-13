-- +goose Up
ALTER TABLE net_worth_snapshots ADD COLUMN valuation_currency TEXT NOT NULL DEFAULT 'BRL' CHECK (valuation_currency = 'BRL');

-- +goose Down
ALTER TABLE net_worth_snapshots DROP COLUMN valuation_currency;
