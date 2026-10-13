package db

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestAssetClassificationMigrationPreservesExistingTypes(t *testing.T) {
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
	if _, err := provider.UpTo(ctx, 42); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ kind, class string }{
		{"FIXED_INCOME", "fixed_income"}, {"AÇÃO", "variable_income"},
		{"Criptomoeda", "crypto"}, {"ETF de renda fixa", "fixed_income"},
		{"ETF de criptoativos", "crypto"}, {"ETF", "other"}, {"Fundo", "other"},
		{"Fundo multimercado", "multimarket"}, {"Fundo cambial", "currency"},
	}
	for _, tc := range cases {
		mustExec(t, conn, `INSERT INTO investment_assets
			(id, canonical_key, name, asset_type, currency_code, created_at, updated_at)
			VALUES (?, ?, ?, ?, 'BRL', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
			tc.kind, "name:"+tc.kind, "Ativo "+tc.kind, tc.kind)
	}
	if _, err := provider.UpTo(ctx, 43); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		var class, kind string
		if err := conn.QueryRow(`SELECT asset_class, asset_type FROM investment_assets WHERE id = ?`, tc.kind).Scan(&class, &kind); err != nil {
			t.Fatal(err)
		}
		if class != tc.class || kind != tc.kind {
			t.Errorf("%s became %s / %s, want %s and original type", tc.kind, class, kind, tc.class)
		}
	}
	if _, err := provider.Down(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM investment_assets").Scan(&count); err != nil || count != len(cases) {
		t.Fatalf("rollback lost catalog rows: %d, %v", count, err)
	}
}
