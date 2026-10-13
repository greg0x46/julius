-- Which way the money moved is a domain fact the yield math depends on, but
-- until now it was re-derived at read time by string-matching Pluggy's
-- movement_type against a hardcoded list ('SELL'/'REDEMPTION' = outflow,
-- everything else = inflow). That list was wrong for INTEREST: a dividend or
-- JCP payout leaves the investment, and counting it as a contribution made
-- healthy positions report negative yields (HGLG11, XINA11 in production).
--
-- Pluggy sends the direction explicitly as movementType (CREDIT = into the
-- investment, DEBIT = out of it) and we were discarding it. Normalizing it
-- once at ingestion into inflow/outflow means the money math never has to
-- know a provider's vocabulary again — the point being that investments stay
-- meaningful with Pluggy only as a registration convenience.
--
-- Nullable on purpose: a movement whose direction we could not establish
-- must make the yield unavailable, not silently pick a side.

-- +goose Up

ALTER TABLE financial_investment_transactions ADD COLUMN direction TEXT
    CHECK (direction IN ('inflow', 'outflow'));

-- Backfill from the movement types actually observed. INTEREST lands on
-- outflow — the correction this migration exists for. Anything else stays
-- NULL rather than being guessed; the next sync fills it from movementType.
UPDATE financial_investment_transactions
SET direction = CASE movement_type
    WHEN 'BUY' THEN 'inflow'
    WHEN 'APPLICATION' THEN 'inflow'
    WHEN 'TRANSFER_IN' THEN 'inflow'
    WHEN 'SELL' THEN 'outflow'
    WHEN 'REDEMPTION' THEN 'outflow'
    WHEN 'INTEREST' THEN 'outflow'
    WHEN 'DIVIDEND' THEN 'outflow'
    WHEN 'INCOME' THEN 'outflow'
    WHEN 'AMORTIZATION' THEN 'outflow'
    WHEN 'TRANSFER_OUT' THEN 'outflow'
END
WHERE direction IS NULL;

-- +goose Down

ALTER TABLE financial_investment_transactions DROP COLUMN direction;
