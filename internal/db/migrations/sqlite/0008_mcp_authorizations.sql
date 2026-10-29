-- +goose Up
CREATE TABLE mcp_authorizations (
    id TEXT PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('mcp:access', 'mcp:access mcp:write', 'mcp:access mcp:write mcp:operations')),
    approved_scope TEXT NOT NULL CHECK (approved_scope IN ('mcp:access', 'mcp:access mcp:write', 'mcp:access mcp:write mcp:operations')),
    revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    last_used_at TEXT,
    expires_at TEXT NOT NULL,
    revoked_at TEXT,
    CHECK (
        scope = approved_scope OR scope = 'mcp:access'
        OR (scope = 'mcp:access mcp:write' AND approved_scope = 'mcp:access mcp:write mcp:operations')
    )
);
ALTER TABLE oauth_grants ADD COLUMN authorization_id TEXT REFERENCES mcp_authorizations(id) ON DELETE CASCADE;
CREATE INDEX ix_oauth_grants_authorization ON oauth_grants(authorization_id);

-- +goose Down
DROP INDEX ix_oauth_grants_authorization;
ALTER TABLE oauth_grants DROP COLUMN authorization_id;
DROP TABLE mcp_authorizations;
