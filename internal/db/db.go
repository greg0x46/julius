// Package db opens the application's database and applies migrations
// embedded into the binary, so a single executable needs no external schema
// or migration files at runtime. SQLite is the zero-config default; a
// "postgres://" or "postgresql://" DSN opts into Postgres instead, for
// deployments that run several julius instances against one shared
// database — see README "Rodando com Postgres". Postgres migrations are
// serialized with an advisory lock so instances starting together apply each
// migration exactly once.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"strings"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	_ "modernc.org/sqlite"
)

//go:embed migrations/sqlite/*.sql
var sqliteMigrationsFS embed.FS

//go:embed migrations/postgres/*.sql
var postgresMigrationsFS embed.FS

// DefaultPath returns the configured database path, falling back to an
// existing Contadinho database for installations upgrading to Julius.
// New installations use julius.db.
func DefaultPath() string {
	if configured := os.Getenv("JULIUS_DB"); configured != "" {
		return configured
	}
	if _, err := os.Stat("julius.db"); err == nil || !os.IsNotExist(err) {
		return "julius.db"
	}
	if _, err := os.Stat("contadinho.db"); err == nil {
		return "contadinho.db"
	}
	return "julius.db"
}

// Open opens the application database at dsn (a plain filesystem path, or
// ":memory:", selects SQLite; a "postgres://"/"postgresql://" URL selects
// Postgres) and applies the migrations embedded for that dialect.
func Open(dsn string) (*sql.DB, error) {
	if isPostgresDSN(dsn) {
		return openPostgres(dsn)
	}
	return openSQLite(dsn)
}

func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// openSQLite opens the SQLite database at path with WAL journaling, foreign
// keys enforced, and a busy timeout so concurrent access from the HTTP
// server and the sync worker doesn't fail immediately under lock contention.
func openSQLite(path string) (*sql.DB, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?" + url.Values{
			"_foreign_keys": {"1"},
			"_journal_mode": {"WAL"},
			"_busy_timeout": {"5000"},
			"_synchronous":  {"NORMAL"},
		}.Encode()
	}

	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite has no real concurrent-writer story: serialize all access
	// through a single connection so WAL readers never race a writer that
	// goose or the app opened on a second pooled connection.
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("enable sqlite foreign keys: %w", err)
	}

	if err := migrate(conn, goose.DialectSQLite3, sqliteMigrationsFS, "migrations/sqlite"); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// openPostgres opens conn through the qmark-rewriting connector in
// postgres.go, so callers keep writing the same `?` placeholders they use
// for SQLite. Unlike SQLite, Postgres has a real concurrent-writer story —
// exactly what lets several julius instances share one database — so
// the pool isn't pinned to a single connection. Its limits and lifetimes
// come from the JULIUS_DB_* variables in pool.go, with defaults when unset.
// Migrations run under a Postgres session advisory lock, so when several
// instances start at once only one applies pending migrations; the others
// wait (polling every second for up to ~5 minutes) and then find nothing
// left to apply.
func openPostgres(dsn string) (*sql.DB, error) {
	pool, err := poolConfigFromEnv(os.Getenv)
	if err != nil {
		return nil, fmt.Errorf("postgres pool config: %w", err)
	}
	connector, err := newPGConnector(dsn)
	if err != nil {
		return nil, err
	}
	conn := sql.OpenDB(connector)
	pool.apply(conn)

	locker, err := lock.NewPostgresSessionLocker(lock.WithLockTimeout(1, 300))
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create migration locker: %w", err)
	}
	if err := migrate(conn, goose.DialectPostgres, postgresMigrationsFS, "migrations/postgres", goose.WithSessionLocker(locker)); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// Migrate applies all pending embedded SQLite migrations to conn.
func Migrate(conn *sql.DB) error {
	return migrate(conn, goose.DialectSQLite3, sqliteMigrationsFS, "migrations/sqlite")
}

// migrate applies every pending migration under root. SQLite callers pass no
// options: goose rejects a session locker on a pool capped at one connection.
func migrate(conn *sql.DB, dialect goose.Dialect, migrationsFS embed.FS, root string, opts ...goose.ProviderOption) error {
	if !strings.Contains(root, "/archive/") {
		if err := guardSchema(conn, dialect); err != nil {
			return err
		}
	}
	migrations, err := fs.Sub(migrationsFS, root)
	if err != nil {
		return fmt.Errorf("root migrations fs: %w", err)
	}
	provider, err := goose.NewProvider(dialect, conn, migrations, opts...)
	if err != nil {
		return fmt.Errorf("create migration provider: %w", err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
