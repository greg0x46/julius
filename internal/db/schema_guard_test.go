package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestOpenRejectsLegacySchema(t *testing.T) {
	for _, backend := range []struct {
		name string
		head int64
		open func(*testing.T, int64) (*sql.DB, *goose.Provider)
	}{
		{"sqlite", 47, migrationProviderAt}, {"postgres", 46, postgresMigrationProviderAt},
	} {
		for _, version := range []int64{1, backend.head} {
			t.Run(backend.name+"/"+stringVersion(version), func(t *testing.T) {
				conn, _ := backend.open(t, version)
				var before int
				if err := conn.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&before); err != nil {
					t.Fatal(err)
				}
				dsn := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
				if backend.name == "sqlite" {
					var seq int
					var name string
					if err := conn.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &dsn); err != nil {
						t.Fatal(err)
					}
					if err := Migrate(conn); err == nil || !strings.Contains(err.Error(), "legacy database schema") {
						t.Fatalf("Migrate error=%v", err)
					}
				}
				opened, err := Open(dsn)
				if err == nil {
					opened.Close()
					t.Fatal("Open migrated a legacy database")
				}
				if !strings.Contains(err.Error(), "unsupported legacy database schema") {
					t.Fatalf("unclear rejection: %v", err)
				}
				var after int
				if err := conn.QueryRow(`SELECT COUNT(*) FROM goose_db_version`).Scan(&after); err != nil {
					t.Fatal(err)
				}
				if before != after {
					t.Fatalf("migration history changed from %d to %d rows", before, after)
				}
				if _, err := conn.Exec(`INSERT INTO julius_schema_version(id,generation) VALUES(1,1)`); err == nil {
					t.Fatal("rejection created a generation marker")
				}
			})
		}
	}
}

func stringVersion(version int64) string {
	if version == 1 {
		return "first-version"
	}
	return "fully-migrated"
}

func TestSchemaGuardRejectsUnknownGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	mustExec(t, conn, `CREATE TABLE julius_schema_version(id INTEGER PRIMARY KEY,generation INTEGER NOT NULL)`)
	mustExec(t, conn, `INSERT INTO julius_schema_version VALUES(1,2)`)
	if opened, err := Open(path); err == nil {
		opened.Close()
		t.Fatal("accepted unknown generation")
	} else if !strings.Contains(err.Error(), "generation 2") {
		t.Fatal(err)
	}
}

func TestSQLiteBaselineEnforcesForeignKeys(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var enabled int
	if err := conn.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if enabled != 1 {
		t.Fatal("foreign keys are disabled")
	}
	_, err = conn.Exec(`INSERT INTO sync_runs(id,source_id,started_at) VALUES('run','missing','2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Fatal("insert accepted a missing source")
	}
}
