package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/pressly/goose/v3"
)

// TestPayablesMigrationPreservesData seeds rows into the pre-00013 (SQLite)
// debts/receivables/scenarios schema, applies migration 00013_payables, and
// asserts every row survived into payables/payable_transaction_links with
// the same ids, amounts, and FKs repointed correctly.
func TestPayablesMigrationPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	// Mirrors openSQLite's single-connection pinning in db.go: PRAGMA
	// foreign_keys is per-connection, so a pool bigger than one risks the
	// migration's OFF/ON toggle landing on a different physical connection
	// than the statements it's meant to guard.
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
	if _, err := provider.UpTo(ctx, 12); err != nil {
		t.Fatalf("migrate up to 12: %v", err)
	}

	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('src1', 'pluggy', 'item-1', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('run1', 'src1', 'completed', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	mustExec(t, conn, `INSERT INTO raw_imports (
			id, sync_run_id, source_id, scope, page_sequence, request_attempt,
			request_method, request_path, http_status, response_headers, payload,
			payload_sha256, received_at
		) VALUES ('ri1', 'run1', 'src1', 'transactions', 1, 1, 'GET', '/x', 200, '{}', x'00', 'sha', '2026-01-01T00:00:00Z')`)
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

	if _, err := provider.UpTo(ctx, 13); err != nil {
		t.Fatalf("migrate up to 13: %v", err)
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
		err := conn.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != sql.ErrNoRows {
			t.Errorf("expected table %s to be dropped, got err=%v", table, err)
		}
	}
}

// legacyFixture is the pre-unification database every migration test
// starts from: one payable with a plan, an allocation, a recurring
// commitment with a manual reconciliation, and an automation rule
// targeting that commitment. It is seeded at version 26 and migrated to
// 27, so callers see exactly what the unification migration produced.
type legacyFixture struct {
	recurringScenarioID   string
	scenarioTransactionID string
	reconciliationID      string
}

func seedLegacyFixture(t *testing.T) (*sql.DB, *goose.Provider, legacyFixture) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
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
	if _, err := provider.UpTo(ctx, 26); err != nil {
		t.Fatalf("migrate up to 26: %v", err)
	}

	const (
		now       = "2026-08-01T00:00:00.000000000Z"
		category  = "000433b6-3094-5a9c-87df-465b70574a4b"
		payableID = "payable-legacy"
		planID    = "plan-legacy"
		stID      = "scenario-transaction-legacy"
		recID     = "recurring-legacy"
		ruleID    = "rule-legacy"
		linkID    = "payable-link-legacy"
		allocID   = "allocation-legacy"
		reconID   = "reconciliation-legacy"
		txID      = "transaction-settlement"
		reconTxID = "transaction-reconciliation"
	)
	mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, created_at, updated_at)
		VALUES ('source-legacy', 'pluggy', 'item-legacy', ?, ?)`, now, now)
	mustExec(t, conn, `INSERT INTO sync_runs (id, source_id, status, started_at, finished_at)
		VALUES ('run-legacy', 'source-legacy', 'completed', ?, ?)`, now, now)
	mustExec(t, conn, `INSERT INTO raw_imports (
		id, sync_run_id, source_id, scope, page_sequence, request_attempt,
		request_method, request_path, http_status, response_headers, payload,
		payload_sha256, received_at
		) VALUES ('import-legacy', 'run-legacy', 'source-legacy', 'transactions', 1, 1,
		'GET', '/transactions', 200, '{}', x'00', 'legacy-hash', ?)`, now)
	mustExec(t, conn, `INSERT INTO financial_accounts (
		id, source_id, external_id, currency_code, current_raw_import_id,
		normalized_hash, created_at, updated_at
		) VALUES ('account-legacy', 'source-legacy', 'account-legacy', 'BRL',
		'import-legacy', 'account-hash', ?, ?)`, now, now)
	for _, tx := range []struct{ id, description, amount, movement string }{
		{txID, "Pagamento", "-100.00", "DEBIT"},
		{reconTxID, "Salário", "200.00", "CREDIT"},
	} {
		mustExec(t, conn, `INSERT INTO financial_transactions (
			id, source_id, account_id, external_id, description, amount,
			amount_in_account_currency, currency_code, occurred_at, provider_status,
			movement_type, current_raw_import_id, normalized_hash, created_at, updated_at
			) VALUES (?, 'source-legacy', 'account-legacy', ?, ?, ?, ?, 'BRL', ?, 'POSTED', ?,
			'import-legacy', 'transaction-hash-' || ?, ?, ?)`,
			tx.id, tx.id, tx.description, tx.amount, tx.amount, now, tx.movement, tx.id, now, now)
	}
	mustExec(t, conn, `INSERT INTO payables (
		id, kind, name, total_amount, starting_settled_amount, created_at, updated_at
		) VALUES (?, 'debt', 'Dívida legada', '1000.00', '0.00', ?, ?)`, payableID, now, now)
	mustExec(t, conn, `INSERT INTO payable_transaction_links (
		id, payable_id, transaction_id, linked_amount, linked_at
		) VALUES (?, ?, ?, '100.00', ?)`, linkID, payableID, txID, now)
	mustExec(t, conn, `INSERT INTO scenarios (
		id, kind, name, payable_id, created_at, updated_at
		) VALUES (?, 'debt_plan', 'Plano legado', ?, ?, ?)`, planID, payableID, now, now)
	mustExec(t, conn, `INSERT INTO scenario_transactions (
		id, scenario_id, description, amount, projected_at, created_at, updated_at
		) VALUES (?, ?, 'Parcela legada', '100.00', '2026-08-15', ?, ?)`, stID, planID, now, now)
	mustExec(t, conn, `INSERT INTO scenario_transaction_realizations (
		id, scenario_transaction_id, payable_link_id, allocated_amount, created_at
		) VALUES (?, ?, ?, '100.00', ?)`, allocID, stID, linkID, now)
	mustExec(t, conn, `INSERT INTO recurring_commitments (
		id, name, kind, amount, category_id, account_id, cadence, day_of_month,
		month_of_year, start_date, end_date, is_active, created_at, updated_at
		) VALUES (?, 'Recorrência legada', 'income', '200.00', ?, NULL, 'monthly', 15,
		NULL, '2026-01-01', NULL, 0, ?, ?)`, recID, category, now, now)
	mustExec(t, conn, `INSERT INTO recurrence_reconciliations (
		id, recurring_commitment_id, occurrence_date, state, transaction_id, created_at
		) VALUES (?, ?, '2026-08-15', 'linked', ?, ?)`, reconID, recID, reconTxID, now)
	mustExec(t, conn, `INSERT INTO automation_rules (
		id, name, is_active, logic_operator, created_at, updated_at
		) VALUES (?, 'Regra legada', 1, 'and', ?, ?)`, ruleID, now, now)
	mustExec(t, conn, `INSERT INTO automation_rule_conditions (
		id, rule_id, field, operator, value, position
		) VALUES ('condition-legacy', ?, 'description', 'contains', 'salário', 0)`, ruleID)
	mustExec(t, conn, `INSERT INTO automation_rule_actions (
		id, rule_id, action_type, recurring_commitment_id, category_id, position
		) VALUES ('action-legacy', ?, 'reconcile', ?, NULL, 0)`, ruleID, recID)

	if _, err := provider.UpTo(ctx, 27); err != nil {
		t.Fatalf("migrate up to 27: %v", err)
	}

	var scenarioID string
	if err := conn.QueryRow(`SELECT scenario_id FROM recurring_commitment_scenario_map WHERE recurring_commitment_id = ?`, recID).Scan(&scenarioID); err != nil {
		t.Fatal(err)
	}
	return conn, provider, legacyFixture{
		recurringScenarioID: scenarioID, scenarioTransactionID: stID, reconciliationID: reconID,
	}
}

func TestScenarioProjectionMigrationPreservesAndMapsLegacyData(t *testing.T) {
	conn, _, seeded := seedLegacyFixture(t)
	scenarioID := seeded.recurringScenarioID
	const (
		recID   = "recurring-legacy"
		reconID = "reconciliation-legacy"
	)

	var recurringCount, scheduleCount, settlementCount, allocationCount, reconciliationCount int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenarios WHERE kind = 'recurring'`).Scan(&recurringCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenario_recurring_schedules`).Scan(&scheduleCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'settlement'`).Scan(&settlementCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'allocation'`).Scan(&allocationCount); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'reconciliation'`).Scan(&reconciliationCount); err != nil {
		t.Fatal(err)
	}
	if recurringCount != 1 || scheduleCount != 1 || settlementCount != 1 || allocationCount != 1 || reconciliationCount != 1 {
		t.Fatalf("migrated counts = recurring %d, schedules %d, settlements %d, allocations %d, reconciliations %d; want one of each", recurringCount, scheduleCount, settlementCount, allocationCount, reconciliationCount)
	}

	var kind string
	if scenarioID == recID {
		t.Fatal("migration reused recurring commitment id as scenario id")
	}
	if err := conn.QueryRow(`SELECT kind FROM scenarios WHERE id = ?`, scenarioID).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "recurring" {
		t.Fatalf("mapped scenario kind = %q, want recurring", kind)
	}
	var mappedScenarioID, actionScenarioID string
	if err := conn.QueryRow(`SELECT scenario_id FROM recurrence_reconciliations WHERE id = ?`, reconID).Scan(&mappedScenarioID); err != nil {
		t.Fatal(err)
	}
	if mappedScenarioID != scenarioID {
		t.Fatalf("reconciliation scenario_id = %q, want %q", mappedScenarioID, scenarioID)
	}
	if err := conn.QueryRow(`SELECT scenario_id FROM automation_rule_actions WHERE id = 'action-legacy'`).Scan(&actionScenarioID); err != nil {
		t.Fatal(err)
	}
	if actionScenarioID != scenarioID {
		t.Fatalf("automation scenario_id = %q, want %q", actionScenarioID, scenarioID)
	}
	var active int
	if err := conn.QueryRow(`SELECT is_active FROM scenarios WHERE id = ?`, scenarioID).Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("migrated inactive recurring scenario is_active = %d, want 0", active)
	}
	var orphaned int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM scenario_realizations sr WHERE NOT EXISTS (SELECT 1 FROM scenarios s WHERE s.id = sr.scenario_id)`).Scan(&orphaned); err != nil {
		t.Fatal(err)
	}
	if orphaned != 0 {
		t.Fatalf("migrated generic realizations with missing scenarios = %d", orphaned)
	}
}

// The three drop migrations retire the compatibility tables the unification
// left behind. Each one has to carry forward whatever only ever reached the
// legacy table, so this continues the fixture above from version 27 to head
// and checks the same legacy rows are still readable through the canonical
// tables afterwards.
func TestDropMigrationsCarryLegacyRowsForward(t *testing.T) {
	conn, provider, seeded := seedLegacyFixture(t)
	ctx := context.Background()

	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("migrate to head: %v", err)
	}

	for _, table := range []string{
		"scenario_transaction_realizations", "recurrence_reconciliations",
		"recurring_commitments", "recurring_commitment_scenario_map",
	} {
		var name string
		err := conn.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("table %s still exists after the drop migrations (err=%v)", table, err)
		}
	}

	var allocations, reconciliations int
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'allocation' AND scenario_transaction_id = ?`,
		seeded.scenarioTransactionID,
	).Scan(&allocations); err != nil {
		t.Fatal(err)
	}
	if allocations != 1 {
		t.Errorf("allocations after the drop = %d, want 1", allocations)
	}
	if err := conn.QueryRow(
		`SELECT COUNT(*) FROM scenario_realizations WHERE relation_type = 'reconciliation' AND scenario_id = ?`,
		seeded.recurringScenarioID,
	).Scan(&reconciliations); err != nil {
		t.Fatal(err)
	}
	if reconciliations != 1 {
		t.Errorf("reconciliations after the drop = %d, want 1", reconciliations)
	}

	// The recurring commitment survives as its Scenario, schedule included,
	// and the automation rule still targets it by scenario id.
	var name, cashflowKind, actionScenarioID string
	if err := conn.QueryRow(`
		SELECT s.name, r.cashflow_kind
		FROM scenarios s JOIN scenario_recurring_schedules r ON r.scenario_id = s.id
		WHERE s.id = ?`, seeded.recurringScenarioID).Scan(&name, &cashflowKind); err != nil {
		t.Fatal(err)
	}
	if name != "Recorrência legada" || cashflowKind != "income" {
		t.Errorf("recurring scenario = %q/%q, want \"Recorrência legada\"/income", name, cashflowKind)
	}
	if err := conn.QueryRow(
		`SELECT scenario_id FROM automation_rule_actions WHERE id = 'action-legacy'`,
	).Scan(&actionScenarioID); err != nil {
		t.Fatal(err)
	}
	if actionScenarioID != seeded.recurringScenarioID {
		t.Errorf("reconcile target = %q, want %q", actionScenarioID, seeded.recurringScenarioID)
	}
}

func assertRow(t *testing.T, conn *sql.DB, table, id, column, want string) {
	t.Helper()
	var got string
	query := "SELECT " + column + " FROM " + table + " WHERE id = ?"
	if err := conn.QueryRow(query, id).Scan(&got); err != nil {
		t.Fatalf("query %s.%s for id=%s: %v", table, column, id, err)
	}
	if got != want {
		t.Errorf("%s.%s for id=%s = %q, want %q", table, column, id, got, want)
	}
}

func mustExec(t *testing.T, conn *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := conn.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

// TestConnectionsMigrationCarriesTheConfiguredItem covers 00032's backfill:
// the single item id the setup screen used to store in `settings` has to
// become the first data_sources row, or an existing install would come out of
// the upgrade with no connection at all and nothing left to sync.
func TestConnectionsMigrationCarriesTheConfiguredItem(t *testing.T) {
	for _, tc := range []struct {
		name           string
		seedDataSource bool
	}{
		// Set up but never synced: nothing created the row lazily, so the
		// migration is the only thing that can.
		{name: "never synced", seedDataSource: false},
		// Already synced at least once: handleCreateSyncRun made the row, and
		// the backfill must not add a second one for the same item.
		{name: "already synced", seedDataSource: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			conn, provider := migrationProviderAt(t, 31)
			ctx := context.Background()

			mustExec(t, conn, `INSERT INTO settings (key, value, is_encrypted, updated_at)
				VALUES ('pluggy.item_id', 'item-1', 0, '2026-01-01T00:00:00.000000000Z')`)
			if tc.seedDataSource {
				mustExec(t, conn, `INSERT INTO data_sources (id, provider, external_item_id, display_name, created_at, updated_at)
					VALUES ('src1', 'pluggy', 'item-1', 'Banco de Exemplo', '2026-01-01T00:00:00.000000000Z', '2026-01-01T00:00:00.000000000Z')`)
			}

			if _, err := provider.UpTo(ctx, 32); err != nil {
				t.Fatalf("migrate up to 32: %v", err)
			}

			var count int
			var id string
			var isActive int
			if err := conn.QueryRow(
				`SELECT COUNT(*) FROM data_sources WHERE provider = 'pluggy' AND external_item_id = 'item-1'`,
			).Scan(&count); err != nil {
				t.Fatalf("count connections: %v", err)
			}
			if count != 1 {
				t.Fatalf("connections for the configured item = %d, want exactly 1", count)
			}
			if err := conn.QueryRow(
				`SELECT id, is_active FROM data_sources WHERE external_item_id = 'item-1'`,
			).Scan(&id, &isActive); err != nil {
				t.Fatalf("read connection: %v", err)
			}
			if isActive != 1 {
				t.Errorf("is_active = %d, want the carried-over connection to keep syncing", isActive)
			}
			if tc.seedDataSource {
				if id != "src1" {
					t.Errorf("connection id = %q, want the existing row to be reused", id)
				}
			} else if _, err := uuid.Parse(id); err != nil {
				// The frontend rejects a non-uuid id, so a generated one has
				// to look like the ones the app itself writes.
				t.Errorf("generated connection id %q is not a uuid: %v", id, err)
			}

			var leftover int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'pluggy.item_id'`).Scan(&leftover); err != nil {
				t.Fatalf("count settings: %v", err)
			}
			if leftover != 0 {
				t.Errorf("pluggy.item_id survived the migration; data_sources is the single source of truth now")
			}
		})
	}
}

// migrationProviderAt opens a fresh SQLite database migrated up to version,
// mirroring openSQLite's single-connection pinning.
func migrationProviderAt(t *testing.T, version int64) (*sql.DB, *goose.Provider) {
	t.Helper()
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
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
	if _, err := provider.UpTo(context.Background(), version); err != nil {
		t.Fatalf("migrate up to %d: %v", version, err)
	}
	return conn, provider
}
