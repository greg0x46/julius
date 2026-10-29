# Legacy database conversion

This binary initializes a new schema generation on empty databases. Postgres
stores money as exact `NUMERIC`, instants as `TIMESTAMPTZ`, calendar days as
`DATE`, flags as `BOOLEAN`, and UUID identities as `UUID`; SQLite retains exact
text money and UTC timestamps. Both dialects use the same migration numbers.

**Existing Postgres installations:** startup automatically converts the final
legacy schema (version 46) into generation 1 in the same database. Stop old
application instances before upgrading and retain an external database backup.
The database role needs permission to create schemas. A transaction locks the
legacy tables, builds the native baseline, copies and verifies every converted
row, then activates it without changing the DSN. The original tables, migration
history and append-only trigger function remain in a `julius_legacy_<id>` schema;
the startup log identifies it. Concurrent startups serialize, and subsequent
starts skip conversion. Any invalid value, schema mismatch or constraint failure
rolls back the entire operation and prevents startup with the original intact.
Older Postgres schemas must first reach version 46 using the legacy binary.
The backup schema is retained indefinitely; do not delete it before acceptance.

**Existing SQLite installations:** databases from the old migration sequence are
rejected at startup, without applying migrations. Use the one-shot
`cmd/migrate-legacy` tool with a separate target; it replaces
`cmd/migrate-to-postgres`. Keep the old binary, database and external master key.
Do not change the old database's migration version to bypass the guard.

1. Stop Julius and make a consistent SQLite backup. The source must have reached
   the final legacy schema (SQLite migration `00047_manual_bank_accounts`); if
   necessary, upgrade it with the previous binary before taking the backup.
2. Choose a new SQLite path or a fresh Postgres database. Keep both applications
   stopped throughout conversion. The tool initializes the target schema and
   refuses existing application data, except untouched baseline category seeds.
3. Run a dry-run against the backup:

   ```sh
   go run ./cmd/migrate-legacy -sqlite legacy-backup.db -target converted.db -dry-run
   # For Postgres, replace converted.db with a postgres:// DSN.
   ```

   It reports all malformed rows and constraint errors together, with table,
   row identity and value. Require `zero errors` and matching row counts,
   per-account balances and movement sums. Imported data and replaced category
   seeds roll back; the new target schema remains initialized.
4. Repeat the same command without `-dry-run`. Require `copy complete; counts
   and balances match` before starting Julius with the target database and the
   same master key. Check accounts, transactions, decisions and sign-in before
   retiring the previous installation.

The source opens read-only without applying migrations. IDs, encrypted settings,
raw payloads, sessions and user decisions carry over; auth timestamps and calendar
fields follow the documented [conversion contract](../development/schema/inventory.md).
Postgres rejects legacy IDs that cannot fit its UUID columns; SQLite preserves
opaque legacy IDs. No rounding or guessed values are used. An interrupted or
failed real copy can leave earlier tables committed: discard only the separate
target and retry with a fresh one. Keep the source untouched as the rollback
backup. This tool is temporary and should be removed after cutover.
