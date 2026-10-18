package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/pressly/goose/v3"

	"github.com/greg0x46/julius/internal/db/dbtest"
)

func tableExists(t *testing.T, conn *sql.DB, backend, table string) bool {
	t.Helper()
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = '` + table + `'`
	if backend == "postgres" {
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = '` + table + `'`
	}
	var n int
	if err := conn.QueryRow(query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// The goals migration replaces the investment grouping: it must run over a
// database that already holds groupings and links, and roll back to the old
// (empty) tables.
func TestGoalsMigrationReplacesTheInvestmentGrouping(t *testing.T) {
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
			if _, err := provider.DownTo(ctx, 4); err != nil {
				t.Fatal(err)
			}
			if tableExists(t, conn, backend.Name, "goals") || !tableExists(t, conn, backend.Name, "investment_portfolios") {
				t.Fatal("expected the pre-goals schema at version 4")
			}
			for _, query := range []string{
				`INSERT INTO investment_portfolios (id, name, target_amount, created_at, updated_at)
				 VALUES ('00000000-0000-0000-0000-0000000000a1', 'Reserva', '1000', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`,
				`INSERT INTO investment_accounts (id, name, kind, currency_code, created_at, updated_at)
				 VALUES ('00000000-0000-0000-0000-0000000000b1', 'Custódia', 'manual', 'BRL', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`,
				`INSERT INTO investment_assets (id, canonical_key, name, asset_type, created_at, updated_at)
				 VALUES ('00000000-0000-0000-0000-0000000000c1', 'name:cdb', 'CDB', 'Renda fixa', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`,
				`INSERT INTO investment_positions (id, account_id, asset_id, created_at, updated_at)
				 VALUES ('00000000-0000-0000-0000-0000000000d1', '00000000-0000-0000-0000-0000000000b1', '00000000-0000-0000-0000-0000000000c1', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`,
				`INSERT INTO investment_position_portfolios (manual_position_id, portfolio_id, created_at, updated_at)
				 VALUES ('00000000-0000-0000-0000-0000000000d1', '00000000-0000-0000-0000-0000000000a1', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`,
			} {
				if _, err := conn.Exec(query); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			if _, err := provider.UpTo(ctx, 5); err != nil {
				t.Fatalf("up over existing groupings: %v", err)
			}
			if !tableExists(t, conn, backend.Name, "goals") ||
				tableExists(t, conn, backend.Name, "investment_portfolios") ||
				tableExists(t, conn, backend.Name, "investment_position_portfolios") {
				t.Fatal("goals did not replace the grouping tables")
			}
			var positions int
			if err := conn.QueryRow(`SELECT COUNT(*) FROM investment_positions`).Scan(&positions); err != nil || positions != 1 {
				t.Fatalf("positions after migration = %d, %v; the grouping must not take holdings with it", positions, err)
			}

			target := map[string]string{"zero": "0", "negative": "-1"}
			for name, amount := range target {
				if _, err := conn.Exec(`INSERT INTO goals (id, name, metric, target_amount, created_at, updated_at)
					VALUES ('00000000-0000-0000-0000-0000000000e1', 'x', 'invested_value', '` + amount + `', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`); err == nil {
					t.Errorf("a %s target was accepted", name)
				}
			}
			if _, err := conn.Exec(`INSERT INTO goals (id, name, metric, target_amount, created_at, updated_at)
				VALUES ('00000000-0000-0000-0000-0000000000e1', 'x', 'bogus', '10', '2026-10-12T00:00:00Z', '2026-10-12T00:00:00Z')`); err == nil {
				t.Error("an unknown metric was accepted")
			}

			if _, err := provider.DownTo(ctx, 4); err != nil {
				t.Fatalf("down: %v", err)
			}
			if tableExists(t, conn, backend.Name, "goals") || !tableExists(t, conn, backend.Name, "investment_portfolios") ||
				!tableExists(t, conn, backend.Name, "investment_position_portfolios") {
				t.Fatal("rollback did not restore the grouping tables")
			}
			if _, err := provider.Up(ctx); err != nil {
				t.Fatalf("re-up: %v", err)
			}
		})
	}
}
