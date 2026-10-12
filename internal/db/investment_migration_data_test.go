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

// The investment management migration is 00037 on SQLite and 00036 on
// Postgres: the Postgres baseline collapses SQLite's early table-rebuild
// migrations, so its numbering stays one behind (see migration_down_test.go).
const (
	investmentMigrationSQLite   int64 = 37
	investmentMigrationPostgres int64 = 36
)

// seedProviderInvestments fills the pre-migration schema with two connections
// — one labelled, one falling back to its external item id — where only the
// first holds provider investments, plus an investment movement and a bank
// account. Exercising a connection *without* holdings is the point: the
// backfill groups what exists today and must not invent a custody account for
// a connection that has none.
func seedProviderInvestments(t *testing.T, conn *sql.DB, payload any) {
	t.Helper()
	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, display_name, label, created_at, updated_at)
		VALUES ('src-corretora', 'pluggy', 'item-corretora', 'Corretora', 'Minha corretora', '2026-01-01T00:00:00Z', '2026-01-02T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('src-banco', 'pluggy', 'item-banco', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('run1', 'src-corretora', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES ('ri1', 'run1', 'src-corretora', 'investments', 1, 1, 'GET', '/x', 200, '{}', ?, 'sha', '2026-01-01T00:00:00Z')`, payload)
	mustExec(t, conn, `INSERT INTO financial_accounts (
			id, source_id, external_id, currency_code, current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES ('acc-corretora', 'src-corretora', 'acc-corretora', 'BRL', 'ri1', 'hash', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_investments (
			id, source_id, external_id, name, balance, currency_code, quantity, amount,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES ('inv-cdb', 'src-corretora', 'ext-cdb', 'CDB Liquidez', '1500.00', 'BRL', '1', '1400.00',
			'ri1', 'hash', '2026-01-03T00:00:00Z', '2026-01-09T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_investment_transactions (
			id, source_id, investment_id, external_id, movement_type, amount,
			current_raw_import_id, normalized_hash, created_at, updated_at
		) VALUES ('mov-cdb', 'src-corretora', 'inv-cdb', 'ext-mov-cdb', 'BUY', '1400.00',
			'ri1', 'hash', '2026-01-03T00:00:00Z', '2026-01-03T00:00:00Z')`)
}

// assertInvestmentBackfill checks what the migration promises an existing
// install: one local custody grouping per connection that actually holds
// investments, named after that connection, with the provider's own rows
// untouched and no goal invented for them.
func assertInvestmentBackfill(t *testing.T, conn *sql.DB) {
	t.Helper()

	var accounts int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM investment_accounts`).Scan(&accounts); err != nil {
		t.Fatalf("count investment_accounts: %v", err)
	}
	if accounts != 1 {
		t.Fatalf("investment_accounts = %d, want 1 (only the connection holding investments)", accounts)
	}

	const groupingID = "integrated:src-corretora"
	assertRow(t, conn, "investment_accounts", groupingID, "kind", "integrated")
	assertRow(t, conn, "investment_accounts", groupingID, "source_id", "src-corretora")
	// The label wins over display_name, and no account number is invented.
	assertRow(t, conn, "investment_accounts", groupingID, "name", "Minha corretora")

	var currency, financialAccount sql.NullString
	if err := conn.QueryRow(`SELECT currency_code, financial_account_id FROM investment_accounts WHERE id = ?`, groupingID).
		Scan(&currency, &financialAccount); err != nil {
		t.Fatalf("query grouping nullable columns: %v", err)
	}
	if currency.Valid || financialAccount.Valid {
		t.Errorf("grouping currency=%v financial_account=%v, want both unset until the user links them", currency, financialAccount)
	}

	// Imported identifiers and balances survive untouched.
	assertRow(t, conn, "financial_investments", "inv-cdb", "external_id", "ext-cdb")
	assertRow(t, conn, "financial_investments", "inv-cdb", "balance", "1500.00")
	assertRow(t, conn, "financial_investment_transactions", "mov-cdb", "amount", "1400.00")

	for _, table := range []string{"investment_portfolios", "investment_position_portfolios", "investment_positions", "investment_operations", "investment_reconciliations"} {
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s = %d rows, want 0: migrated holdings start with no goal and no local ledger", table, count)
		}
	}
}

// assertInvestmentSchemaGuards exercises the constraints the ledger relies on
// instead of re-checking them in Go: a manual account cannot claim a provider
// connection, and one parcel of a bank line cannot be linked twice to the same
// operation while a deliberate split across operations stays allowed.
func assertInvestmentSchemaGuards(t *testing.T, conn *sql.DB) {
	t.Helper()

	if _, err := conn.Exec(`INSERT INTO investment_accounts (id, name, kind, currency_code, source_id, created_at, updated_at)
		VALUES ('bad', 'Manual com conexão', 'manual', 'BRL', 'src-corretora', '2026-02-01T00:00:00Z', '2026-02-01T00:00:00Z')`); err == nil {
		t.Error("manual account with a source_id was accepted; the kind/source CHECK is not protecting the grouping")
	}

	mustExec(t, conn, `INSERT INTO investment_accounts (id, name, kind, currency_code, created_at, updated_at)
		VALUES ('manual', 'Carteira manual', 'manual', 'BRL', '2026-02-01T00:00:00Z', '2026-02-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount, amount_in_account_currency,
			currency_code, occurred_at, provider_status, movement_type, current_raw_import_id,
			normalized_hash, created_at, updated_at
		) VALUES ('tx-aporte', 'src-corretora', 'acc-corretora', 'tx-aporte', 'Aporte', '-1000.00', '-1000.00', 'BRL',
			'2026-02-02T00:00:00Z', 'POSTED', 'DEBIT', 'ri1', 'hash', '2026-02-02T00:00:00Z', '2026-02-02T00:00:00Z')`)
	for _, id := range []string{"op-a", "op-b"} {
		mustExec(t, conn, `INSERT INTO investment_operations (id, account_id, kind, occurred_on, amount, created_at, updated_at)
			VALUES (?, 'manual', 'deposit', '2026-02-02', '500', '2026-02-02T00:00:00Z', '2026-02-02T00:00:00Z')`, id)
	}
	mustExec(t, conn, `INSERT INTO investment_reconciliations (id, operation_id, financial_transaction_id, amount, created_at)
		VALUES ('link-a', 'op-a', 'tx-aporte', '500', '2026-02-02T00:00:00Z')`)
	// The other half of the same bank line, on a different operation.
	mustExec(t, conn, `INSERT INTO investment_reconciliations (id, operation_id, financial_transaction_id, amount, created_at)
		VALUES ('link-b', 'op-b', 'tx-aporte', '500', '2026-02-02T00:00:00Z')`)
	if _, err := conn.Exec(`INSERT INTO investment_reconciliations (id, operation_id, financial_transaction_id, amount, created_at)
		VALUES ('link-dup', 'op-a', 'tx-aporte', '500', '2026-02-02T00:00:00Z')`); err == nil {
		t.Error("the same operation/transaction pair was linked twice; the partial unique index is missing")
	}
}

// TestInvestmentManagementMigrationGroupsProviderHoldings covers 00037's
// backfill on SQLite. An install that already syncs investments has to come
// out of the upgrade with its holdings grouped and nothing about them
// rewritten.
func TestInvestmentManagementMigrationGroupsProviderHoldings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	// See TestPayablesMigrationPreservesData: PRAGMA foreign_keys is
	// per-connection, so the pool has to stay at one physical connection.
	conn.SetMaxOpenConns(1)
	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("enable foreign keys: %v", err)
	}

	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/sqlite")
	if err != nil {
		t.Fatalf("root migrations fs: %v", err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	ctx := context.Background()
	if _, err := provider.UpTo(ctx, investmentMigrationSQLite-1); err != nil {
		t.Fatalf("migrate up to %d: %v", investmentMigrationSQLite-1, err)
	}
	seedProviderInvestments(t, conn, []byte{0})
	if _, err := provider.UpTo(ctx, investmentMigrationSQLite); err != nil {
		t.Fatalf("migrate up to %d: %v", investmentMigrationSQLite, err)
	}

	assertInvestmentBackfill(t, conn)
	assertInvestmentSchemaGuards(t, conn)
}

// TestPostgresInvestmentManagementMigrationGroupsProviderHoldings is the same
// assertion on the other dialect, where the equivalent migration is 00036 and
// is_active is a BOOLEAN rather than an INTEGER CHECK.
func TestPostgresInvestmentManagementMigrationGroupsProviderHoldings(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, investmentMigrationPostgres-1); err != nil {
		t.Fatalf("migrate up to %d: %v", investmentMigrationPostgres-1, err)
	}
	seedProviderInvestments(t, conn, []byte{0})
	if _, err := provider.UpTo(ctx, investmentMigrationPostgres); err != nil {
		t.Fatalf("migrate up to %d: %v", investmentMigrationPostgres, err)
	}

	assertInvestmentBackfill(t, conn)
	assertInvestmentSchemaGuards(t, conn)
}
