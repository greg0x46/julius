package db

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// The round trip has to reach the three migrations that dropped the legacy
// allocation, reconciliation and recurring-commitment tables: their Down
// blocks rebuild a schema rather than just drop what they made, so they are
// the ones worth exercising.
//
// These name the oldest of those three per dialect, and the round trip rolls
// down to just before it. A version, not a count: counting back from the head
// meant every new migration silently narrowed the window until someone
// remembered to widen the constant, and a forgotten bump is invisible — the
// test still passes, just without covering what it was written for. The two
// dialects differ because Postgres' baseline collapses SQLite's early
// table-rebuild migrations, leaving its numbering one behind.
const (
	oldestDownMigrationSQLite   int64 = 28 // 00028_drop_scenario_transaction_realizations
	oldestDownMigrationPostgres int64 = 27 // 00027_drop_scenario_transaction_realizations
)

// TestDownUpRoundTrip applies every migration, rolls the drop migrations back
// and re-applies them. Nothing else in the suite runs a Down block at all, so
// without this a broken rollback would only be discovered by someone actually
// needing to roll back.
//
// It asserts that the round trip *runs*, not that it is lossless — it is not,
// deliberately: a recurring scenario with no category has no representation in
// the old recurring_commitments shape, which each Down block documents.
func TestDownUpRoundTrip(t *testing.T) {
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "roundtrip.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := migrate(conn, goose.DialectSQLite3, legacyMigrationsFS, "migrations/archive/sqlite"); err != nil {
		t.Fatalf("initial up: %v", err)
	}

	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/sqlite")
	if err != nil {
		t.Fatalf("sub fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.DownTo(ctx, oldestDownMigrationSQLite-1); err != nil {
		t.Fatalf("down to %d: %v", oldestDownMigrationSQLite-1, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("re-up after rollback: %v", err)
	}
}

// TestPostgresDownUpRoundTrip is the same round trip on the other dialect,
// where the drop migrations use ALTER TABLE instead of a table rebuild.
func TestPostgresDownUpRoundTrip(t *testing.T) {
	dsn := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set; skipping Postgres integration test")
	}
	// Open already applies the Postgres migrations, so the tree is fully
	// migrated by the time the rollback below starts.
	conn, _ := postgresMigrationProviderAt(t, 46)
	var err error
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/postgres")
	if err != nil {
		t.Fatalf("sub fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, migrations)
	if err != nil {
		t.Fatalf("provider: %v", err)
	}
	ctx := context.Background()
	if _, err := provider.DownTo(ctx, oldestDownMigrationPostgres-1); err != nil {
		t.Fatalf("down to %d: %v", oldestDownMigrationPostgres-1, err)
	}
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("re-up after rollback: %v", err)
	}
}
