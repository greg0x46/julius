package networth_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/investments"
	"github.com/greg0x46/julius/internal/networth"
	"github.com/greg0x46/julius/internal/timeline"
	"github.com/shopspring/decimal"
)

func TestManualInvestmentsAndLinkedBrokerageCashCountOnce(t *testing.T) {
	f := newFixture(t)
	bankID := f.addAccount("BANK", "500")
	f.addInvestment("2000")
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('manual','Manual','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('opening','manual','initial_balance','2026-09-01','1000',?,?)`, now, now)
	f.exec(`INSERT INTO investment_assets (id,canonical_key,name,asset_type,created_at,updated_at) VALUES ('asset','ticker:ACOES','Ações','EQUITY',?,?)`, now, now)
	f.exec(`INSERT INTO investment_positions (id,account_id,asset_id,created_at,updated_at) VALUES ('position','manual','asset',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,position_id,kind,occurred_on,amount,quantity,unit_price,created_at,updated_at) VALUES ('buy','manual','position','buy','2026-09-02','600','6','100',?,?)`, now, now)
	b, err := networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "cash", b.CashBalance, "500")
	assertDecimalEqual(t, "investments", b.InvestmentBalance, "3000")
	assertDecimalEqual(t, "net worth", b.NetWorth, "3500")
	// A linked bank cash snapshot must not also add the manual cash ledger.
	f.exec(`UPDATE investment_accounts SET financial_account_id=? WHERE id='manual'`, bankID)
	b, err = networth.Compute(context.Background(), f.conn)
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "linked investments", b.InvestmentBalance, "2600")
	assertDecimalEqual(t, "linked net worth", b.NetWorth, "3100")
}

func TestManualHoldingInIntegratedAccountCountsOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	f.addInvestment("2000")
	if err := investments.EnsureIntegratedAccounts(ctx, f.conn); err != nil {
		t.Fatal(err)
	}
	ticker := "BTC"
	if _, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{
		AccountID: "integrated:" + f.sourceID, Name: "Bitcoin", Ticker: &ticker, AssetType: "Criptoativo",
		InitialQuantity: decimal.RequireFromString("0.5"), InitialUnitCost: decimal.RequireFromString("1000"),
		OccurredOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	b, err := networth.Compute(ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	assertDecimalEqual(t, "investments", b.InvestmentBalance, "2500")
	assertDecimalEqual(t, "net worth", b.NetWorth, "2500")
}

func TestInvestmentLifecycleConservesWealthAndSeparatesSpending(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	bank := f.addAccount("BANK", "5000")
	account, err := investments.CreateAccount(ctx, f.conn, investments.AccountInput{Name: "Corretora"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := investments.CreatePosition(ctx, f.conn, investments.PositionInput{AccountID: account.ID, Name: "Ativo", AssetType: "Ações"})
	if err != nil {
		t.Fatal(err)
	}
	day := func(n int) time.Time { return time.Date(2026, 9, n, 0, 0, 0, 0, time.UTC) }
	dec := func(s string) decimal.Decimal { return decimal.RequireFromString(s) }
	ptr := func(s string) *decimal.Decimal { v := dec(s); return &v }
	operation := func(kind investments.OperationKind, date int, amount string, quantity *decimal.Decimal) investments.Operation {
		in := investments.OperationInput{AccountID: account.ID, Kind: kind, OccurredOn: day(date), Amount: dec(amount), Quantity: quantity}
		if kind == investments.OperationBuy || kind == investments.OperationSell || kind == investments.OperationValuation {
			in.PositionID = &position.ID
		}
		op, err := investments.CreateOperation(ctx, f.conn, in)
		if err != nil {
			t.Fatal(err)
		}
		return op
	}
	wealth := func(want string) {
		t.Helper()
		b, err := networth.Compute(ctx, f.conn)
		if err != nil {
			t.Fatal(err)
		}
		assertDecimalEqual(t, "wealth", b.NetWorth, want)
	}
	wealth("5000")
	deposit := operation(investments.OperationDeposit, 1, "1000", nil)
	// The provider reports the actual bank transfer; recording a manual bank
	// line alone never changes its authoritative balance.
	f.exec(`UPDATE financial_accounts SET balance='4000' WHERE id=?`, bank)
	bankDeposit := f.addCardTransaction(bank, day(1), "-1000", "DEBIT")
	if _, err := investments.CreateReconciliation(ctx, f.conn, investments.ReconciliationInput{OperationID: deposit.ID, FinancialTransactionID: &bankDeposit, Amount: dec("1000")}); err != nil {
		t.Fatal(err)
	}
	wealth("5000")
	operation(investments.OperationBuy, 2, "1000", ptr("10"))
	wealth("5000")
	operation(investments.OperationValuation, 3, "1200", nil)
	wealth("5200")
	operation(investments.OperationSell, 4, "480", ptr("4"))
	wealth("5200")
	withdrawal := operation(investments.OperationWithdrawal, 5, "480", nil)
	f.exec(`UPDATE financial_accounts SET balance='4480' WHERE id=?`, bank)
	bankWithdrawal := f.addCardTransaction(bank, day(5), "480", "CREDIT")
	if _, err := investments.CreateReconciliation(ctx, f.conn, investments.ReconciliationInput{OperationID: withdrawal.ID, FinancialTransactionID: &bankWithdrawal, Amount: dec("480")}); err != nil {
		t.Fatal(err)
	}
	wealth("5200")
	series, err := timeline.BuildSeries(ctx, f.conn, timeline.BuildParams{From: day(1), To: day(30), ReferenceDate: day(17)})
	if err != nil {
		t.Fatal(err)
	}
	totals := timeline.TotalsForPeriod(series)
	if !totals.Expense.IsZero() || !totals.Income.IsZero() {
		t.Fatalf("totals %+v", totals)
	}
}
