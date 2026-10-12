package db

import (
	"context"
	"database/sql"
	"testing"

	"github.com/pressly/goose/v3"
)

func TestManualAccountMigrationPreservesSyncedData(t *testing.T) {
	for _, backend := range []struct {
		name     string
		open     func(*testing.T, int64) (*sql.DB, *goose.Provider)
		previous int64
	}{
		{"sqlite", migrationProviderAt, 46}, {"postgres", postgresMigrationProviderAt, 45},
	} {
		t.Run(backend.name, func(t *testing.T) {
			conn, provider := backend.open(t, backend.previous)
			now := "2026-01-01T00:00:00Z"
			mustExec(t, conn, `INSERT INTO data_sources(id,provider,external_item_id,created_at,updated_at) VALUES('source','pluggy','item',?,?)`, now, now)
			mustExec(t, conn, `INSERT INTO sync_runs(id,source_id,status,started_at,finished_at) VALUES('run','source','completed',?,?)`, now, now)
			mustExec(t, conn, `INSERT INTO raw_imports(id,sync_run_id,source_id,scope,page_sequence,request_attempt,request_method,request_path,http_status,response_headers,payload,payload_sha256,received_at) VALUES('raw','run','source','accounts',1,1,'GET','/accounts',200,'{}',?,'sha',?)`, []byte("{}"), now)
			mustExec(t, conn, `INSERT INTO financial_accounts(id,source_id,external_id,balance,currency_code,account_type,current_raw_import_id,normalized_hash,created_at,updated_at,manual_closing_day,balance_as_of) VALUES('account','source','external','150.25','EUR','CREDIT','raw','hash',?,?,12,?)`, now, now, now)
			mustExec(t, conn, `INSERT INTO financial_transactions(id,account_id,origin,amount,amount_in_account_currency,currency_code,provider_status,movement_type,created_at,updated_at) VALUES('movement','account','manual','10','10','EUR','POSTED','CREDIT',?,?)`, now, now)
			if _, err := provider.Up(context.Background()); err != nil {
				t.Fatal(err)
			}
			var origin, source, external, balance, raw, hash, asOf string
			var closing int
			if err := conn.QueryRow(`SELECT origin,source_id,external_id,balance,current_raw_import_id,normalized_hash,manual_closing_day,balance_as_of FROM financial_accounts WHERE id='account'`).Scan(&origin, &source, &external, &balance, &raw, &hash, &closing, &asOf); err != nil {
				t.Fatal(err)
			}
			if origin != "synced" || source != "source" || external != "external" || balance != "150.25" || raw != "raw" || hash != "hash" || closing != 12 || asOf != now {
				t.Fatalf("lost account data: %s %s %s %s %s %s %d %s", origin, source, external, balance, raw, hash, closing, asOf)
			}
			var accountID string
			if err := conn.QueryRow(`SELECT account_id FROM financial_transactions WHERE id='movement'`).Scan(&accountID); err != nil || accountID != "account" {
				t.Fatalf("lost child: %s %v", accountID, err)
			}
			mustExec(t, conn, `UPDATE financial_accounts SET balance='175', normalized_hash='new-hash' WHERE source_id='source' AND external_id='external'`)
			if _, err := conn.Exec(`DELETE FROM financial_accounts WHERE id='account'`); err == nil {
				t.Fatal("foreign key lost")
			}
		})
	}
}
