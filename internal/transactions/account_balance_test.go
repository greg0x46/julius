package transactions_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
	"github.com/shopspring/decimal"
)

func TestManualAccountBalanceBothDialects(t *testing.T) {
	dbtest.Each(t, func(t *testing.T, conn *sql.DB) {
		ctx := context.Background()
		now := db.FormatTime(time.Now())
		_, err := conn.Exec(`INSERT INTO financial_accounts(id,origin,name,account_type,currency_code,opening_balance,created_at,updated_at) VALUES('052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c','manual','USD','BANK','USD','100',?,?)`, now, now)
		if err != nil {
			t.Fatal(err)
		}
		id, err := transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: "052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c", Description: "Deposit", Amount: decimal.RequireFromString("10"), OccurredAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		var amount, currency string
		if err := conn.QueryRow(`SELECT amount, currency_code FROM financial_transactions WHERE id=?`, id).Scan(&amount, &currency); err != nil || amount != "10" || currency != "USD" {
			t.Fatalf("money %s %s: %v", amount, currency, err)
		}
		balance, err := transactions.AccountBalance(ctx, conn, "052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c")
		if err != nil || *balance != "110" {
			t.Fatalf("balance %v: %v", balance, err)
		}
		_, err = conn.Exec(`INSERT INTO account_balance_adjustments(id,account_id,amount,description,created_at) VALUES('1ee0cccf-77d3-54ed-8f55-f6e5dafb0483','052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c','-5','Correction',?)`, now)
		if err != nil {
			t.Fatal(err)
		}
		balance, err = transactions.AccountBalance(ctx, conn, "052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c")
		if err != nil || *balance != "105" {
			t.Fatalf("adjusted balance %v: %v", balance, err)
		}
		cash, err := transactions.CashOnHandIn(ctx, conn, nil, money.BRL)
		if err != nil || !cash.IsZero() {
			t.Fatalf("foreign cash mixed into BRL: %v %v", cash, err)
		}
		if _, err := conn.Exec(`UPDATE financial_accounts SET currency_code='BRL' WHERE id='052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c'`); err != nil {
			t.Fatal(err)
		}
		cash, err = transactions.CashOnHandIn(ctx, conn, nil, money.BRL)
		if err != nil || cash.String() != "105" {
			t.Fatalf("manual cash: %v %v", cash, err)
		}
		if _, err := conn.Exec(`UPDATE financial_accounts SET balance='105' WHERE id='052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c'`); err == nil {
			t.Fatal("accepted second authoritative balance")
		}
		if _, err := conn.Exec(`UPDATE financial_accounts SET source_id='899b45f1-2113-5832-8612-d3cdda30672f' WHERE id='052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c'`); err == nil {
			t.Fatal("accepted fake manual identity")
		}
		if _, err := conn.Exec(`UPDATE financial_accounts SET archived_at=? WHERE id='052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c'`, now); err != nil {
			t.Fatal(err)
		}
		_, err = transactions.CreateManual(ctx, conn, transactions.ManualInput{AccountID: "052e2a8b-6b4c-5c1d-90c3-2f8654bcd27c", Amount: decimal.RequireFromString("10"), OccurredAt: time.Now()})
		if err != transactions.ErrAccountArchived {
			t.Fatalf("archive: %v", err)
		}
	})
}
