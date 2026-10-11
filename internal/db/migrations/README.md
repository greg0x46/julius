# Migrations

Two independent goose sequences, one per dialect. `internal/db` picks the
directory from the DSN and each database tracks its own version, so the two
sequences never meet at runtime.

## Numbers differ between dialects

The same logical step has different numbers in `sqlite/` and `postgres/`. This
is intentional: the sequences diverged early (SQLite got migrations Postgres
never needed and vice versa — `00024_drop_stale_payables_receivable_column`
exists only in Postgres), and renumbering history would break any database
already migrated. Since `00025`, Postgres is exactly **one number behind**
SQLite for the same steps:

| logical step | sqlite | postgres |
|---|---|---|
| `recurrence_reconciliations` | 00026 | 00025 |
| `scenario_projection_unification` | 00027 | 00026 |
| `drop_scenario_transaction_realizations` | 00028 | 00027 |
| `drop_recurrence_reconciliations` | 00029 | 00028 |
| `drop_recurring_commitments` | 00030 | 00029 |

**When adding a migration:** match by *suffix* (the step name), never by number.
If the step applies to both databases, create both files with the same suffix
and the next number **of each** directory.

## SQLite: `PRAGMA foreign_keys`

A table rebuild (`CREATE ... _new` / copy / `DROP` / `RENAME`) only needs to
turn FKs off when **another** table references the one being rebuilt, or when a
row must point at one that does not exist yet. Otherwise leave FKs on and keep
the migration inside goose's transaction — that is what gives a real rollback
if something fails midway. `PRAGMA foreign_keys` is ignored inside a
transaction, so using it forces `-- +goose NO TRANSACTION`, and then a
mid-migration failure leaves the schema half-applied *and* a pooled connection
with FKs off. Prefer reordering the inserts (see
`00030_drop_recurring_commitments.sql`) over paying that price.
