-- +goose Up
-- Existing clients are dated at migration time, so none is stale at once.
ALTER TABLE oauth_clients ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT now();

-- +goose Down
ALTER TABLE oauth_clients DROP COLUMN created_at;
