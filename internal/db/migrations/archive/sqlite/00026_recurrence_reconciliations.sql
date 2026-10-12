-- +goose Up

-- recurrence_reconciliations stores ONE thing: a user's manual decision about
-- a single occurrence of a recurring commitment. It is deliberately not the
-- reconciliation state — that stays recomputed on every read (principle 1 of
-- .specs/motores-de-dominio.md), now with this table consulted *before* the
-- automation rule. With no row here, an occurrence resolves exactly as it did
-- before this table existed.
--
-- It is the explicit link table principle 2 calls for (same shape as
-- payable_transaction_links / scenario_transaction_realizations): the real
-- transaction still never "knows" a commitment is using it for planning.
CREATE TABLE recurrence_reconciliations (
    id                      TEXT PRIMARY KEY,
    recurring_commitment_id TEXT NOT NULL REFERENCES recurring_commitments (id) ON DELETE CASCADE,
    -- The occurrence's calendar day (YYYY-MM-DD), which is the only identity
    -- an occurrence has: they are generated from the commitment's scheduling
    -- fields, never stored (see internal/recurrences/occurrence.go).
    occurrence_date         TEXT NOT NULL,
    state                   TEXT NOT NULL CHECK (state IN ('linked', 'detached')),
    -- CASCADE, not the RESTRICT payable_transaction_links uses: a
    -- reconciliation is an annotation about a fact, not the record of a debt
    -- being paid down — if the transaction goes, the annotation is meaningless.
    transaction_id          TEXT REFERENCES financial_transactions (id) ON DELETE CASCADE,
    created_at              TEXT NOT NULL,
    UNIQUE (recurring_commitment_id, occurrence_date),
    -- One transaction settles at most one occurrence, or the same money would
    -- suppress two projections. Multiple NULLs don't collide in either
    -- dialect, so 'detached' rows coexist freely.
    UNIQUE (transaction_id),
    CHECK ((state = 'linked'   AND transaction_id IS NOT NULL) OR
           (state = 'detached' AND transaction_id IS NULL))
);

CREATE INDEX idx_recurrence_reconciliations_commitment
    ON recurrence_reconciliations (recurring_commitment_id, occurrence_date);

-- +goose Down

DROP TABLE recurrence_reconciliations;
