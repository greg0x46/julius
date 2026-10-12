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
version 1. `db.Open` and `db.Migrate` reject application databases without that
marker before applying migrations. This binary initializes empty databases and
reopens generation 1 databases; it does **not** convert existing installations.
Keep the old binary and a backup until a separate legacy conversion tool is
available. Never delete the old database or rewrite its Goose version to bypass
the guard. The SQLite-to-Postgres copier also cannot convert legacy generations.

`archive/sqlite` and `archive/postgres` retain the original migrations unchanged.
They are embedded only in tests that pin historical data migrations and rollbacks,
not in the application binary. Their independent numbering is historical and
must never be reused by the active runner.

## Validation

`TestBaselineSchemaParity` applies both baselines and compares tables, columns,
nullability, index definitions, checks and foreign keys. It normalizes equivalent
dialect syntax and native type-domain checks; `TestBaselineNativeTypes` separately
pins every money/decimal/date/instant/boolean column against the inventory.
Postgres tests require a disposable `JULIUS_TEST_POSTGRES_DSN`. Guard tests cover
both the first and final legacy versions, unchanged migration history, and an
unsupported generation.

SQLite table rebuilds should remain transactional where possible. Turning off
foreign keys requires `-- +goose NO TRANSACTION`; prefer ordering inserts and
using a transaction over leaving a half-applied schema after a failure.
