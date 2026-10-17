package transactions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestReconciledInvestmentSplitFlowsThroughQueryAndCategories(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	accountID := f.addAccount(account{CurrencyCode: strp("BRL"), Balance: strp("500")})
	day := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	id := f.addTransaction(txn{AccountID: accountID, Amount: strp("-1050"), CurrencyCode: strp("BRL"), OccurredAt: &day, ProviderStatus: strp("POSTED"), MovementType: strp("DEBIT")})
	now := db.FormatTime(day)
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('invest','Carteira','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('deposit','invest','deposit','2026-09-10','1000',?,?)`, now, now)
	f.exec(`INSERT INTO investment_reconciliations (id,operation_id,financial_transaction_id,amount,created_at) VALUES ('link','deposit',?,'1000',?)`, id, now)
	result, err := transactions.Query(ctx, f.conn, transactions.QueryRequest{Timezone: "UTC", GroupBy: money.GroupMonth, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Totals) != 1 || result.Totals[0].Outflow != "50" || len(result.Groups) != 1 || result.Groups[0].Totals[0].Outflow != "50" {
		t.Fatalf("totals = %+v groups = %+v", result.Totals, result.Groups)
	}
	item, found, err := transactions.GetItem(ctx, f.conn, id)
	if err != nil || !found {
		t.Fatalf("GetItem: %v found=%v", err, found)
	}
	if item.ReportableAmount == nil || *item.ReportableAmount != "50" || item.EffectiveMoney.Value != "-1050" {
		t.Fatalf("item = %+v", item)
	}
	spending, err := transactions.SpendingByCategory(ctx, f.conn, transactions.Filters{}, "UTC")
	if err != nil || len(spending) != 1 || spending[0].Amount != "50" {
		t.Fatalf("spending = %+v, err=%v", spending, err)
	}
	cash, err := transactions.CashOnHandIn(ctx, f.conn, nil, money.BRL)
	if err != nil || cash.String() != "500" {
		t.Fatalf("cash changed: %s err=%v", cash, err)
	}
	f.exec(`DELETE FROM investment_reconciliations WHERE id='link'`)
	item, _, err = transactions.GetItem(ctx, f.conn, id)
	if err != nil || item.ReportableAmount == nil || *item.ReportableAmount != "1050" {
		t.Fatalf("unlink did not restore reporting: %+v err=%v", item, err)
	}
}

func TestManualBankTransactionCannotInvalidateInvestmentLink(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	accountID := f.addAccount(account{CurrencyCode: strp("BRL")})
	input := transactions.ManualInput{AccountID: accountID, Description: "Aporte", Amount: decimal.NewFromInt(-1000), OccurredAt: time.Now()}
	id, err := transactions.CreateManual(ctx, f.conn, input)
	if err != nil {
		t.Fatal(err)
	}
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('invest','Carteira','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('deposit','invest','deposit','2026-09-10','1000',?,?)`, now, now)
	f.exec(`INSERT INTO investment_reconciliations (id,operation_id,financial_transaction_id,amount,created_at) VALUES ('link','deposit',?,'1000',?)`, id, now)
	if err := transactions.UpdateManual(ctx, f.conn, id, input); !errors.Is(err, transactions.ErrInvestmentLinked) {
		t.Fatalf("update = %v", err)
	}
	if err := transactions.DeleteManual(ctx, f.conn, id, nil); !errors.Is(err, transactions.ErrInvestmentLinked) {
		t.Fatalf("delete = %v", err)
	}
	f.exec(`DELETE FROM investment_reconciliations WHERE id='link'`)
	if err := transactions.DeleteManual(ctx, f.conn, id, nil); err != nil {
		t.Fatal(err)
	}
}
