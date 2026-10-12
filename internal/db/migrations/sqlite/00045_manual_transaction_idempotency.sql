-- +goose Up
CREATE TABLE manual_transaction_idempotency (
    owner_id INTEGER NOT NULL,
    idempotency_key TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    transaction_id TEXT REFERENCES financial_transactions(id),
    created_at TEXT NOT NULL,
    PRIMARY KEY (owner_id, idempotency_key)
);

-- +goose Down
DROP TABLE manual_transaction_idempotency;
