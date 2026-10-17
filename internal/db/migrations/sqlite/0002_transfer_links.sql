-- +goose Up
CREATE TABLE transfer_links (
    id TEXT PRIMARY KEY,
    created_at TEXT NOT NULL
);
CREATE TABLE transfer_transaction_links (
    transaction_id TEXT PRIMARY KEY REFERENCES financial_transactions(id) ON DELETE RESTRICT,
    transfer_id TEXT NOT NULL REFERENCES transfer_links(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('source', 'destination', 'fee')),
    UNIQUE (transfer_id, role)
);

-- +goose Down
DROP TABLE transfer_transaction_links;
DROP TABLE transfer_links;
