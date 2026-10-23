package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
	dbtypes "github.com/greg0x46/julius/internal/db/types"
	"github.com/pressly/goose/v3"
)

func TestSnapshotCurrencyMigrationPreservesHistory(t *testing.T) {
	for _, backend := range dbtest.Backends() {
		t.Run(backend.Name, func(t *testing.T) {
			conn := backend.Open(t)
			dialect := goose.DialectSQLite3
			if backend.Name == "postgres" {
				dialect = goose.DialectPostgres
			}
			provider, err := goose.NewProvider(dialect, conn, os.DirFS("migrations/"+backend.Name))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := provider.DownTo(ctx, 1); err != nil {
				t.Fatal(err)
			}
			seedOldSnapshot(t, conn)
			if _, err := provider.UpTo(ctx, 4); err != nil {
				t.Fatal(err)
			}
			var amount, code string
			if err := conn.QueryRow(`SELECT net_worth,valuation_currency FROM net_worth_snapshots`).Scan(dbtypes.Decimal(&amount), &code); err != nil {
				t.Fatal(err)
			}
			if amount != "123.456789012345" || code != "BRL" {
				t.Fatalf("migration changed valuation: %s %s", amount, code)
			}
		})
	}
}

func seedOldSnapshot(t *testing.T, conn *sql.DB) {
	t.Helper()
	_, err := conn.Exec(`INSERT INTO net_worth_snapshots(id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at)
 VALUES ('00000000-0000-0000-0000-000000000066','2026-10-12','123.456789012345','0','123.456789012345','123.456789012345','0','0','0',FALSE,'2026-10-12T00:00:00Z','2026-10-12T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotValuationMigrationMarksExistingRowsLegacy(t *testing.T) {
	for _, backend := range dbtest.Backends() {
		t.Run(backend.Name, func(t *testing.T) {
			conn := backend.Open(t)
			dialect := goose.DialectSQLite3
			if backend.Name == "postgres" {
				dialect = goose.DialectPostgres
			}
			provider, err := goose.NewProvider(dialect, conn, os.DirFS("migrations/"+backend.Name))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := provider.DownTo(ctx, 3); err != nil {
				t.Fatal(err)
			}
			seedOldSnapshot(t, conn)
			if _, err := provider.UpTo(ctx, 4); err != nil {
				t.Fatal(err)
			}
			var amount, status, rates string
			var version int
			var valuedAt, policy sql.NullString
			if err := conn.QueryRow(`SELECT net_worth,valuation_version,valuation_status,valued_at,rate_policy,valuation_rates FROM net_worth_snapshots`).
				Scan(dbtypes.Decimal(&amount), &version, &status, &valuedAt, &policy, &rates); err != nil {
				t.Fatal(err)
			}
			if amount != "123.456789012345" || version != 0 || status != "legacy" || valuedAt.Valid || policy.Valid || rates != "{}" {
				t.Fatalf("legacy row changed: %s v%d %s %v %v %s", amount, version, status, valuedAt, policy, rates)
			}
			if _, err := conn.Exec(`UPDATE net_worth_snapshots SET valuation_status = 'guessed'`); err == nil {
				t.Fatal("an unknown valuation status was accepted")
			}
		})
	}
}

func TestDropNetWorthSnapshotsMigration(t *testing.T) {
	for _, backend := range dbtest.Backends() {
		t.Run(backend.Name, func(t *testing.T) {
			conn := backend.Open(t)
			dialect := goose.DialectSQLite3
			if backend.Name == "postgres" {
				dialect = goose.DialectPostgres
			}
			provider, err := goose.NewProvider(dialect, conn, os.DirFS("migrations/"+backend.Name))
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			if _, err := provider.DownTo(ctx, 5); err != nil {
				t.Fatal(err)
			}
			seedOldSnapshot(t, conn)
			if _, err := provider.Up(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(`SELECT 1 FROM net_worth_snapshots`); err == nil {
				t.Fatal("net_worth_snapshots survived the migration")
			}
			if _, err := provider.DownTo(ctx, 5); err != nil {
				t.Fatal(err)
			}
			if _, err := conn.Exec(`SELECT 1 FROM net_worth_snapshots`); err != nil {
				t.Fatalf("rolling back must restore the table: %v", err)
			}
		})
	}
}
