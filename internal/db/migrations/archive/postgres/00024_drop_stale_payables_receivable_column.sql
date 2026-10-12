-- +goose Up

-- A Postgres database migrated from an early draft of 00022 can still carry
-- a "payables_receivable" NOT NULL column that draft briefly had before it
-- was dropped from the schema (receivables were deliberately excluded from
-- Breakdown — see internal/networth's package doc comment). No app code has
-- ever written to it, so every INSERT into net_worth_snapshots since has
-- been failing its NOT NULL constraint on such a database. IF EXISTS makes
-- this a no-op on a database that never had the column (the common case).
ALTER TABLE net_worth_snapshots DROP COLUMN IF EXISTS payables_receivable;

-- +goose Down

-- Down is a no-op: recreating a column this app never wrote to, with no
-- data to restore, would only reintroduce the NOT NULL failure this
-- migration exists to fix.
