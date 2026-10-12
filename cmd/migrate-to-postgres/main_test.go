package main

import (
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/db/types"
)

func TestCopyTypedBaseline(t *testing.T) {
	src := dbtest.OpenSQLite(t)
	dst := dbtest.OpenPostgres(t)
	for _, query := range []string{
		`INSERT INTO auth_owner (id, email, password_hash, revision, created_at) VALUES
		 (1, 'owner@example.invalid', 'synthetic', 7, '2026-01-01T00:00:00Z')`,
		`INSERT INTO settings (key, value, is_encrypted, updated_at) VALUES
		 ('plain', NULL, 0, '2026-01-01T00:00:00Z'),
		 ('encrypted', 'synthetic', 1, '2026-01-01T00:00:00Z')`,
		`INSERT INTO automation_rules (id, name, is_active, logic_operator, created_at, updated_at) VALUES
		 ('00000000-0000-0000-0000-000000000001', 'Active', 1, 'and', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z'),
		 ('00000000-0000-0000-0000-000000000002', 'Inactive', 0, 'or', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := src.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	tables, err := orderedTables(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec("TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		if _, err := copyTable(src, dst, table); err != nil {
			t.Fatalf("copy %s: %v", table, err)
		}
	}
	for _, check := range []struct {
		query string
		want  bool
	}{
		{`SELECT is_encrypted FROM settings WHERE key = 'plain'`, false},
		{`SELECT is_encrypted FROM settings WHERE key = 'encrypted'`, true},
		{`SELECT is_active FROM automation_rules WHERE name = 'Active'`, true},
		{`SELECT is_active FROM automation_rules WHERE name = 'Inactive'`, false},
	} {
		var got bool
		if err := dst.QueryRow(check.query).Scan(types.Bool(&got)); err != nil {
			t.Fatal(err)
		}
		if got != check.want {
			t.Errorf("%s: got %t, want %t", check.query, got, check.want)
		}
	}
	var value any
	if err := dst.QueryRow(`SELECT value FROM settings WHERE key = 'plain'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != nil {
		t.Errorf("NULL setting value changed to %v", value)
	}
	var revision int
	if err := dst.QueryRow(`SELECT revision FROM auth_owner WHERE id = 1`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 7 {
		t.Errorf("integer revision changed to %d", revision)
	}
}
