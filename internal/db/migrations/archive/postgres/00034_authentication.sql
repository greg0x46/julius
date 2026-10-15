-- +goose Up
CREATE TABLE auth_owner (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    email TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    revision INTEGER NOT NULL DEFAULT 1,
    created_at BIGINT NOT NULL
);
CREATE TABLE auth_sessions (
    token_hash TEXT PRIMARY KEY,
    owner_id INTEGER NOT NULL REFERENCES auth_owner(id),
    revision INTEGER NOT NULL,
    created_at BIGINT NOT NULL,
    last_seen_at BIGINT NOT NULL,
    expires_at BIGINT NOT NULL
);
CREATE INDEX auth_sessions_expiry ON auth_sessions(expires_at);
-- +goose Down
DROP TABLE auth_sessions;
DROP TABLE auth_owner;
