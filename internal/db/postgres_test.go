package db_test

import (
	"database/sql"
	"os"
	"strings"
	"sync"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/greg0x46/julius/internal/db"
)

// resetPostgresTestSchema skips unless JULIUS_TEST_POSTGRES_DSN points at a
// reachable Postgres server (see README "Rodando com Postgres" for a local
// docker one-liner). It wipes the public schema, leaving an empty database,
// and returns the DSN.
func resetPostgresTestSchema(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set; skipping Postgres integration test")
	}

	raw, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open raw pgx connection: %v", err)
	}
	if _, err := raw.Exec("DROP SCHEMA public CASCADE; CREATE SCHEMA public;"); err != nil {
		raw.Close()
		t.Fatalf("reset public schema: %v", err)
	}
	raw.Close()
	return dsn
}

// openPostgresTest opens a clean, freshly migrated Postgres database so every
// test function starts from the same state.
func openPostgresTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := resetPostgresTestSchema(t)

	conn, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func TestPostgresMigrateAppliesAllTables(t *testing.T) {
	conn := openPostgresTest(t)

	tables := []string{
		"data_sources", "sync_runs", "raw_imports", "financial_accounts",
		"financial_transactions", "normalization_events", "sync_failures",
		"automation_rules", "automation_rule_conditions",
		"transaction_inclusion_decisions", "transaction_inclusion_events",
		"categories", "transaction_category_decisions", "transaction_category_events",
		"payables", "payable_transaction_links", "settings", "app_auth",
		"scenarios", "scenario_transactions", "scenario_realizations",
		"scenario_recurring_schedules",
		"financial_investments", "financial_investment_transactions",
	}
	for _, table := range tables {
		var name string
		err := conn.QueryRow(
			"SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename = ?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %s missing: %v", table, err)
		}
	}
}

func TestPostgresMigrateIsIdempotent(t *testing.T) {
	openPostgresTest(t) // first Open already applied every migration once

	dsn := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	conn2, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	conn2.Close()
}

// TestPostgresConcurrentOpenAppliesMigrationsOnce simulates several instances
// starting against one empty database: the advisory lock must let exactly one
// of them apply each migration while the others wait and then skip.
func TestPostgresConcurrentOpenAppliesMigrationsOnce(t *testing.T) {
	dsn := resetPostgresTestSchema(t)

	const instances = 6
	conns := make([]*sql.DB, instances)
	errs := make([]error, instances)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			conns[i], errs[i] = db.Open(dsn)
		}()
	}
	close(start)
	wg.Wait()
	t.Cleanup(func() {
		for _, conn := range conns {
			if conn != nil {
				conn.Close()
			}
		}
	})
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Open #%d: %v", i, err)
		}
	}

	conn := conns[0]
	rows, err := conn.Query(
		"SELECT version_id, COUNT(*) FROM goose_db_version WHERE version_id > ? GROUP BY version_id HAVING COUNT(*) > ?", 0, 1,
	)
	if err != nil {
		t.Fatalf("query duplicate migrations: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var version int64
		var count int
		if err := rows.Scan(&version, &count); err != nil {
			t.Fatalf("scan duplicate migration: %v", err)
		}
		t.Errorf("migration %d applied %d times, want once", version, count)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate duplicate migrations: %v", err)
	}

	entries, err := os.ReadDir("migrations/postgres")
	if err != nil {
		t.Fatalf("read postgres migrations dir: %v", err)
	}
	want := 0
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			want++
		}
	}

	var applied int
	if err := conn.QueryRow(
		"SELECT COUNT(DISTINCT version_id) FROM goose_db_version WHERE version_id > ?", 0,
	).Scan(&applied); err != nil {
		t.Fatalf("count applied migrations: %v", err)
	}
	if applied != want {
		t.Errorf("got %d applied migrations, want %d", applied, want)
	}
}

func TestPostgresCategoriesAreSeeded(t *testing.T) {
	conn := openPostgresTest(t)

	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM categories").Scan(&count); err != nil {
		t.Fatalf("count categories: %v", err)
	}
	if count != 29 {
		t.Errorf("got %d seeded categories, want 29", count)
	}

	var kind string
	err := conn.QueryRow(
		"SELECT kind FROM categories WHERE id = ?", "533d9187-99b6-542b-a2f3-6eb9cbb299ce",
	).Scan(&kind)
	if err != nil {
		t.Fatalf("query transfer category: %v", err)
	}
	if kind != "transfer" {
		t.Errorf("got kind %q, want transfer", kind)
	}
}

func TestPostgresForeignKeysAreEnforced(t *testing.T) {
	conn := openPostgresTest(t)

	_, err := conn.Exec(
		`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		 VALUES ('eb7ecba5-0dfb-5855-a8dd-cbc0eaeb8559', '95d9ca5b-29ef-515d-81b5-2926677b7f46', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:01Z')`,
	)
	if err == nil {
		t.Fatal("expected foreign key violation inserting sync_runs with missing source_id")
	}
}

func TestPostgresOnlyOneActiveSyncRunPerSource(t *testing.T) {
	conn := openPostgresTest(t)

	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'pluggy', 'item-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('eb7ecba5-0dfb-5855-a8dd-cbc0eaeb8559', '48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'in_progress', '2026-01-01T00:00:00Z', NULL)`)

	_, err := conn.Exec(`INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('e4073650-2349-54e0-aae1-7f77de2e95bc', '48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'in_progress', '2026-01-01T00:00:01Z', NULL)`)
	if err == nil {
		t.Fatal("expected unique violation for a second in_progress sync run on the same source (uq_sync_runs_active_source)")
	}
}

func TestPostgresAppendOnlyTablesRejectUpdateAndDelete(t *testing.T) {
	conn := openPostgresTest(t)

	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'pluggy', 'item-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('eb7ecba5-0dfb-5855-a8dd-cbc0eaeb8559', '48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:01Z')`)
	mustExec(t, conn, `INSERT INTO raw_imports (
		id, sync_run_id, source_id, scope, page_sequence, request_attempt,
		request_method, request_path, http_status, response_headers, payload,
		payload_sha256, received_at
	) VALUES (
		'2f1c6e3b-1201-59c3-803c-4b4975e218ce', 'eb7ecba5-0dfb-5855-a8dd-cbc0eaeb8559', '48f6bbd2-ab66-504e-9c84-1d192fc6514d', 'item', 1, 1, 'GET', '/items/1', 200, '{}', ?,
		'deadbeef', '2026-01-01T00:00:00Z'
	)`, []byte{0})

	if _, err := conn.Exec("UPDATE raw_imports SET http_status = 500 WHERE id = '2f1c6e3b-1201-59c3-803c-4b4975e218ce'"); err == nil {
		t.Error("expected UPDATE on raw_imports to be rejected")
	}
	if _, err := conn.Exec("DELETE FROM raw_imports WHERE id = '2f1c6e3b-1201-59c3-803c-4b4975e218ce'"); err == nil {
		t.Error("expected DELETE on raw_imports to be rejected")
	}
}

// TestPostgresOnConflictReturning exercises the ON CONFLICT (...) DO NOTHING
// ... RETURNING pattern syncsvc uses (e.g. internal/syncsvc/service.go) —
// this is the shape most sensitive to the qmark-to-$N rewrite in
// internal/db/postgres.go, since it mixes a RETURNING clause with a
// conflict target.
func TestPostgresOnConflictReturning(t *testing.T) {
	conn := openPostgresTest(t)

	insert := `INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, external_item_id) DO NOTHING
		RETURNING id`

	var id string
	if err := conn.QueryRow(insert, "48f6bbd2-ab66-504e-9c84-1d192fc6514d", "pluggy", "item-1", "Nubank", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z").Scan(&id); err != nil {
		t.Fatalf("first insert should return the new id: %v", err)
	}
	if id != "48f6bbd2-ab66-504e-9c84-1d192fc6514d" {
		t.Errorf("got id %q, want s1", id)
	}

	err := conn.QueryRow(insert, "f64786c2-b4c3-5ac6-b14a-d3f58117c09d", "pluggy", "item-1", "Nubank (duplicate)", "2026-01-01T00:00:00Z", "2026-01-01T00:00:00Z").Scan(&id)
	if err != sql.ErrNoRows {
		t.Errorf("conflicting insert should return no rows, got err=%v", err)
	}
}

func TestPostgresPartialAndExpressionIndexesExist(t *testing.T) {
	conn := openPostgresTest(t)

	indexes := []string{"uq_sync_runs_active_source", "uq_raw_imports_identity"}
	for _, idx := range indexes {
		var name string
		err := conn.QueryRow(
			"SELECT indexname FROM pg_indexes WHERE schemaname = 'public' AND indexname = ?", idx,
		).Scan(&name)
		if err != nil {
			t.Errorf("index %s missing: %v", idx, err)
		}
	}
}

func TestPostgresPoolHonorsEnv(t *testing.T) {
	t.Setenv("JULIUS_DB_MAX_OPEN_CONNS", "3")
	conn := openPostgresTest(t)
	if got := conn.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
}

func TestPostgresPoolRejectsInvalidEnv(t *testing.T) {
	dsn := os.Getenv("JULIUS_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("JULIUS_TEST_POSTGRES_DSN not set; skipping Postgres integration test")
	}
	t.Setenv("JULIUS_DB_CONN_MAX_LIFETIME", "forever")
	conn, err := db.Open(dsn)
	if err == nil {
		conn.Close()
		t.Fatal("Open succeeded with an invalid JULIUS_DB_CONN_MAX_LIFETIME")
	}
	if !strings.Contains(err.Error(), "JULIUS_DB_CONN_MAX_LIFETIME") {
		t.Fatalf("error %q does not name the variable", err)
	}
}
