-- +goose Up

-- SQLite can't ALTER a CHECK constraint in place — rebuild scenarios so
-- payable_id is nullable for kind='standalone' while staying required for
-- debt_plan/receivable_plan, per .specs/relatorio-financeiro/m2-cenarios-standalone.md.

CREATE TABLE scenarios_new (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    payable_id TEXT REFERENCES payables (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (kind = 'standalone' AND payable_id IS NULL) OR
        (kind IN ('debt_plan', 'receivable_plan') AND payable_id IS NOT NULL)
    )
);
INSERT INTO scenarios_new SELECT * FROM scenarios;
DROP TABLE scenarios;
ALTER TABLE scenarios_new RENAME TO scenarios;

-- +goose Down

CREATE TABLE scenarios_old (
    id         TEXT PRIMARY KEY,
    kind       TEXT NOT NULL,
    name       TEXT NOT NULL,
    payable_id TEXT REFERENCES payables (id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (payable_id IS NOT NULL)
);
INSERT INTO scenarios_old SELECT * FROM scenarios WHERE kind != 'standalone';
DROP TABLE scenarios;
ALTER TABLE scenarios_old RENAME TO scenarios;
