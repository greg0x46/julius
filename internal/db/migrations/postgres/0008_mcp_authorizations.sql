-- +goose Up
CREATE TABLE mcp_authorizations (
    id UUID PRIMARY KEY,
    client_id TEXT NOT NULL REFERENCES oauth_clients(id) ON DELETE CASCADE,
    scope TEXT NOT NULL CHECK (scope IN ('mcp:read', 'mcp:read mcp:write')),
    approved_scope TEXT NOT NULL CHECK (approved_scope IN ('mcp:read', 'mcp:read mcp:write')),
    revision INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    CHECK (scope = 'mcp:read' OR approved_scope = 'mcp:read mcp:write')
);
ALTER TABLE oauth_grants ADD COLUMN authorization_id UUID REFERENCES mcp_authorizations(id) ON DELETE CASCADE;
CREATE INDEX ix_oauth_grants_authorization ON oauth_grants(authorization_id);

-- +goose Down
DROP INDEX ix_oauth_grants_authorization;
ALTER TABLE oauth_grants DROP COLUMN authorization_id;
DROP TABLE mcp_authorizations;
