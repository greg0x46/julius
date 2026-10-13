package timeline_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/timeline"
)

func TestBuildSeriesStartingBalanceSumsAccounts(t *testing.T) {
	f := newFixture(t)
	f.addAccount("1000.00")
	f.addAccount("500.00")

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-01-01"), To: date(t, "2026-12-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if series.StartingBalance.String() != "1500" {
		t.Errorf("StartingBalance = %s, want 1500", series.StartingBalance.String())
	}
}

func TestBuildSeriesStartingBalanceFilteredByAccount(t *testing.T) {
	f := newFixture(t)
	a := f.addAccount("1000.00")
	f.addAccount("500.00")

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-01-01"), To: date(t, "2026-12-31"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{a},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if series.StartingBalance.String() != "1000" {
		t.Errorf("StartingBalance = %s, want 1000", series.StartingBalance.String())
	}
}

func TestBuildSeriesStartingBalanceExcludesCreditCards(t *testing.T) {
	f := newFixture(t)
	f.addAccount("1000.00")
	creditCard := f.addAccount("500.00")
	f.exec(`UPDATE financial_accounts SET account_type = 'CREDIT' WHERE id = ?`, creditCard)

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-01-01"), To: date(t, "2026-12-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if series.StartingBalance.String() != "1000" {
		t.Errorf("StartingBalance = %s, want 1000 (credit card balance excluded)", series.StartingBalance.String())
	}
}

func TestBuildSeriesCreditCardEntryProjectsToBillDueDate(t *testing.T) {
	f := newFixture(t)
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-1", "2026-09-10")
	cat := categorySupermercado
	metadata := `{"billId":"bill-1"}`
	f.addTransaction(txn{
		AccountID: card, Amount: "-150.00", OccurredAt: date(t, "2026-08-20"),
		CategoryID: &cat, CreditCardMetadata: &metadata,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-01-01"), To: date(t, "2026-12-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(series.Entries))
	}
	entry := series.Entries[0]
	if !entry.Date.Equal(date(t, "2026-09-10")) {
		t.Errorf("Date = %s, want 2026-09-10 (the bill's due date, not occurred_at)", entry.Date)
	}
	if entry.Tier != timeline.TierConfirmado {
		t.Errorf("Tier = %s, want Confirmado", entry.Tier)
	}
	if entry.Amount.String() != "-150" {
		t.Errorf("Amount = %s, want -150", entry.Amount.String())
	}
}

func TestBuildSeriesCreditCardEntryWithoutBillFallsBackToOccurredAt(t *testing.T) {
	f := newFixture(t)
	card := f.addCreditCardAccount("0")
	cat := categorySupermercado
	f.addTransaction(txn{
		AccountID: card, Amount: "-40.00", OccurredAt: date(t, "2026-08-20"), CategoryID: &cat,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-01-01"), To: date(t, "2026-12-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1", len(series.Entries))
	}
	if !series.Entries[0].Date.Equal(date(t, "2026-08-20")) {
		t.Errorf("Date = %s, want 2026-08-20 (no bill history to project from)", series.Entries[0].Date)
	}
}

// A card purchase re-dated to a bill that already fell due lands outside
// the window, where no DayPoint can carry it. Left in the series it would
// not merely be an unrenderable row: buildPoints subtracts every entry
// dated on or before the reference date when anchoring the balance, so this
// one would be subtracted and never added back, lifting every point — and
// the "menor saldo" the dashboard reports — above the real balance.
func TestBuildSeriesEntryDatedBeforeWindowDoesNotShiftBalance(t *testing.T) {
	f := newFixture(t)
	f.addAccount("1000.00")
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-1", "2026-08-10")
	cat := categorySupermercado
	metadata := `{"billId":"bill-1"}`
	f.addTransaction(txn{
		AccountID: card, Amount: "-800.00", OccurredAt: date(t, "2026-08-05"),
		CategoryID: &cat, CreditCardMetadata: &metadata,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-25"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-08-25"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 0 {
		t.Errorf("Entries = %d, want 0 (the bill fell due before the window)", len(series.Entries))
	}
	if !series.Points[0].Balance.Equal(series.StartingBalance) {
		t.Errorf("first point balance = %s, want %s (StartingBalance)", series.Points[0].Balance, series.StartingBalance)
	}
	if !series.LowestBalance.Balance.Equal(series.StartingBalance) {
		t.Errorf("LowestBalance = %s, want %s (nothing in the window moves it)",
			series.LowestBalance.Balance, series.StartingBalance)
	}
}

// The mirror image: a purchase made before the window is paid inside it, so
// its bill has to reach the projection even though occurred_at never enters
// the queried range.
func TestBuildSeriesIncludesBillFromPurchaseBeforeWindow(t *testing.T) {
	f := newFixture(t)
	f.addAccount("1000.00")
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-1", "2026-09-10")
	cat := categorySupermercado
	metadata := `{"billId":"bill-1"}`
	f.addTransaction(txn{
		AccountID: card, Amount: "-800.00", OccurredAt: date(t, "2026-08-05"),
		CategoryID: &cat, CreditCardMetadata: &metadata,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-25"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-08-25"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 1 {
		t.Fatalf("Entries = %d, want 1 (the bill due inside the window)", len(series.Entries))
	}
	if !series.Entries[0].Date.Equal(date(t, "2026-09-10")) {
		t.Errorf("Date = %s, want 2026-09-10", series.Entries[0].Date)
	}
	if series.LowestBalance.Balance.String() != "200" {
		t.Errorf("LowestBalance = %s, want 200 (1000 starting balance minus the bill)", series.LowestBalance.Balance)
	}
}

func TestBuildSeriesRealEntriesOnlyIncludesEligibleTransactions(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00")
	cat := categorySupermercado
	f.addTransaction(txn{AccountID: account, Amount: "-100.00", OccurredAt: date(t, "2026-08-01"), CategoryID: &cat})
	sal := categorySalario
	f.addTransaction(txn{AccountID: account, Amount: "2000.00", OccurredAt: date(t, "2026-08-05"), CategoryID: &sal})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 2 {
		t.Fatalf("Entries = %+v, want 2", series.Entries)
	}
	for _, e := range series.Entries {
		if e.Tier != timeline.TierRealizado || e.Source != timeline.SourceReal {
			t.Errorf("entry = %+v, want TierRealizado/SourceReal", e)
		}
	}
}

func TestBuildSeriesEntriesFilteredByCategory(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00")
	cat := categorySupermercado
	f.addTransaction(txn{AccountID: account, Amount: "-100.00", OccurredAt: date(t, "2026-08-01"), CategoryID: &cat})
	sal := categorySalario
	f.addTransaction(txn{AccountID: account, Amount: "2000.00", OccurredAt: date(t, "2026-08-05"), CategoryID: &sal})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-31"), ReferenceDate: date(t, "2026-08-15"),
		CategoryIDs: []string{categorySupermercado},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if len(series.Entries) != 1 || series.Entries[0].CategoryName != "Supermercado" {
		t.Errorf("Entries = %+v, want only the Supermercado entry", series.Entries)
	}
}

// TestBuildSeriesBalanceAnchoredAtReference verifies the historical
// reconstruction described in build.go's buildPoints comment: with only
// past entries, every point's balance equals StartingBalance minus
// whatever hadn't happened yet at that point in time, so the balance at
// the most recent entry (and at every later day, M3 has no future
// sources) reads as exactly StartingBalance.
func TestBuildSeriesBalanceAnchoredAtReference(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("1000.00")
	f.addTransaction(txn{AccountID: account, Amount: "-100.00", OccurredAt: date(t, "2026-08-01")})
	f.addTransaction(txn{AccountID: account, Amount: "300.00", OccurredAt: date(t, "2026-08-10")})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-15"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	last := series.Points[len(series.Points)-1]
	if !last.Balance.Equal(series.StartingBalance) {
		t.Errorf("last point balance = %s, want %s (StartingBalance)", last.Balance, series.StartingBalance)
	}
	first := series.Points[0]
	// A DayPoint's balance is "as of the end of that day": on 2026-08-01,
	// only that day's own -100 entry has applied yet (the +300 on 08-10
	// hasn't happened), so its balance is StartingBalance minus what
	// hadn't happened yet as of end-of-day 08-01 (+300): 1000 - 300 = 700.
	if first.Balance.String() != "700" {
		t.Errorf("first point balance = %s, want 700", first.Balance.String())
	}
	if series.LowestBalance.Balance.String() != series.StartingBalance.String() {
		t.Errorf("LowestBalance = %s, want %s (M3 has no future sources, so it's flat at reference)",
			series.LowestBalance.Balance, series.StartingBalance)
	}
	if series.FirstNegative != nil {
		t.Errorf("FirstNegative = %v, want nil", series.FirstNegative)
	}
}

func TestBuildSeriesFirstNegativeWhenStartingBalanceIsAlreadyNegative(t *testing.T) {
	f := newFixture(t)
	f.addAccount("-50.00")

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-31"), ReferenceDate: date(t, "2026-08-15"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if series.FirstNegative == nil || !series.FirstNegative.Equal(date(t, "2026-08-15")) {
		t.Errorf("FirstNegative = %v, want 2026-08-15", series.FirstNegative)
	}
}

// TestBuildSeriesKeepsTransferCategorizedCashMovement is the regression test
// for the transfer rule's blast radius. A transfer category keeps a
// transaction out of income/expense totals, but the money still left the
// account — and buildPoints anchors the curve by subtracting past entries
// from today's real balance, so dropping the entry makes every day *before*
// it read short by the transfer's full amount. On an account-filtered series
// the counterpart leg is not even in the set, so nothing cancels the error
// out.
func TestBuildSeriesKeepsTransferCategorizedCashMovement(t *testing.T) {
	f := newFixture(t)
	origin := f.addAccount("700.00")
	transfer := categoryTransferencia
	f.addTransaction(txn{
		AccountID: origin, Amount: "-300.00", OccurredAt: date(t, "2026-08-10"),
		CategoryID: &transfer,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-20"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{origin},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}

	// 700 today, so 1000 before the 300 left on the 10th.
	if got := series.Points[0].Balance.String(); got != "1000" {
		t.Errorf("balance before the transfer = %s, want 1000 — the money really did leave the account", got)
	}
	last := series.Points[len(series.Points)-1]
	if got := last.Balance.String(); got != "700" {
		t.Errorf("balance after the transfer = %s, want 700", got)
	}
}

// TestBuildSeriesTransferMovesCashButIsNeitherIncomeNorExpense pins down
// the two faces of a transfer between the user's own accounts. The money did
// leave this account, so the balance curve must fall by it — that's the
// previous test. But nothing was earned or spent: the same reais are sitting
// in another account, and counting them as an expense would make every
// month with a transfer look poorer than it was. The series carries both
// readings side by side: buildPoints walks Entry.Amount (cash), while
// TotalsForPeriod walks Entry.ReportableAmount (income/expense), which
// transactions.toItem zeroes for anything TotalsEligibility excludes.
// Before ReportableAmount existed the totals read Amount and the transfer
// counted at full value.
func TestBuildSeriesTransferMovesCashButIsNeitherIncomeNorExpense(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("600.00")
	supermercado := categorySupermercado
	f.addTransaction(txn{
		AccountID: account, Amount: "-100.00", OccurredAt: date(t, "2026-08-05"),
		CategoryID: &supermercado,
	})
	transfer := categoryTransferencia
	f.addTransaction(txn{
		AccountID: account, Amount: "-300.00", OccurredAt: date(t, "2026-08-10"),
		CategoryID: &transfer,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-20"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{account},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}

	// Cash: 600 today, so 1000 before both outflows — the transfer is
	// reversed out of the anchor exactly like the expense is.
	if got := series.Points[0].Balance.String(); got != "1000" {
		t.Errorf("balance before both movements = %s, want 1000 (100 expense + 300 transfer both left the account)", got)
	}
	last := series.Points[len(series.Points)-1]
	if got := last.Balance.String(); got != "600" {
		t.Errorf("balance after both movements = %s, want 600", got)
	}

	// Income/expense: only the supermarket run counts.
	totals := timeline.TotalsForPeriod(series)
	if got := totals.Expense.String(); got != "100" {
		t.Errorf("Expense = %s, want 100 (the transfer is not an expense)", got)
	}
	if got := totals.Income.String(); got != "0" {
		t.Errorf("Income = %s, want 0", got)
	}
}

// ...while an ignored transaction stays out: "ignored" says the row should
// not be here at all (a reversal, a duplicate), so it never moved cash and
// the anchor must not reverse it.
func TestBuildSeriesStillDropsIgnoredTransactions(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("700.00")
	id := f.addTransaction(txn{
		AccountID: account, Amount: "-300.00", OccurredAt: date(t, "2026-08-10"),
	})
	f.exec(`INSERT INTO transaction_inclusion_decisions (transaction_id, state, revision, changed_at, origin)
		VALUES (?, 'ignored', 1, ?, 'manual')`, id, "2026-08-11T00:00:00.000000000Z")

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-20"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{account},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if got := series.Points[0].Balance.String(); got != "700" {
		t.Errorf("balance = %s, want a flat 700 — an ignored transaction never moved cash", got)
	}
}

// TestBuildSeriesDropsAnUnsettledTransfer is the other edge of the transfer
// rule. MovesCash treats "transfer_category" as "the money really did leave
// the account", so that reason must never be reached by a transaction the
// provider has not settled — money.Eligibility checks the transfer category
// last for exactly this reason. Reversing an unsettled transfer out of the
// anchor made every day before it read high by its full amount.
func TestBuildSeriesDropsAnUnsettledTransfer(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("700.00")
	transfer := categoryTransferencia
	id := f.addTransaction(txn{
		AccountID: account, Amount: "-300.00", OccurredAt: date(t, "2026-08-10"),
		CategoryID: &transfer,
	})
	f.exec(`UPDATE financial_transactions SET provider_status = 'PROCESSING' WHERE id = ?`, id)

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-20"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{account},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if got := series.Points[0].Balance.String(); got != "700" {
		t.Errorf("balance = %s, want a flat 700 — an unsettled transfer has not moved cash yet", got)
	}
}

// ...and the same for a transfer whose movement type cannot be classified.
// realEntries takes an entry's direction from the classification, not from
// the value's sign, so an unclassified transfer used to be added as a credit:
// the balance came out wrong by twice the amount rather than merely short.
func TestBuildSeriesDropsAnUnclassifiedTransfer(t *testing.T) {
	f := newFixture(t)
	account := f.addAccount("700.00")
	transfer := categoryTransferencia
	id := f.addTransaction(txn{
		AccountID: account, Amount: "-300.00", OccurredAt: date(t, "2026-08-10"),
		CategoryID: &transfer,
	})
	f.exec(`UPDATE financial_transactions SET movement_type = 'SOMETHING_NEW' WHERE id = ?`, id)

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-08-01"), To: date(t, "2026-08-20"), ReferenceDate: date(t, "2026-08-15"),
		AccountIDs: []string{account},
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if got := series.Points[0].Balance.String(); got != "700" {
		t.Errorf("balance = %s, want a flat 700 — an unclassified movement has no direction to apply", got)
	}
}
