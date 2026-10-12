package db

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"testing"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// TestPostgresPayablesMigrationPreservesData mirrors
// TestPayablesMigrationPreservesData for the Postgres dialect: seeds rows
// into the pre-00011 debts/receivables/scenarios schema, applies migration
// 00011_payables, and asserts every row survived into
// payables/payable_transaction_links with ids, amounts, and FKs intact.
func TestPostgresPayablesMigrationPreservesData(t *testing.T) {
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

	connector, err := newPGConnector(dsn)
	if err != nil {
		t.Fatalf("new pg connector: %v", err)
	}
	conn := sql.OpenDB(connector)
	t.Cleanup(func() { conn.Close() })

	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/postgres")
	if err != nil {
		t.Fatalf("root migrations fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, migrations)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 10); err != nil {
		t.Fatalf("migrate up to 10: %v", err)
	}

	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('src1', 'pluggy', 'item-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('run1', 'src1', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES ('ri1', 'run1', 'src1', 'transactions', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', '2026-01-01T00:00:00Z')`, []byte{0})
	mustExec(t, conn, `INSERT INTO financial_accounts (
			id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES ('acc1', 'src1', 'acc1', 'BRL', 'ri1', 'hash', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES ('tx1', 'src1', 'acc1', 'tx1', 'Pagamento dívida', '-100.00', '-100.00', 'BRL', '2026-01-05T00:00:00Z', 'POSTED', 'DEBIT', 'ri1', 'hash', '2026-01-05T00:00:00Z', '2026-01-05T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES ('tx2', 'src1', 'acc1', 'tx2', 'Recebimento', '200.00', '200.00', 'BRL', '2026-01-06T00:00:00Z', 'POSTED', 'CREDIT', 'ri1', 'hash', '2026-01-06T00:00:00Z', '2026-01-06T00:00:00Z')`)

	mustExec(t, conn, `INSERT INTO debts (id, name, total_amount, starting_paid_amount, created_at, updated_at)
		VALUES ('debt1', 'Empréstimo carro', '1000.00', '50.00', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO debt_transaction_links (id, debt_id, transaction_id, linked_amount, linked_at)
		VALUES ('dlink1', 'debt1', 'tx1', '100.00', '2026-01-05T00:00:00Z')`)

	mustExec(t, conn, `INSERT INTO receivables (id, name, total_amount, starting_received_amount, created_at, updated_at)
		VALUES ('recv1', 'Empréstimo para amigo', '2000.00', '100.00', '2026-02-01T00:00:00Z', '2026-02-02T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO receivable_transaction_links (id, receivable_id, transaction_id, linked_amount, linked_at)
		VALUES ('rlink1', 'recv1', 'tx2', '200.00', '2026-01-06T00:00:00Z')`)

	mustExec(t, conn, `INSERT INTO scenarios (id, kind, name, debt_id, created_at, updated_at)
		VALUES ('scen1', 'debt_plan', 'Plano dívida', 'debt1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO scenarios (id, kind, name, receivable_id, created_at, updated_at)
		VALUES ('scen2', 'receivable_plan', 'Plano recebível', 'recv1', '2026-02-01T00:00:00Z', '2026-02-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO scenario_transactions (id, scenario_id, description, amount, projected_at, created_at, updated_at)
		VALUES ('stx1', 'scen1', 'Parcela 1', '100.00', '2026-01-05T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO scenario_transactions (id, scenario_id, description, amount, projected_at, created_at, updated_at)
		VALUES ('stx2', 'scen2', 'Recebimento 1', '200.00', '2026-01-06T00:00:00Z', '2026-02-01T00:00:00Z', '2026-02-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO scenario_transaction_realizations (id, scenario_transaction_id, debt_link_id, allocated_amount, created_at)
		VALUES ('real1', 'stx1', 'dlink1', '100.00', '2026-01-05T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO scenario_transaction_realizations (id, scenario_transaction_id, receivable_link_id, allocated_amount, created_at)
		VALUES ('real2', 'stx2', 'rlink1', '200.00', '2026-01-06T00:00:00Z')`)

	if _, err := provider.UpTo(ctx, 11); err != nil {
		t.Fatalf("migrate up to 11: %v", err)
	}

	assertRow(t, conn, "payables", "debt1", "kind", "debt")
	assertRow(t, conn, "payables", "debt1", "total_amount", "1000.00")
	assertRow(t, conn, "payables", "debt1", "starting_settled_amount", "50.00")
	assertRow(t, conn, "payables", "recv1", "kind", "receivable")
	assertRow(t, conn, "payables", "recv1", "starting_settled_amount", "100.00")

	assertRow(t, conn, "payable_transaction_links", "dlink1", "payable_id", "debt1")
	assertRow(t, conn, "payable_transaction_links", "dlink1", "linked_amount", "100.00")
	assertRow(t, conn, "payable_transaction_links", "rlink1", "payable_id", "recv1")

	assertRow(t, conn, "scenarios", "scen1", "payable_id", "debt1")
	assertRow(t, conn, "scenarios", "scen2", "payable_id", "recv1")

	assertRow(t, conn, "scenario_transaction_realizations", "real1", "payable_link_id", "dlink1")
	assertRow(t, conn, "scenario_transaction_realizations", "real2", "payable_link_id", "rlink1")

	for _, table := range []string{"debts", "receivables", "debt_transaction_links", "receivable_transaction_links"} {
		var name string
		err := conn.QueryRow(
			"SELECT tablename FROM pg_tables WHERE schemaname = 'public' AND tablename = ?", table,
		).Scan(&name)
		if err != sql.ErrNoRows {
			t.Errorf("expected table %s to be dropped, got err=%v", table, err)
		}
	}
}

// TestPostgresConnectionsMigrationCarriesTheConfiguredItem mirrors
// TestConnectionsMigrationCarriesTheConfiguredItem on the other dialect,
// where the id has to come from gen_random_uuid() rather than randomblob.
func TestPostgresConnectionsMigrationCarriesTheConfiguredItem(t *testing.T) {
	conn, provider := postgresMigrationProviderAt(t, 30)
	ctx := context.Background()

	mustExec(t, conn, `INSERT INTO settings (key, value, is_encrypted, updated_at)
		VALUES ('pluggy.item_id', 'item-1', 0, '2026-01-01T00:00:00.000000000Z')`)

	if _, err := provider.UpTo(ctx, 31); err != nil {
		t.Fatalf("migrate up to 31: %v", err)
	}

	var id string
	var isActive int
	if err := conn.QueryRow(
		`SELECT id, is_active FROM data_sources WHERE provider = 'pluggy' AND external_item_id = 'item-1'`,
	).Scan(&id, &isActive); err != nil {
		t.Fatalf("read carried-over connection: %v", err)
	}
	if isActive != 1 {
		t.Errorf("is_active = %d, want the carried-over connection to keep syncing", isActive)
	}
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("generated connection id %q is not a uuid: %v", id, err)
	}

	var leftover int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'pluggy.item_id'`).Scan(&leftover); err != nil {
		t.Fatalf("count settings: %v", err)
	}
	if leftover != 0 {
		t.Errorf("pluggy.item_id survived the migration; data_sources is the single source of truth now")
	}
}

// postgresMigrationProviderAt resets the public schema and migrates it up to
// version, skipping the test when no Postgres is configured.
func postgresMigrationProviderAt(t *testing.T, version int64) (*sql.DB, *goose.Provider) {
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

	connector, err := newPGConnector(dsn)
	if err != nil {
		t.Fatalf("new pg connector: %v", err)
	}
	conn := sql.OpenDB(connector)
	t.Cleanup(func() { conn.Close() })

	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/postgres")
	if err != nil {
		t.Fatalf("root migrations fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, conn, migrations)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	if _, err := provider.UpTo(context.Background(), version); err != nil {
		t.Fatalf("migrate up to %d: %v", version, err)
	}
	return conn, provider
}
