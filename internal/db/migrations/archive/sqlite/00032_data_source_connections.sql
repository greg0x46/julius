-- Several Pluggy connections instead of one. The schema was always
-- multi-source — every financial table is keyed by source_id, and
-- uq_sync_runs_active_source is partial per source_id so two connections can
-- sync without blocking each other — but the only item id the app knew was
-- settings['pluggy.item_id'], a scalar the setup screen wrote once and no
-- route could change. This promotes data_sources itself to the registry of
-- connections, so adding a second bank is a row rather than a reinstall.
--
-- label is the user's own name for a connection, deliberately separate from
-- display_name: sync overwrites display_name with the institution the
-- provider reports on every run, so two items at the same bank (personal and
-- business, say) would otherwise be indistinguishable in every list.

-- +goose Up

ALTER TABLE data_sources ADD COLUMN is_active INTEGER NOT NULL DEFAULT 1 CHECK (is_active IN (0, 1));
ALTER TABLE data_sources ADD COLUMN label TEXT;

-- Carry the single configured item over as the first connection. A row may
-- already exist for it — handleCreateSyncRun created one lazily on the first
-- sync — hence the NOT EXISTS guard; only a database set up but never synced
-- needs the insert.
INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at, is_active)
SELECT
    lower(
        substr(hex(randomblob(4)), 1, 8) || '-' ||
        substr(hex(randomblob(2)), 1, 4) || '-4' ||
        substr(hex(randomblob(2)), 2, 3) || '-' ||
        substr('89ab', abs(random()) % 4 + 1, 1) ||
        substr(hex(randomblob(2)), 2, 3) || '-' ||
        substr(hex(randomblob(6)), 1, 12)
    ),
    'pluggy', s.value, s.updated_at, s.updated_at, 1
FROM settings s
WHERE s.key = 'pluggy.item_id'
  AND s.value IS NOT NULL
  AND s.value <> ''
  AND NOT EXISTS (
      SELECT 1 FROM data_sources d
      WHERE d.provider = 'pluggy' AND d.external_item_id = s.value
  );

DELETE FROM settings WHERE key = 'pluggy.item_id';

-- +goose Down

-- Lossy on purpose: the old shape holds exactly one item, so every connection
-- past the first is dropped. The oldest one wins, since it is the one the
-- setup screen would have configured.
DELETE FROM settings WHERE key = 'pluggy.item_id';

INSERT INTO settings (key, value, is_encrypted, updated_at)
SELECT 'pluggy.item_id', external_item_id, 0, updated_at
FROM data_sources
WHERE provider = 'pluggy'
ORDER BY created_at, id
LIMIT 1;

ALTER TABLE data_sources DROP COLUMN label;
ALTER TABLE data_sources DROP COLUMN is_active;
