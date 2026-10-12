-- Pluggy sends three fields for FIXED_INCOME holdings (CDB/LCA) that were
-- previously parsed nowhere and silently dropped: amountOriginal (the
-- principal originally applied), taxes (IR provisioned) and taxes2 (IOF,
-- regressive, nonzero only in the holding's first 30 days) — all three
-- empirically verified against real Nubank data. The yield itself is
-- computed without them (investments_handlers.go's applyYield); these are the
-- detail the UI deducts from it to state the yield net, and shows as amount
-- invested, IR withheld and IOF withheld.
-- Pluggy never populates any of the three for EQUITY — expected, not a bug.
--
-- Note: investment_operations already has its own unrelated "taxes" column
-- (a single manual-ledger operation's tax amount) — different table, no
-- collision, just don't confuse the two later.

-- +goose Up

ALTER TABLE financial_investments ADD COLUMN amount_original TEXT;
ALTER TABLE financial_investments ADD COLUMN taxes TEXT;
ALTER TABLE financial_investments ADD COLUMN taxes2 TEXT;

-- +goose Down

ALTER TABLE financial_investments DROP COLUMN taxes2;
ALTER TABLE financial_investments DROP COLUMN taxes;
ALTER TABLE financial_investments DROP COLUMN amount_original;
