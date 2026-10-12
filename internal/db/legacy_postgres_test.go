package db

import (
	"os"
	"strings"
	"testing"
)

func TestAutomaticLegacyPostgresConversion(t *testing.T) {
	conn, _ := postgresMigrationProviderAt(t, 46)
	mustExec(t, conn, `INSERT INTO auth_owner VALUES(1,'owner@example.invalid','synthetic-hash',7,0)`)
	mustExec(t, conn, `INSERT INTO auth_sessions VALUES('synthetic-session',1,7,0,1,604800)`)
	mustExec(t, conn, `INSERT INTO settings VALUES('encrypted','synthetic-ciphertext',1,'2026-10-10T02:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_accounts(id,name,account_type,currency_code,origin,opening_balance,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000001','Synthetic','BANK','BRL','manual','100.00000000000000000001','2026-10-10T02:00:00Z','2026-10-10T02:00:00Z')`)
	mustExec(t, conn, `INSERT INTO financial_transactions(id,account_id,origin,amount,amount_in_account_currency,currency_code,provider_status,occurred_at,created_at,updated_at) VALUES('00000000-0000-0000-0000-000000000002','00000000-0000-0000-0000-000000000001','manual','-12.00000000000000000001','-12.00000000000000000001','BRL','POSTED','2026-10-10T02:00:00Z','2026-10-10T02:00:00Z','2026-10-10T02:00:00Z')`)
	mustExec(t, conn, `INSERT INTO account_balance_adjustments VALUES('00000000-0000-0000-0000-000000000003','00000000-0000-0000-0000-000000000001','2','Synthetic','2026-10-10T02:00:00Z')`)
	var originalOID int64
	if err := conn.QueryRow(`SELECT 'auth_owner'::regclass::oid`).Scan(&originalOID); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(os.Getenv("JULIUS_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	var balance, ciphertext string
	if err = opened.QueryRow(`SELECT opening_balance + (SELECT SUM(amount_in_account_currency) FROM financial_transactions) + (SELECT SUM(amount) FROM account_balance_adjustments) FROM financial_accounts`).Scan(&balance); err != nil || balance != "90.00000000000000000000" {
		t.Fatal(balance, err)
	}
	if err = opened.QueryRow(`SELECT value FROM settings WHERE key='encrypted'`).Scan(&ciphertext); err != nil || ciphertext != "synthetic-ciphertext" {
		t.Fatal(ciphertext, err)
	}
	var backup string
	if err = opened.QueryRow(`SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.oid=?`, originalOID).Scan(&backup); err != nil {
		t.Fatal(err)
	}
	var unix int64
	if err = opened.QueryRow(`SELECT created_at FROM ` + pgTable(backup, "auth_owner")).Scan(&unix); err != nil || unix != 0 {
		t.Fatal(unix, err)
	}
	var epoch int64
	if err = opened.QueryRow(`SELECT EXTRACT(EPOCH FROM expires_at)::bigint FROM auth_sessions`).Scan(&epoch); err != nil || epoch != 604800 {
		t.Fatal(epoch, err)
	}
	again, err := Open(os.Getenv("JULIUS_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	again.Close()
}

func TestAutomaticLegacyPostgresRollback(t *testing.T) {
	conn, _ := postgresMigrationProviderAt(t, 46)
	mustExec(t, conn, `INSERT INTO settings VALUES('invalid','synthetic',1,'invalid-time')`)
	opened, err := Open(os.Getenv("JULIUS_TEST_POSTGRES_DSN"))
	if err == nil {
		opened.Close()
		t.Fatal("accepted invalid timestamp")
	}
	if !strings.Contains(err.Error(), "settings") {
		t.Fatal(err)
	}
	var original string
	if err = conn.QueryRow(`SELECT updated_at FROM settings WHERE key='invalid'`).Scan(&original); err != nil || original != "invalid-time" {
		t.Fatal(original, err)
	}
	var marker bool
	if err = conn.QueryRow(`SELECT EXISTS(SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='julius_schema_version')`).Scan(&marker); err != nil || marker {
		t.Fatal(marker, err)
	}
	mustExec(t, conn, `UPDATE settings SET updated_at='2026-10-10T02:00:00Z'`)
	opened, err = Open(os.Getenv("JULIUS_TEST_POSTGRES_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	opened.Close()
}

func TestAutomaticLegacyPostgresConcurrentStartup(t *testing.T) {
	conn, _ := postgresMigrationProviderAt(t, 46)
	var before int
	if err := conn.QueryRow(`SELECT count(*) FROM information_schema.schemata WHERE schema_name LIKE 'julius_legacy_%'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			opened, err := Open(os.Getenv("JULIUS_TEST_POSTGRES_DSN"))
			if err == nil {
				opened.Close()
			}
			results <- err
		}()
	}
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	var after int
	if err := conn.QueryRow(`SELECT count(*) FROM information_schema.schemata WHERE schema_name LIKE 'julius_legacy_%'`).Scan(&after); err != nil || after != before+1 {
		t.Fatal(before, after, err)
	}
}

func TestLegacyPostgresCalendarConversion(t *testing.T) {
	conn, _ := postgresMigrationProviderAt(t, 46)
	for _, tc := range []struct{ table, col, input, want string }{
		{"financial_bills", "due_date", "2026-10-10T02:00:00Z", "2026-10-10"},
		{"financial_bills", "due_date", "2026-10-10T23:59:59.999999999Z", "2026-10-10"},
		{"financial_investment_transactions", "trade_date", "2026-10-10T00:00:00.000000001Z", "2026-10-09"},
		{"financial_investments", "due_date", "2026-10-10T02:00:00Z", "2026-10-09"},
		{"financial_investment_transactions", "trade_date", "2026-10-10T00:00:00Z", "2026-10-10"},
		{"financial_investment_transactions", "trade_date", "2026-10-10T02:00:00Z", "2026-10-09"},
	} {
		var got string
		query := `SELECT (` + legacyPostgresExpr(tc.table, tc.col, "date") + `)::text FROM (SELECT ?::text AS ` + pgIdent(tc.col) + `) input`
		if err := conn.QueryRow(query, tc.input).Scan(&got); err != nil || got != tc.want {
			t.Errorf("%s.%s got=%s err=%v want=%s", tc.table, tc.col, got, err, tc.want)
		}
	}
}
