package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/db/types"
)

const stamp = "2026-10-10T02:00:00.000000000Z"
const accountID = "00000000-0000-0000-0000-000000000001"

func legacyFixture(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetMaxOpenConns(1)
	t.Cleanup(func() { conn.Close() })
	schema, err := os.ReadFile("../../.specs/reorganizacao-schema/legado-sqlite.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(string(schema)); err != nil {
		t.Fatal(err)
	}
	queries := []string{
		`INSERT INTO auth_owner VALUES (1,'owner@example.invalid','synthetic',7,0)`,
		`INSERT INTO auth_sessions VALUES ('session-hash',1,7,0,1,604800)`,
		`INSERT INTO settings VALUES ('encrypted','synthetic-ciphertext',1,'` + stamp + `')`,
		`INSERT INTO settings VALUES ('nullable',NULL,0,'` + stamp + `')`,
		`INSERT INTO categories (id,name,kind,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000002','Synthetic','expense','` + stamp + `','` + stamp + `')`,
		`INSERT INTO financial_accounts (id,name,account_type,currency_code,origin,opening_balance,balance_close_date,balance_due_date,created_at,updated_at) VALUES ('` + accountID + `','Synthetic','BANK','BRL','manual','100.00000000000000000001','` + stamp + `',NULL,'` + stamp + `','` + stamp + `')`,
		`INSERT INTO financial_transactions (id,account_id,origin,amount,amount_in_account_currency,currency_code,provider_status,occurred_at,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000003','` + accountID + `','manual','-12.00000000000000000001','-12.00000000000000000001','BRL','POSTED','` + stamp + `','` + stamp + `','` + stamp + `')`,
		`INSERT INTO account_balance_adjustments VALUES ('00000000-0000-0000-0000-000000000004','` + accountID + `','2','Synthetic','` + stamp + `')`,
		`INSERT INTO transaction_category_decisions VALUES ('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000002',1,'` + stamp + `','manual')`,
		`INSERT INTO data_sources (id,provider,external_item_id,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000005','csv','synthetic','` + stamp + `','` + stamp + `')`,
		`INSERT INTO sync_runs (id,source_id,status,started_at,finished_at) VALUES ('00000000-0000-0000-0000-000000000006','00000000-0000-0000-0000-000000000005','completed','` + stamp + `','` + stamp + `')`,
		`INSERT INTO raw_imports (id,sync_run_id,source_id,scope,page_sequence,request_attempt,payload,payload_sha256,received_at) VALUES ('00000000-0000-0000-0000-000000000007','00000000-0000-0000-0000-000000000006','00000000-0000-0000-0000-000000000005','file',1,1,X'0001FF','synthetic-hash','` + stamp + `')`,
		`INSERT INTO financial_accounts (id,source_id,external_id,current_raw_import_id,normalized_hash,balance,currency_code,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000008','00000000-0000-0000-0000-000000000005','opaque-provider-account','00000000-0000-0000-0000-000000000007','synthetic-hash','123.456789012345678901','USD','` + stamp + `','` + stamp + `')`,
		`INSERT INTO financial_transactions (id,account_id,origin,amount,amount_in_account_currency,currency_code,provider_status,occurred_at,created_at,updated_at,deleted_at) VALUES ('00000000-0000-0000-0000-000000000009','` + accountID + `','manual','-3','-3','BRL','POSTED','` + stamp + `','` + stamp + `','` + stamp + `','` + stamp + `')`,
		`INSERT INTO financial_transactions (id,account_id,origin,amount,amount_in_account_currency,currency_code,provider_status,occurred_at,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000010','` + accountID + `','manual','5','5','BRL','PENDING','` + stamp + `','` + stamp + `','` + stamp + `')`,
	}
	for _, query := range queries {
		if _, err := conn.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	return conn, path
}

func TestLegacyCopyBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, dst *sql.DB) {
		writer, path := legacyFixture(t)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		src, err := openSource(path)
		if err != nil {
			t.Fatal(err)
		}
		defer src.Close()
		if _, err := src.Exec(`UPDATE auth_owner SET revision=9`); err == nil {
			t.Fatal("source writable")
		}
		var seeds int
		if err := dst.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&seeds); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := migrateLegacy(src, dst, true, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "zero errors") || !strings.Contains(out.String(), "source=90 target=90") {
			t.Fatal(out.String())
		}
		var count int
		if err := dst.QueryRow(`SELECT COUNT(*) FROM auth_owner`).Scan(&count); err != nil || count != 0 {
			t.Fatal("dry-run persisted owner", count, err)
		}
		if err := dst.QueryRow(`SELECT COUNT(*) FROM categories`).Scan(&count); err != nil || count != seeds {
			t.Fatal("dry-run changed seeds", count, err)
		}
		if err := migrateLegacy(src, dst, false, &out); err != nil {
			t.Fatal(err)
		}
		var created, due, balance string
		if err := dst.QueryRow(`SELECT created_at FROM auth_owner`).Scan(types.Time(&created)); err != nil || created != "1970-01-01T00:00:00.000000000Z" {
			t.Fatal(created, err)
		}
		var close string
		var null any
		if err := dst.QueryRow(`SELECT balance_close_date,balance_due_date,opening_balance FROM financial_accounts WHERE origin='manual'`).Scan(types.Date(&close), &null, types.Decimal(&balance)); err != nil {
			t.Fatal(err)
		}
		due = close
		if due != "2026-10-10" || null != nil || balance != "100.00000000000000000001" {
			t.Fatal(due, null, balance)
		}
		var payload []byte
		if err := dst.QueryRow(`SELECT payload FROM raw_imports`).Scan(&payload); err != nil || !bytes.Equal(payload, []byte{0, 1, 255}) {
			t.Fatal(payload, err)
		}
		var encrypted string
		if err := dst.QueryRow(`SELECT value FROM settings WHERE key='encrypted'`).Scan(&encrypted); err != nil || encrypted != "synthetic-ciphertext" {
			t.Fatal(encrypted, err)
		}
		var revision int
		if err := dst.QueryRow(`SELECT revision FROM transaction_category_decisions`).Scan(&revision); err != nil || revision != 1 {
			t.Fatal(revision, err)
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("source changed", err)
		}
		if err := migrateLegacy(src, dst, false, &out); err == nil {
			t.Fatal("accepted populated target")
		}
		writer.Close()
	})
}

func TestDryRunReportsAllErrorsAndRollsBack(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, dst *sql.DB) {
		src, _ := legacyFixture(t)
		for _, q := range []string{
			`PRAGMA ignore_check_constraints=ON`,
			`UPDATE auth_owner SET created_at='bad-unix'`,
			`UPDATE account_balance_adjustments SET amount='bad-money'`,
			`UPDATE financial_accounts SET balance_close_date='bad-date'`,
			`UPDATE settings SET updated_at='bad-time' WHERE key='encrypted'`,
			`INSERT INTO payables (id,kind,name,total_amount,created_at,updated_at) VALUES ('00000000-0000-0000-0000-000000000008','debt','Synthetic','0','` + stamp + `','` + stamp + `')`,
		} {
			if _, err := src.Exec(q); err != nil {
				t.Fatal(err)
			}
		}
		var out bytes.Buffer
		err := migrateLegacy(src, dst, true, &out)
		if err == nil {
			t.Fatal("accepted malformed values")
		}
		for _, want := range []string{"auth_owner [id=1]", "bad-unix", "bad-money", "bad-date", "bad-time", "payables [id="} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("missing %q in %v", want, err)
			}
		}
		var count int
		if err := dst.QueryRow(`SELECT COUNT(*) FROM raw_imports`).Scan(&count); err != nil || count != 0 {
			t.Fatal("dry-run retained data", count, err)
		}
	})
}

func TestConversionMap(t *testing.T) {
	cases := []struct {
		key   string
		input any
		want  any
	}{
		{"auth_sessions.expires_at", int64(0), "1970-01-01T00:00:00.000000000Z"},
		{"financial_bills.closing_date", stamp, "2026-10-10"},
		{"financial_investments.due_date", stamp, "2026-10-09"},
		{"financial_investments.issue_date", stamp, "2026-10-09"},
		{"financial_investments.purchase_date", stamp, "2026-10-09"},
		{"financial_investment_transactions.trade_date", stamp, "2026-10-09"},
		{"financial_investment_transactions.trade_date", "2026-10-10T00:00:00.000000000Z", "2026-10-10"},
		{"financial_investment_transactions.trade_date", "2026-10-10T00:00:00.000000001Z", "2026-10-09"},
		{"investment_portfolios.target_date", "2026-10-10", "2026-10-10"},
		{"settings.is_encrypted", int64(1), true},
		{"settings.value", "opaque", "opaque"},
		{"financial_accounts.balance_due_date", nil, nil},
	}
	for _, tc := range cases {
		got, err := convert(tc.key, tc.input, true)
		if err != nil || got != tc.want {
			t.Errorf("%s: %v %v, want %v", tc.key, got, err, tc.want)
		}
	}
	for _, tc := range []struct {
		key   string
		value any
	}{
		{"settings.is_encrypted", int64(2)}, {"auth_owner.created_at", "0"}, {"financial_accounts.balance", 1.5}, {"financial_accounts.id", "opaque"}, {"investment_portfolios.target_date", "2026-02-30"},
	} {
		if _, err := convert(tc.key, tc.value, true); err == nil {
			t.Errorf("accepted %s=%v", tc.key, tc.value)
		}
	}
	if got, err := convert("financial_accounts.id", "opaque", false); err != nil || got != "opaque" {
		t.Fatal(got, err)
	}
}

func TestConversionKindsCoverBaseline(t *testing.T) {
	dst := dbtest.OpenPostgres(t)
	rows, err := dst.Query(`SELECT table_name,column_name,data_type FROM information_schema.columns WHERE table_schema=current_schema()`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	kinds := map[string]string{"numeric": "numeric", "timestamp with time zone": "timestamptz", "date": "date", "boolean": "boolean", "uuid": "uuid"}
	seen := map[string]bool{}
	for rows.Next() {
		var table, col, kind string
		if err := rows.Scan(&table, &col, &kind); err != nil {
			t.Fatal(err)
		}
		if expected := kinds[kind]; expected != "" && table != "goose_db_version" && table != "goals" && table != "oauth_grants" {
			key := table + "." + col
			seen[key] = true
			if columnKinds[key] != expected {
				t.Errorf("%s: kind %q, want %s", key, columnKinds[key], expected)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	for key := range columnKinds {
		if !seen[key] && !skipped[strings.SplitN(key, ".", 2)[0]] {
			t.Errorf("unknown mapped column %s", key)
		}
	}
}

func TestCopyTableRollback(t *testing.T) {
	src, _ := legacyFixture(t)
	dst := dbtest.OpenSQLite(t)
	// Fail after a valid first row in the same table.
	if _, err := src.Exec(`UPDATE settings SET updated_at='bad-time' WHERE key='nullable'`); err != nil {
		t.Fatal(err)
	}
	sourceTx, err := src.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer sourceTx.Rollback()
	meta, err := columns(dst, "settings")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := dst.Begin()
	if err != nil {
		t.Fatal(err)
	}
	_, failures := copyTable(sourceTx, tx, "settings", meta, false, false)
	if len(failures) == 0 {
		t.Fatal("accepted bad row")
	}
	tx.Rollback()
	var n int
	if err := dst.QueryRow(`SELECT COUNT(*) FROM settings`).Scan(&n); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}

func TestValidationDetectsBalanceMismatch(t *testing.T) {
	src, _ := legacyFixture(t)
	dst := dbtest.OpenSQLite(t)
	var out bytes.Buffer
	if err := migrateLegacy(src, dst, false, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := dst.Exec(`UPDATE account_balance_adjustments SET amount='3'`); err != nil {
		t.Fatal(err)
	}
	tables, err := orderedTables(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(context.Background(), src, dst, tables, &out); err == nil || !strings.Contains(err.Error(), "balance mismatch") {
		t.Fatal(err)
	}
}

func TestDefaultsAndExplicitSkips(t *testing.T) {
	src, _ := legacyFixture(t)
	for _, query := range []string{
		`ALTER TABLE categories DROP COLUMN icon`,
		`CREATE TABLE recurrence_reconciliations (id TEXT PRIMARY KEY)`,
	} {
		if _, err := src.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	dst := dbtest.OpenSQLite(t)
	var out bytes.Buffer
	if err := migrateLegacy(src, dst, false, &out); err != nil {
		t.Fatal(err)
	}
	var icon string
	if err := dst.QueryRow(`SELECT icon FROM categories`).Scan(&icon); err != nil || icon != "ellipsis" {
		t.Fatal(icon, err)
	}
	if !strings.Contains(out.String(), "skip recurrence_reconciliations") {
		t.Fatal(out.String())
	}
}

func TestRejectUnknownSchemaAndEditedSeeds(t *testing.T) {
	for _, scenario := range []string{"unknown table", "unknown column", "missing table", "edited seeds", "typed source"} {
		t.Run(scenario, func(t *testing.T) {
			src, _ := legacyFixture(t)
			dst := dbtest.OpenSQLite(t)
			switch scenario {
			case "unknown table":
				src.Exec(`CREATE TABLE unknown_table (id TEXT)`)
			case "unknown column":
				src.Exec(`ALTER TABLE settings ADD COLUMN unknown_column TEXT`)
			case "missing table":
				src.Exec(`DROP TABLE exchange_rates`)
			case "edited seeds":
				dst.Exec(`UPDATE categories SET name='Edited'`)
			case "typed source":
				src = dbtest.OpenSQLite(t)
			}
			if err := migrateLegacy(src, dst, true, &bytes.Buffer{}); err == nil {
				t.Fatal("accepted", scenario)
			}
		})
	}
}
