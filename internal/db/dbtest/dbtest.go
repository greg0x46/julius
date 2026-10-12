// Package dbtest opens migrated test databases on every supported dialect, so
// one test body can run against SQLite and, when JULIUS_TEST_POSTGRES_DSN is
// set, Postgres. It is meant for _test files only.
package dbtest

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/greg0x46/julius/internal/db"
)

// Backend opens a fresh, migrated database for one test.
type Backend struct {
	Name string
	Open func(t *testing.T) *sql.DB
}

// Backends lists SQLite and Postgres. The Postgres backend skips the test
// without JULIUS_TEST_POSTGRES_DSN and otherwise uses a private schema that
// is dropped when the test ends.
func Backends() []Backend {
	return []Backend{{Name: "sqlite", Open: OpenSQLite}, {Name: "postgres", Open: OpenPostgres}}
}

// Each runs fn as one subtest per backend.
func Each(t *testing.T, fn func(t *testing.T, conn *sql.DB)) {
	t.Helper()
	for _, backend := range Backends() {
		t.Run(backend.Name, func(t *testing.T) {
			fn(t, backend.Open(t))
		})
	}
}

// OpenSQLite opens a migrated SQLite database in a temporary file.
func OpenSQLite(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// OpenPostgres opens a migrated private schema of the Postgres at
// JULIUS_TEST_POSTGRES_DSN, skipping the test when it is not set.
func OpenPostgres(t *testing.T) *sql.DB {
	t.Helper()
	base := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if base == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set")
	}
	// Not db.Open: the schema named by base itself stays unmigrated.
	admin, err := sql.Open("pgx", base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	schema := "dbtest_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) })

	u, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	conn, err := db.Open(u.String())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

// IsPostgres reports whether conn talks to Postgres.
func IsPostgres(conn *sql.DB) bool {
	return !strings.Contains(fmt.Sprintf("%T", conn.Driver()), "sqlite")
}

// FailInserts makes every insert into table abort while when (a boolean SQL
// expression over NEW, valid on both dialects; "" means always) holds. It
// returns a func that removes the fault. when must not contain "?".
func FailInserts(t *testing.T, conn *sql.DB, table, when string) (drop func()) {
	t.Helper()
	if when == "" {
		when = "1 = 1"
	}
	name := "fail_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	var create, remove []string
	if IsPostgres(conn) {
		create = []string{
			`CREATE FUNCTION ` + name + `() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				IF (` + when + `) THEN
					RAISE EXCEPTION 'injected insert failure';
				END IF;
				RETURN NEW;
			END $$`,
			`CREATE TRIGGER ` + name + ` BEFORE INSERT ON ` + table + ` FOR EACH ROW EXECUTE FUNCTION ` + name + `()`,
		}
		remove = []string{`DROP TRIGGER ` + name + ` ON ` + table, `DROP FUNCTION ` + name + `()`}
	} else {
		create = []string{`CREATE TRIGGER ` + name + ` BEFORE INSERT ON ` + table + ` WHEN (` + when + `)
			BEGIN SELECT RAISE(ABORT, 'injected insert failure'); END`}
		remove = []string{`DROP TRIGGER ` + name}
	}
	for _, stmt := range create {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("install insert fault on %s: %v", table, err)
		}
	}
	return func() {
		t.Helper()
		for _, stmt := range remove {
			if _, err := conn.Exec(stmt); err != nil {
				t.Fatalf("remove insert fault on %s: %v", table, err)
			}
		}
	}
}
