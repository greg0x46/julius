-- +goose Up
-- SQLite cannot add a column with a non-constant default, so the default is a
-- sentinel for rows inserted without a value; registration always sets one.
ALTER TABLE oauth_clients ADD COLUMN created_at TEXT NOT NULL DEFAULT '1970-01-01T00:00:00.000000000Z';
-- Existing clients are dated at migration time, so none is stale at once.
UPDATE oauth_clients SET created_at = strftime('%Y-%m-%dT%H:%M:%S', 'now') || '.000000000Z';

-- +goose Down
ALTER TABLE oauth_clients DROP COLUMN created_at;
