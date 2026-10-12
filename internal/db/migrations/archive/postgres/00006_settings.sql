-- +goose Up

-- Application configuration, replacing the Python reference's .env file.
-- Holds both plain config (e.g. sync intervals) and Pluggy credentials,
-- edited from the frontend setup screen instead of environment variables.
-- Sensitive values (is_encrypted = 1) hold base64(nonce || AES-256-GCM
-- ciphertext) in `value`, keyed by app_auth's password-derived key — see
-- package settings.
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT,
    is_encrypted INTEGER NOT NULL DEFAULT 0 CHECK (is_encrypted IN (0, 1)),
    updated_at TEXT NOT NULL
);

-- Singleton row (id is always 1): the Argon2id salt and a verifier blob used
-- to confirm a candidate password derives the right key before trusting it
-- to decrypt anything else in `settings`. Absence of this row means the app
-- has never been set up.
CREATE TABLE app_auth (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    salt TEXT NOT NULL,
    verifier TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

-- +goose Down

DROP TABLE app_auth;
DROP TABLE settings;
