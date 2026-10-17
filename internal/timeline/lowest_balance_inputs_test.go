package timeline_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/timeline"
)

// TestBuildSeriesFromTodayCombinesEverySource pins what Home's daily allowance
// reads: with From = ReferenceDate = today, one point per day starting today,
// over recurring income and expenses, payable-plan installments and card bills
// re-dated to their due date, all together.
func TestBuildSeriesFromTodayCombinesEverySource(t *testing.T) {
	f := newFixture(t)
	f.addAccount("3000.00")
	ctx := context.Background()

	if _, err := recurrences.Create(ctx, f.conn, recurrences.Write{
		Name: "Aluguel", Kind: recurrences.KindExpense, Amount: decT(t, "1500.00"),
		CategoryID: categorySupermercado,
		Cadence:    recurrences.CadenceMonthly, DayOfMonth: 12, StartDate: date(t, "2026-01-01"), IsActive: true,
	}); err != nil {
		t.Fatalf("recurrences.Create expense: %v", err)
	}
	if _, err := recurrences.Create(ctx, f.conn, recurrences.Write{
		Name: "Salário", Kind: recurrences.KindIncome, Amount: decT(t, "2000.00"),
		CategoryID: categorySalario,
		Cadence:    recurrences.CadenceMonthly, DayOfMonth: 20, StartDate: date(t, "2026-01-01"), IsActive: true,
	}); err != nil {
		t.Fatalf("recurrences.Create income: %v", err)
	}

	debt, err := payables.Create(ctx, f.conn, payables.KindDebt, "Financiamento", decT(t, "300.00"), decT(t, "300.00"))
	if err != nil {
		t.Fatalf("payables.Create: %v", err)
	}
	plan, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindDebtPlan, "Plano", &debt.ID)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if _, err := scenarios.CreateScenarioTransaction(ctx, f.conn, plan.ID, "Parcela", decT(t, "300.00"), date(t, "2026-10-15"), nil); err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}

	// Bought before today, paid on the bill's due date inside the window.
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-oct", "2026-10-18")
	cat := categorySupermercado
	metadata := `{"billId":"bill-oct"}`
	f.addTransaction(txn{
		AccountID: card, Amount: "-400.00", OccurredAt: date(t, "2026-10-05"),
		CategoryID: &cat, CreditCardMetadata: &metadata,
	})

	series, err := timeline.BuildSeries(ctx, f.conn, timeline.BuildParams{
		From: date(t, "2026-10-10"), To: date(t, "2026-10-31"), ReferenceDate: date(t, "2026-10-10"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}

	sources := map[timeline.SourceKind]int{}
	for _, e := range series.Entries {
		sources[e.Source]++
	}
	if sources[timeline.SourceRecurring] != 2 || sources[timeline.SourcePayablePlan] != 1 || sources[timeline.SourceReal] != 1 {
		t.Fatalf("entries by source = %v, want 2 recurring, 1 payable plan, 1 real (the card bill)", sources)
	}

	if len(series.Points) != 22 || !series.Points[0].Date.Equal(date(t, "2026-10-10")) {
		t.Fatalf("points = %d starting %s, want 22 starting 2026-10-10",
			len(series.Points), series.Points[0].Date.Format("2006-01-02"))
	}

	// 3000 on the 10th, -1500 rent on the 12th (1500), -300 installment on
	// the 15th (1200), -400 card bill on the 18th (800), +2000 salary on the
	// 20th (2800). Without the bill the low would be 1200 on the 15th.
	if !series.LowestBalance.Date.Equal(date(t, "2026-10-18")) || series.LowestBalance.Balance.String() != "800" {
		t.Errorf("LowestBalance = %s on %s, want 800 on 2026-10-18",
			series.LowestBalance.Balance, series.LowestBalance.Date.Format("2006-01-02"))
	}
	if final := series.Points[len(series.Points)-1]; final.Balance.String() != "2800" {
		t.Errorf("final balance = %s, want 2800", final.Balance)
	}
}

// TestBuildSeriesDeductsWhatFallsDueOnTheReferenceDay pins day 0 of the same
// contract. The anchor is cash on hand right now, so a real transaction dated
// today is already in it, but a recurrence, plan installment or card bill due
// today and not yet reconciled is not: today's point must be cash minus
// those, or the low point from today misses what is owed today.
func TestBuildSeriesDeductsWhatFallsDueOnTheReferenceDay(t *testing.T) {
	f := newFixture(t)
	// Cash on hand now, after this morning's real 50.00 expense below.
	bank := f.addAccount("3000.00")
	ctx := context.Background()
	cat := categorySupermercado

	f.addTransaction(txn{AccountID: bank, Amount: "-50.00", OccurredAt: date(t, "2026-10-10"), CategoryID: &cat})
	if _, err := recurrences.Create(ctx, f.conn, recurrences.Write{
		Name: "Aluguel", Kind: recurrences.KindExpense, Amount: decT(t, "1500.00"),
		CategoryID: categorySupermercado,
		Cadence:    recurrences.CadenceMonthly, DayOfMonth: 10, StartDate: date(t, "2026-01-01"), IsActive: true,
	}); err != nil {
		t.Fatalf("recurrences.Create: %v", err)
	}
	debt, err := payables.Create(ctx, f.conn, payables.KindDebt, "Financiamento", decT(t, "300.00"), decT(t, "300.00"))
	if err != nil {
		t.Fatalf("payables.Create: %v", err)
	}
	plan, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindDebtPlan, "Plano", &debt.ID)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if _, err := scenarios.CreateScenarioTransaction(ctx, f.conn, plan.ID, "Parcela", decT(t, "300.00"), date(t, "2026-10-10"), nil); err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-oct", "2026-10-10")
	metadata := `{"billId":"bill-oct"}`
	f.addTransaction(txn{
		AccountID: card, Amount: "-400.00", OccurredAt: date(t, "2026-10-02"),
		CategoryID: &cat, CreditCardMetadata: &metadata,
	})

	for _, from := range []string{"2026-10-10", "2026-10-05"} {
		series, err := timeline.BuildSeries(ctx, f.conn, timeline.BuildParams{
			From: date(t, from), To: date(t, "2026-10-12"), ReferenceDate: date(t, "2026-10-10"),
		})
		if err != nil {
			t.Fatalf("BuildSeries from %s: %v", from, err)
		}
		// 3000 in cash - 1500 rent - 300 installment - 400 card bill; the
		// 50.00 expense is already in the 3000.
		if got := balanceOn(t, series, "2026-10-10"); got != "800" {
			t.Errorf("from %s: balance at the end of the reference day = %s, want 800", from, got)
		}
		if series.StartingBalance.String() != "3000" {
			t.Errorf("from %s: StartingBalance = %s, want 3000 (cash on hand is untouched)", from, series.StartingBalance)
		}
		if !series.LowestBalance.Date.Equal(date(t, "2026-10-10")) || series.LowestBalance.Balance.String() != "800" {
			t.Errorf("from %s: LowestBalance = %s on %s, want 800 on 2026-10-10",
				from, series.LowestBalance.Balance, series.LowestBalance.Date.Format("2006-01-02"))
		}
	}
}

// TestBuildSeriesPaidCardBillDueTodayIsNotChargedTwice is the paid half of
// the case above: the bank leg of the payment is real cash already, and the
// card leg re-dated to the same due date cancels the purchase, so a bill
// settled today leaves today's point at cash on hand.
func TestBuildSeriesPaidCardBillDueTodayIsNotChargedTwice(t *testing.T) {
	f := newFixture(t)
	bank := f.addAccount("850.00") // 1000 before this morning's 150 bill payment
	card := f.addCreditCardAccount("0")
	f.addBill(card, "bill-oct", "2026-10-10")
	metadata := `{"billId":"bill-oct"}`
	cardPaymentCategory := "dd10c680-fb35-4457-8595-4e51c8d279a7"
	f.addTransaction(txn{AccountID: card, Amount: "-150.00", OccurredAt: date(t, "2026-09-20"), CreditCardMetadata: &metadata})
	f.addTransaction(txn{AccountID: bank, Amount: "-150.00", OccurredAt: date(t, "2026-10-10"), CategoryID: &cardPaymentCategory})
	f.addTransaction(txn{
		AccountID: card, Amount: "150.00", OccurredAt: date(t, "2026-10-10"),
		CreditCardMetadata: &metadata, CategoryID: &cardPaymentCategory,
	})

	series, err := timeline.BuildSeries(context.Background(), f.conn, timeline.BuildParams{
		From: date(t, "2026-10-10"), To: date(t, "2026-10-12"), ReferenceDate: date(t, "2026-10-10"),
	})
	if err != nil {
		t.Fatalf("BuildSeries: %v", err)
	}
	if got := balanceOn(t, series, "2026-10-10"); got != "850" {
		t.Errorf("balance on the due day = %s, want 850 (payment already in cash, nothing owed twice)", got)
	}
}
