package timeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db"
	"github.com/greg0x46/julius/internal/timeline"
)

func TestInvestmentReportingKeepsBankCurveAndAvoidsDoubleContributions(t *testing.T) {
	f := newFixture(t)
	bankID := f.addAccount("500")
	id := f.addTransaction(txn{AccountID: bankID, Amount: "-1050", OccurredAt: date(t, "2026-09-10")})
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('manual','Manual','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('deposit','manual','deposit','2026-09-10','1000',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('income','manual','income','2026-09-11','100',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('fee','manual','fee','2026-09-12','10',?,?)`, now, now)
	f.exec(`INSERT INTO investment_reconciliations (id,operation_id,financial_transaction_id,amount,created_at) VALUES ('link','deposit',?,'1000',?)`, id, now)
	params := timeline.BuildParams{From: date(t, "2026-09-01"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-09-17")}
	series, err := timeline.BuildSeries(context.Background(), f.conn, params)
	if err != nil {
		t.Fatal(err)
	}
	totals := timeline.TotalsForPeriod(series)
	if totals.Expense.String() != "60" || totals.Income.String() != "100" {
		t.Fatalf("totals = %+v", totals)
	}
	if series.Points[0].Balance.String() != "1550" || series.Points[len(series.Points)-1].Balance.String() != "500" {
		t.Fatalf("cash curve changed: %+v", series.Points)
	}
	params.AccountIDs = []string{bankID}
	filtered, err := timeline.BuildSeries(context.Background(), f.conn, params)
	if err != nil {
		t.Fatal(err)
	}
	totals = timeline.TotalsForPeriod(filtered)
	if totals.Expense.String() != "50" || !totals.Income.IsZero() {
		t.Fatalf("bank filter leaked investment income/cost: %+v", totals)
	}
}

func TestInvestmentContributionsAndWithdrawalsAreNotIncomeOrExpense(t *testing.T) {
	f := newFixture(t)
	bankID := f.addAccount("0")
	contribution := f.addTransaction(txn{AccountID: bankID, Amount: "-1000", OccurredAt: date(t, "2026-09-05")})
	redemption := f.addTransaction(txn{AccountID: bankID, Amount: "600", OccurredAt: date(t, "2026-09-20")})
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('manual','Manual','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('deposit','manual','deposit','2026-09-05','600',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('withdrawal','manual','withdrawal','2026-09-20','600',?,?)`, now, now)
	f.exec(`INSERT INTO investment_reconciliations (id,operation_id,financial_transaction_id,amount,created_at) VALUES ('in','deposit',?,'600',?)`, contribution, now)
	f.exec(`INSERT INTO investment_reconciliations (id,operation_id,financial_transaction_id,amount,created_at) VALUES ('out','withdrawal',?,'600',?)`, redemption, now)

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-09-01"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-09-30"),
	})
	if err != nil {
		t.Fatal(err)
	}
	// Only the 400 of the bank debit that no contribution explains is spending;
	// the fully reconciled withdrawal is not income.
	totals := timeline.TotalsForPeriod(series)
	if totals.Expense.String() != "400" || !totals.Income.IsZero() {
		t.Fatalf("totals = %+v", totals)
	}
}

func TestUnrealizedValuationDoesNotEnterTimelineTotals(t *testing.T) {
	f := newFixture(t)
	f.addAccount("0")
	now := db.FormatTime(time.Now())
	f.exec(`INSERT INTO investment_accounts (id,name,kind,currency_code,created_at,updated_at) VALUES ('manual','Manual','manual','BRL',?,?)`, now, now)
	f.exec(`INSERT INTO investment_assets (id,canonical_key,name,asset_type,created_at,updated_at) VALUES ('asset','ticker:ACOES','Ações','EQUITY',?,?)`, now, now)
	f.exec(`INSERT INTO investment_positions (id,account_id,asset_id,created_at,updated_at) VALUES ('position','manual','asset',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,position_id,kind,occurred_on,amount,quantity,unit_price,created_at,updated_at) VALUES ('buy','manual','position','buy','2026-09-02','600','6','100',?,?)`, now, now)
	f.exec(`INSERT INTO investment_operations (id,account_id,position_id,kind,occurred_on,amount,created_at,updated_at) VALUES ('valuation','manual','position','valuation','2026-09-10','900',?,?)`, now, now)

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-09-01"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-09-30"),
	})
	if err != nil {
		t.Fatal(err)
	}
	totals := timeline.TotalsForPeriod(series)
	if !totals.Income.IsZero() || !totals.Expense.IsZero() {
		t.Fatalf("valuation reported as income/expense: %+v", totals)
	}
}
