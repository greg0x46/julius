-- +goose Up

-- Marks a row written by internal/networth.Backfill (reconstructed from past
-- transactions) rather than internal/networth.Snapshot (a live read of
-- today's provider-synced balances). The frontend uses this to note that
-- investment_balance is always 0 on a backfilled row — see Backfill's doc
-- comment for why investment history can't be reconstructed.
ALTER TABLE net_worth_snapshots ADD COLUMN is_backfilled INTEGER NOT NULL DEFAULT 0 CHECK (is_backfilled IN (0, 1));

-- +goose Down

ALTER TABLE net_worth_snapshots DROP COLUMN is_backfilled;
