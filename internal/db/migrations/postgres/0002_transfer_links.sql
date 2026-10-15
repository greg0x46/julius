-- +goose Up
CREATE TABLE transfer_links (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL
);
CREATE TABLE transfer_transaction_links (
    transaction_id UUID PRIMARY KEY REFERENCES financial_transactions(id) ON DELETE RESTRICT,
    transfer_id UUID NOT NULL REFERENCES transfer_links(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('source', 'destination', 'fee')),
    UNIQUE (transfer_id, role)
);

-- +goose Down
DROP TABLE transfer_transaction_links;
DROP TABLE transfer_links;
