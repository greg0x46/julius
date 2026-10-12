-- +goose Up

ALTER TABLE scenarios DROP CONSTRAINT scenarios_payable_id_check;
ALTER TABLE scenarios ADD CONSTRAINT scenarios_payable_id_check CHECK (
    (kind = 'standalone' AND payable_id IS NULL) OR
    (kind IN ('debt_plan', 'receivable_plan') AND payable_id IS NOT NULL)
);

-- +goose Down

ALTER TABLE scenarios DROP CONSTRAINT scenarios_payable_id_check;
ALTER TABLE scenarios ADD CONSTRAINT scenarios_payable_id_check CHECK (payable_id IS NOT NULL);
