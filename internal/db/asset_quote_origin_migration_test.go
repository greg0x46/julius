package db

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestAssetQuoteOriginMigrationClassifiesExistingPricesBySource(t *testing.T) {
	conn, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetMaxOpenConns(1)
	migrations, err := fs.Sub(legacyMigrationsFS, "migrations/archive/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectSQLite3, conn, migrations)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := provider.UpTo(ctx, 43); err != nil {
		t.Fatal(err)
	}

	mustExec(t, conn, `INSERT INTO investment_assets
		(id, canonical_key, name, asset_type, currency_code, created_at, updated_at)
		VALUES ('asset-1', 'name:acao:petr4', 'Petrobras PN', 'Ação', 'BRL', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	cases := []struct{ source, origin string }{
		{"pluggy", "sync"}, {"issue", "issue"},
		{"yahoo", "market"}, {"brapi", "market"}, {"coingecko", "market"}, {"manual", "market"},
	}
	for i, tc := range cases {
		mustExec(t, conn, `INSERT INTO investment_asset_quotes
			(asset_id, quoted_on, price, source, created_at, updated_at)
			VALUES ('asset-1', ?, '10', ?, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			"2026-09-0"+string(rune('1'+i)), tc.source)
	}

	if _, err := provider.UpTo(ctx, 44); err != nil {
		t.Fatal(err)
	}
	assertOrigins := func(stage string) {
		t.Helper()
		for _, tc := range cases {
			var origin string
			if err := conn.QueryRow(`SELECT origin FROM investment_asset_quotes WHERE source = ?`, tc.source).Scan(&origin); err != nil {
				t.Fatalf("%s: %s: %v", stage, tc.source, err)
			}
			if origin != tc.origin {
				t.Errorf("%s: a %s price became %q, want %q", stage, tc.source, origin, tc.origin)
			}
		}
	}
	assertOrigins("after up")

	// A writer that does not say what its price is gets the least privileged
	// kind, the one a market quote may replace.
	mustExec(t, conn, `INSERT INTO investment_asset_quotes
		(asset_id, quoted_on, price, source, created_at, updated_at)
		VALUES ('asset-1', '2026-09-21', '10', 'unknown-writer', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)
	var defaulted string
	if err := conn.QueryRow(`SELECT origin FROM investment_asset_quotes WHERE source = 'unknown-writer'`).Scan(&defaulted); err != nil || defaulted != "market" {
		t.Errorf("a price written without an origin became %q (%v), want market", defaulted, err)
	}
	mustExec(t, conn, `DELETE FROM investment_asset_quotes WHERE source = 'unknown-writer'`)

	// New rows must say what they are, and only one of the three kinds.
	if _, err := conn.Exec(`INSERT INTO investment_asset_quotes
		(asset_id, quoted_on, price, source, origin, created_at, updated_at)
		VALUES ('asset-1', '2026-09-20', '10', 'pluggy', 'guess', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err == nil {
		t.Error("an unknown origin was accepted")
	}

	// Rolling back drops only the column; the prices stay and classify again.
	if _, err := provider.Down(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM investment_asset_quotes").Scan(&count); err != nil || count != len(cases) {
		t.Fatalf("rollback lost prices: %d, %v", count, err)
	}
	if _, err := conn.Exec("SELECT origin FROM investment_asset_quotes"); err == nil {
		t.Error("origin survived the rollback")
	}
	if _, err := provider.UpTo(ctx, 44); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	assertOrigins("after re-up")
}
