package scenarios_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

// TestSignedAmountFollowsPlanDirection pins what lets the timeline project a
// plan installment onto a cash-flow series without consulting the backing
// Payable: the direction is derivable from the scenario's own Kind.
func TestSignedAmountFollowsPlanDirection(t *testing.T) {
	amount := dec(t, "250.00")
	cases := []struct {
		kind scenarios.Kind
		want string
	}{
		{scenarios.KindDebtPlan, "-250"},
		{scenarios.KindReceivablePlan, "250"},
		{scenarios.KindStandalone, "250"},
	}
	for _, c := range cases {
		if got := scenarios.SignedAmount(c.kind, amount); got.String() != c.want {
			t.Errorf("SignedAmount(%s) = %s, want %s", c.kind, got, c.want)
		}
	}
}

// TestSignedAmountNegatesRatherThanForcingSign documents preserved behaviour,
// not desired behaviour: a debt-plan installment authored with a negative
// amount reads back as an inflow. Recorded as open note 5 in
// docs/development/architecture.md — deciding it either way moves real totals, so
// this test exists to make the change visible if anyone does decide.
func TestSignedAmountNegatesRatherThanForcingSign(t *testing.T) {
	got := scenarios.SignedAmount(scenarios.KindDebtPlan, dec(t, "-100.00"))
	if got.String() != "100" {
		t.Errorf("SignedAmount = %s, want 100 (negation, not sign forcing)", got)
	}
}

// TestSummarizeAccumulatedDeviationCountsOnlyDueInstallments pins the
// "ritmo necessário vs. ritmo real" framing: only what should already have
// happened says whether a plan is on track, so an installment that is not due
// yet contributes nothing to the deviation.
func TestSummarizeAccumulatedDeviationCountsOnlyDueInstallments(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	d := newDebt(t, conn)

	s, err := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Plano", &d.ID)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	today := date(t, "2026-06-15")
	// Due and untouched: fully behind. Due and paid: on track. Not yet due:
	// invisible to the deviation however large.
	behind, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Abril", dec(t, "300.00"), date(t, "2026-04-10"), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	paid, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Maio", dec(t, "300.00"), date(t, "2026-05-10"), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	if _, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Julho", dec(t, "300.00"), date(t, "2026-07-10"), nil); err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	link := linkFixture(t, conn, d.ID, "-300.00")
	if _, err := scenarios.CreateRealization(ctx, conn, paid.ID, &link.ID, dec(t, "300.00")); err != nil {
		t.Fatalf("CreateRealization: %v", err)
	}

	summary, err := scenarios.Summarize(ctx, conn, s.ID, today)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !summary.AccumulatedDeviation.Equal(dec(t, "300.00")) {
		t.Errorf("AccumulatedDeviation = %s, want 300.00 (April's unpaid installment only)", summary.AccumulatedDeviation)
	}
	if len(summary.Transactions) != 3 {
		t.Fatalf("len(Transactions) = %d, want 3", len(summary.Transactions))
	}

	byID := map[string]scenarios.TransactionSummary{}
	for _, ts := range summary.Transactions {
		byID[ts.Transaction.ID] = ts
	}
	if got := byID[behind.ID].Status; got != scenarios.StatusAtrasada {
		t.Errorf("April status = %s, want atrasada", got)
	}
	if got := byID[paid.ID].Status; got != scenarios.StatusPaga {
		t.Errorf("May status = %s, want paga", got)
	}
	if !byID[paid.ID].RealizedTotal.Equal(dec(t, "300.00")) {
		t.Errorf("May RealizedTotal = %s, want 300.00", byID[paid.ID].RealizedTotal)
	}
}

// TestSummarizeDeviationIsNegativeWhenAhead pins the sign convention the HTTP
// layer renders: positive means behind, negative means ahead.
func TestSummarizeDeviationIsNegativeWhenAhead(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	d := newDebt(t, conn)

	s, _ := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Plano", &d.ID)
	st, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Abril", dec(t, "300.00"), date(t, "2026-04-10"), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	link := linkFixture(t, conn, d.ID, "-500.00")
	if _, err := scenarios.CreateRealization(ctx, conn, st.ID, &link.ID, dec(t, "500.00")); err != nil {
		t.Fatalf("CreateRealization: %v", err)
	}

	summary, err := scenarios.Summarize(ctx, conn, s.ID, date(t, "2026-06-15"))
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !summary.AccumulatedDeviation.Equal(dec(t, "-200.00")) {
		t.Errorf("AccumulatedDeviation = %s, want -200.00 (ahead of plan)", summary.AccumulatedDeviation)
	}
	if summary.Transactions[0].Status != scenarios.StatusPagaAMais {
		t.Errorf("status = %s, want paga_a_mais", summary.Transactions[0].Status)
	}
}

// TestSummarizeFreshMatchesSummarizeTransaction pins that the no-query
// shortcut for a just-created installment agrees with the querying path, so
// the generate/readjust handlers cannot drift from the detail view.
func TestSummarizeFreshMatchesSummarizeTransaction(t *testing.T) {
	conn := newTestDB(t)
	ctx := context.Background()
	d := newDebt(t, conn)

	s, _ := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Plano", &d.ID)
	st, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Abril", dec(t, "300.00"), date(t, "2026-04-10"), nil)
	if err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}
	today := date(t, "2026-03-01")

	fresh := scenarios.SummarizeFresh(st, today)
	queried, err := scenarios.SummarizeTransaction(ctx, conn, st, today)
	if err != nil {
		t.Fatalf("SummarizeTransaction: %v", err)
	}
	if fresh.Status != queried.Status || !fresh.RealizedTotal.Equal(queried.RealizedTotal) {
		t.Errorf("fresh = %+v, queried = %+v — the shortcut must agree with the query", fresh, queried)
	}
	if fresh.Status != scenarios.StatusProjetada {
		t.Errorf("Status = %s, want projetada", fresh.Status)
	}
}
