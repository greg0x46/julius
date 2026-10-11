package db

import (
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestReferenceCurrencyMigrationPreservesLegacySnapshots(t *testing.T) {
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	migrations, err := fs.Sub(sqliteMigrationsFS, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), 45); err != nil {
		t.Fatal(err)
	}
	checkReferenceCurrencyMigration(t, conn, provider, 46)
}

func TestPostgresReferenceCurrencyMigrationPreservesLegacySnapshots(t *testing.T) {
	conn, provider := postgresMigrationProviderAt(t, 44)
	checkReferenceCurrencyMigration(t, conn, provider, 45)
}

func checkReferenceCurrencyMigration(t *testing.T, conn *sql.DB, provider *goose.Provider, version int64) {
	t.Helper()
	for i, backfilled := range []int{0, 1} {
		mustExec(t, conn, `INSERT INTO net_worth_snapshots
		 (id,captured_at,total_assets,total_liabilities,net_worth,cash_balance,investment_balance,credit_card_balance,payables_debt,is_backfilled,created_at,updated_at)
		 VALUES (?,?,'2100','0','2100','2100','0','0','0',?,'2020-01-01','2020-01-01')`, string(rune('a'+i)), "2020-01-0"+string(rune('1'+i)), backfilled)
	}
	assertLegacy := func() {
		t.Helper()
		rows, err := conn.Query(`SELECT cash_balance, reference_currency, valued_at, rate_policy, fx_rates, valuation_version, valuation_status FROM net_worth_snapshots`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		count := 0
		for rows.Next() {
			var amount, rates, status string
			var currency, at, policy sql.NullString
			var version int
			if err := rows.Scan(&amount, &currency, &at, &policy, &rates, &version, &status); err != nil {
				t.Fatal(err)
			}
			if amount != "2100" || currency.Valid || at.Valid || policy.Valid || version != 0 || status != "legacy_unknown" || rates != "{}" {
				t.Fatalf("legacy row changed: %s %v %v %v %d %s %s", amount, currency, at, policy, version, status, rates)
			}
			count++
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("legacy rows: %d", count)
		}
	}
	if _, err := provider.UpTo(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	assertLegacy()
	mustExec(t, conn, `INSERT INTO fx_rates VALUES ('USD','BRL','2020-01-01T00:00:00.000000000Z','5','test')`)
	if _, err := provider.Down(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(t.Context(), version); err != nil {
		t.Fatal(err)
	}
	assertLegacy()
}
