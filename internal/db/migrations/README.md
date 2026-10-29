# Migrations

SQLite and Postgres share one numbered Goose sequence. Each dialect starts at
`0001_initial.sql`, representing the final application schema; add the same
number and suffix in both directories for every subsequent change. Migrations
remain forward-only: never modify an applied migration.

## Storage contract

Postgres uses unconstrained `NUMERIC` for exact money, quantities and rates,
`TIMESTAMPTZ` for UTC instants, `DATE` for calendar days, `BOOLEAN` for flags and
`UUID` for UUID identities and their links. Opaque provider identifiers, hashes,
keys and integrated investment-account IDs (`integrated:<source id>`) remain
text. SQLite uses exact decimal and UTC timestamp text, date text and integer
0/1 flags. Both enforce the same enums, currency codes, relationships and
partial uniqueness; all foreign keys declare their delete behavior.

No decimal check casts to floating point. SQLite's positive/nonnegative checks
validate decimal text and inspect its sign/nonzero digits; Postgres compares
native numeric values and rejects non-finite numbers. Reads use `internal/db/types`
to normalize native values without introducing floating point. Authentication
instants now use the same timestamp representation as other stores.

## Existing databases

`julius_schema_version` distinguishes this schema generation from legacy Goose
version 1. `db.Open` automatically converts final legacy Postgres (version 46)
before running the new migration sequence. It locks the source tables, creates
the baseline in a temporary schema, copies in FK order and checks counts and
complete converted row equality. Activation, fresh Goose history and conversion
commit together; all failures roll back. Original tables, old history and the
append-only trigger function remain in a `julius_legacy_<id>` schema recorded in
startup logs. An advisory transaction lock serializes concurrent conversions.
The same DSN and master key continue to work. Stop old instances before upgrade;
retain an external backup and grant the application role schema creation rights.
Incomplete legacy Postgres and legacy SQLite are rejected without mutation.
Empty databases initialize normally and generation 1 databases reopen normally.
Use `cmd/migrate-legacy` with a read-only final legacy SQLite backup and a
separate fresh SQLite/Postgres target; see the [legacy conversion guide](../../../docs/self-hosting/legacy-conversion.md).
Keep the old binary, database and master key until cutover is validated. Never
delete the old database or rewrite its Goose version to bypass the guard. The
generic SQLite-to-Postgres copier has been replaced by this temporary tool.
It converts the inventory allowlist, checks native types, fills new columns from
baseline defaults, and explicitly skips obsolete tables and Goose metadata.
Real copies use one transaction per table; dry-run uses row savepoints in one
rollback transaction so it can check FKs and report all row errors. Validation
compares every surviving table count, native account balance and account movement
sum with exact decimal arithmetic. Targets with application data or edited seeds
are refused; a failed real copy requires a fresh target for retry.

`archive/sqlite` and `archive/postgres` retain the original migrations unchanged.
They are embedded only in tests that pin historical data migrations and rollbacks,
not in the application binary. Their independent numbering is historical and
must never be reused by the active runner. Historical `.specs/` paths in their
SQL comments intentionally retain the original context; they are not live links.
Maintained references are in [development docs](../../../docs/development/README.md).

## Validation

`TestBaselineSchemaParity` applies both baselines and compares tables, columns,
nullability, index definitions, checks and foreign keys. It normalizes equivalent
dialect syntax and native type-domain checks; `TestBaselineNativeTypes` separately
pins every money/decimal/date/instant/boolean column against the inventory.
Postgres tests require a disposable `JULIUS_TEST_POSTGRES_DSN`. Guard tests cover
legacy SQLite and incomplete Postgres versions, unchanged migration history, and an
unsupported generation. Shared constraint tests also pin currency/enum domains,
boolean flags, and exact decimal round trips beyond floating-point range. The
financial invariant matrix links this precision rule to its tests.

SQLite table rebuilds should remain transactional where possible. Turning off
foreign keys requires `-- +goose NO TRANSACTION`; prefer ordering inserts and
using a transaction over leaving a half-applied schema after a failure.

`legacy_postgres_test.go` pins automatic Postgres conversion, exact balances,
encrypted values, retained backup, unchanged session deadlines, restart, atomic
rollback/retry, concurrent startups and calendar conventions.
