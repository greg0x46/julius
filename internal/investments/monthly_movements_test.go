package investments_test

import (
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) withdraw(amount string, on time.Time) investments.Operation {
	f.t.Helper()
	return f.create(investments.OperationInput{AccountID: f.custodyID, Kind: investments.OperationWithdrawal, OccurredOn: on, Amount: dec(amount)})
}

func (f *ledgerFixture) monthlyMovements(accountID *string) map[string]investments.MonthlyMovement {
	f.t.Helper()
	list, err := investments.MonthlyMovements(f.ctx, f.conn, accountID)
	if err != nil {
		f.t.Fatalf("MonthlyMovements: %v", err)
	}
	byMonth := map[string]investments.MonthlyMovement{}
	for i, movement := range list {
		if i > 0 && list[i-1].Month >= movement.Month {
			f.t.Fatalf("months out of order: %+v", list)
		}
		byMonth[movement.Month] = movement
	}
	return byMonth
}

func TestMonthlyMovementsAreGrossAndIndependentOfReconciliation(t *testing.T) {
	f := newLedgerFixture(t)
	september := f.deposit("1000", day(2))
	f.deposit("500", day(20))
	f.withdraw("300", day(25))
	f.deposit("200", time.Date(2026, 10, 3, 0, 0, 0, 0, time.UTC))
	f.withdraw("1400", time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC))

	assertMonths := func(label string) {
		t.Helper()
		months := f.monthlyMovements(nil)
		if len(months) != 2 {
			t.Fatalf("%s: months = %+v", label, months)
		}
		sep, oct := months["2026-09"], months["2026-10"]
		if sep.Contributions.String() != "1500" || sep.Withdrawals.String() != "300" ||
			!sep.Income.IsZero() || !sep.Fees.IsZero() || !sep.Taxes.IsZero() {
			t.Fatalf("%s: september = %+v", label, sep)
		}
		if oct.Contributions.String() != "200" || oct.Withdrawals.String() != "1400" {
			t.Fatalf("%s: october = %+v", label, oct)
		}
	}
	assertMonths("unlinked")

	bank := f.addBankTransaction("-1000", day(2))
	f.link(september.ID, bank, "600")
	assertMonths("partially linked")

	transfers, err := investments.ReconciledTransactionAmounts(f.ctx, f.conn)
	if err != nil {
		t.Fatal(err)
	}
	if transfers[bank].String() != "600" {
		t.Fatalf("transfer portion = %+v", transfers)
	}
	if got := f.reportedByID()[september.ID]; got != "400" {
		t.Fatalf("partially linked deposit reported %q, want 400", got)
	}

	other := "missing-account"
	if got := f.monthlyMovements(&other); len(got) != 0 {
		t.Fatalf("account filter leaked: %+v", got)
	}
	if got := f.monthlyMovements(&f.custodyID); len(got) != 2 {
		t.Fatalf("account filter dropped months: %+v", got)
	}
}

func TestMonthlyMovementsExcludeImportedReconciliationPivots(t *testing.T) {
	f := newLedgerFixture(t)
	movement := f.addInvestmentMovement()
	accountID := "integrated:" + f.sourceID
	if _, err := investments.ListAccounts(f.ctx, f.conn); err != nil {
		t.Fatal(err)
	}
	if got := f.monthlyMovements(nil); len(got) != 0 {
		t.Fatalf("unlinked imported movement reported: %+v", got)
	}
	f.create(investments.OperationInput{
		AccountID: accountID, Kind: investments.OperationDeposit, OccurredOn: day(1), Amount: dec("200"),
	})
	assertLocalOnly := func(label string) {
		t.Helper()
		for _, filter := range []*string{nil, &accountID} {
			months := f.monthlyMovements(filter)
			month := months["2026-09"]
			if len(months) != 1 || month.Contributions.String() != "200" ||
				!month.Withdrawals.IsZero() || !month.Income.IsZero() || !month.Fees.IsZero() || !month.Taxes.IsZero() {
				t.Fatalf("%s: monthly movements = %+v", label, months)
			}
		}
	}
	assertLocalOnly("unlinked")

	first := f.addBankTransaction("-600", day(1))
	second := f.addBankTransaction("-400", day(1))
	a, err := f.linkProvider(first, movement, "600")
	if err != nil {
		t.Fatal(err)
	}
	assertLocalOnly("partially linked")
	b, err := f.linkProvider(second, movement, "400")
	if err != nil {
		t.Fatal(err)
	}
	assertLocalOnly("fully linked")
	for _, id := range []string{a.ID, b.ID} {
		if err := investments.DeleteReconciliation(f.ctx, f.conn, id); err != nil {
			t.Fatal(err)
		}
		assertLocalOnly("unlinked parcel")
	}
	if got := f.syncedOperationCount(); got != 0 {
		t.Fatalf("orphan pivot survived: %d", got)
	}
}
