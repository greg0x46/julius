# Backups and upgrades

Keep the database, external master key, protected configuration, and the
matching binary version recoverable. Test restoration before relying on a
backup. Database backups contain private financial data and must be protected.

## Back up

Stop all Julius processes and wait for exit before taking an offline backup.
Keep them stopped until the backup completes.

### SQLite

With the `sqlite3` CLI installed, create a consistent backup of the database
selected by `-db` or `JULIUS_DB`. For the getting-started path:

```sh
mkdir -p backups
chmod 700 backups
(umask 077; sqlite3 ./julius.db ".backup './backups/julius-before-upgrade.sqlite'")
sqlite3 ./backups/julius-before-upgrade.sqlite 'PRAGMA integrity_check;'
```

Require `ok`; use a new backup filename for each backup you retain. The SQLite
backup command includes committed WAL data. Do not copy only a live `.db` file
or delete its `-wal`/`-shm` files: committed data may still be in the WAL.

### PostgreSQL

Use PostgreSQL client tools compatible with your server and its normal secure
credential mechanism. For example, with `PGHOST`, `PGUSER`, `PGDATABASE`, and
TLS configured for the same database Julius uses:

```sh
(umask 077; pg_dump --format=custom --file=julius-before-upgrade.dump)
pg_restore --list julius-before-upgrade.dump
```

Keep all Julius instances stopped during the upgrade. A dump listing is only
a readability check; verify recovery by restoring to a separate database.

For either database, save the master key **separately** and retain the previous
binary plus its origin, database, and service settings. Copy backups off the
application host using protected storage.

## Upgrade safely

1. Read the selected [release notes](https://github.com/greg0x46/julius/releases)
   and check the legacy caveats below before changing an existing installation.
2. Download and verify the new binary using [installation](installation.md),
   or build the selected source version. Keep the old binary.
3. Stop all instances and [back up](#back-up) before starting the new binary.
4. Replace the binary and restart with the same database, master key, and
   public origin. Pending migrations run automatically; no separate migration
   command is needed for the current schema generation.
5. Check startup logs, sign in, and verify accounts, transactions, balances,
   and integration settings before resuming normal use.

Do not assume an older binary can use a database migrated by a newer version.
For rollback, stop all instances and restore the pre-upgrade database with its
matching binary, key, and settings. A published release does not update your
installation automatically.

## Restore

Restore to a **new** location while Julius is stopped, preserving the failed
database for investigation. For SQLite, use an unused path with no stale WAL
or SHM files:

```sh
cp ./backups/julius-before-upgrade.sqlite ./julius-restored.db
chmod 600 ./julius-restored.db
sqlite3 ./julius-restored.db 'PRAGMA integrity_check;'
```

Require `ok`, then point `-db` or `JULIUS_DB` to the restored file. Ensure the
service user can read and write it and its directory.

For PostgreSQL, create a fresh empty database owned by the application role,
then restore using your secured PostgreSQL connection settings:

```sh
pg_restore --exit-on-error --no-owner --dbname=julius_restored julius-before-upgrade.dump
```

Point `JULIUS_DB` to that database. Start the binary matching the backup with
the original master key, then check login, balances, transactions, and settings.
Do not run `auth init` against a restored database; its owner is already stored.

## Legacy installations

Old schema generations require a separate cutover:

- **SQLite:** startup rejects the old migration sequence. Keep the old source
  untouched, bring it to final legacy migration 47 with the legacy binary,
  then use `cmd/migrate-legacy` against a backup and a separate fresh target.
  Run a dry-run first and require matching counts and balances.
- **PostgreSQL:** startup converts final legacy schema version 46 in place.
  Stop old instances and take an external backup first. The database role
  needs permission to create schemas; original tables remain in a
  `julius_legacy_<id>` schema. Older schemas must first reach version 46 with
  the legacy binary.

Follow the full [schema conversion procedure](../legacy-readme.md#database-schema-generations)
for these exceptional upgrades; do not bypass the schema guard or manually
change migration versions.

When moving from Contadinho, replace `CONTADINHO_*` variables with `JULIUS_*`;
the old names are not read. A database still using the old unlock password
also needs the one-time interactive `./julius auth migrate -db ./julius.db`
with its old unlock password and a new owner email/password. Follow the
[authentication migration procedure](../legacy-readme.md#authentication-and-encryption-key)
after schema conversion, keeping the old backup for rollback.
