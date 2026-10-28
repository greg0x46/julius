-- +goose Up
CREATE TABLE oauth_clients (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    redirect_uris TEXT NOT NULL
);
CREATE TABLE oauth_grants (
    token_hash TEXT PRIMARY KEY,
    kind TEXT NOT NULL CHECK (kind IN ('consent', 'code', 'access')),
    client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    redirect_uri TEXT NOT NULL,
    resource TEXT NOT NULL,
    scope TEXT NOT NULL,
    challenge TEXT NOT NULL,
    state TEXT NOT NULL,
    csrf_hash TEXT NOT NULL,
    revision INTEGER NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE INDEX ix_oauth_grants_expiry ON oauth_grants(expires_at);

-- +goose Down
DROP TABLE oauth_grants;
DROP TABLE oauth_clients;
