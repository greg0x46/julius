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

func TestTransferMigrationPreservesExistingMovements(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		dialect, directory := goose.DialectSQLite3, "sqlite"
		if dbtest.IsPostgres(conn) {
			dialect, directory = goose.DialectPostgres, "postgres"
		}
		provider, err := goose.NewProvider(dialect, conn, os.DirFS("migrations/"+directory))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.DownTo(ctx, 1); err != nil {
			t.Fatal(err)
		}
		const account = "11111111-1111-4111-8111-111111111111"
		const movement = "22222222-2222-4222-8222-222222222222"
		const now = "2026-10-01T12:00:00Z"
		const amount = "-540.00000000000000000001"
		if _, err := conn.Exec(`INSERT INTO financial_accounts (id, name, origin, account_type, currency_code, opening_balance, created_at, updated_at) VALUES (?, 'Synthetic', 'manual', 'BANK', 'BRL', '1000', ?, ?)`, account, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Exec(`INSERT INTO financial_transactions (id, account_id, origin, amount, amount_in_account_currency, currency_code, provider_status, movement_type, occurred_at, created_at, updated_at) VALUES (?, ?, 'manual', ?, ?, 'BRL', 'POSTED', 'DEBIT', ?, ?, ?)`, movement, account, amount, amount, now, now, now); err != nil {
			t.Fatal(err)
		}
		if _, err := provider.Up(ctx); err != nil {
			t.Fatal(err)
		}
		var retained string
		if err := conn.QueryRow(`SELECT amount_in_account_currency FROM financial_transactions WHERE id = ?`, movement).Scan(dbtypes.Decimal(&retained)); err != nil || retained != amount {
			t.Fatalf("native amount: %q %v", retained, err)
		}
		var count int
		if err := conn.QueryRow(`SELECT COUNT(*) FROM transfer_links`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("new annotations: %d %v", count, err)
		}
		if _, err := provider.Up(ctx); err != nil {
			t.Fatal(err)
		}
	})
}
