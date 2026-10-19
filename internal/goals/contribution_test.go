package goals

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
)

func TestMonthlyContributionUsesMonthOpeningNotTodaysValue(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	clock := utc(2026, 10, 19)
	old := now
	now = func() time.Time { return clock }
	defer func() { now = old }()
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Custody"})
	if err != nil {
		t.Fatal(err)
	}
	targetDate := utc(2026, 12, 31)
	// Creation halfway through the month uses the whole current-month plan.
	goal, err := Create(ctx, conn, Input{Name: "Reserve", Metric: MetricInvestedValue, TargetAmount: d("6000"), TargetDate: &targetDate})
	if err != nil {
		t.Fatal(err)
	}
	check := func(target, net, remaining string) {
		t.Helper()
		g, err := Get(ctx, conn, goal.ID)
		if err != nil {
			t.Fatal(err)
		}
		m := g.MonthlyContribution
		if m == nil || m.Target == nil || m.Target.String() != target || m.NetContributed.String() != net || m.Remaining.String() != remaining {
			t.Fatalf("monthly = %+v, want %s/%s/%s", m, target, net, remaining)
		}
	}
	operation := func(kind investments.OperationKind, amount string, day time.Time) {
		t.Helper()
		_, err := investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: account.ID, Kind: kind, Amount: d(amount), OccurredOn: day})
		if err != nil {
			t.Fatal(err)
		}
	}
	check("2000", "0", "2000")
	operation(investments.OperationDeposit, "650", clock)
	check("2000", "650", "1350")
	operation(investments.OperationDeposit, "1350", clock)
	check("2000", "2000", "0")
	operation(investments.OperationDeposit, "100", clock)
	check("2000", "2100", "0")
	operation(investments.OperationWithdrawal, "300", clock)
	check("2000", "1800", "200")
	// Income and fees change current value but never the opening plan or principal.
	operation(investments.OperationIncome, "100", clock)
	operation(investments.OperationFee, "20", clock)
	check("2000", "1800", "200")
	_, err = Update(ctx, conn, goal.ID, Input{Name: "Edited reserve", Metric: MetricInvestedValue, TargetAmount: d("9000"), TargetDate: &targetDate})
	if err != nil {
		t.Fatal(err)
	}
	check("3000", "1800", "1200")
	// Next month starts afresh from the previous month's closing cash.
	clock = utc(2026, 11, 1)
	check("3560", "0", "3560")
}

func TestNextContributionGoalSelection(t *testing.T) {
	future := utc(2026, 12, 31)
	later := utc(2027, 1, 1)
	p := &Projection{Status: StatusBehind}
	list := []Goal{
		{ID: "reached", TargetDate: &future, TargetAmount: d("1"), CurrentValue: d("1"), Projection: p},
		{ID: "undated", TargetAmount: d("1")},
		{ID: "later", TargetDate: &later, TargetAmount: d("1"), Projection: p},
		{ID: "z", TargetDate: &future, TargetAmount: d("200"), Projection: p},
		{ID: "b", TargetDate: &future, TargetAmount: d("100"), Projection: p},
		{ID: "a", TargetDate: &future, TargetAmount: d("100"), Projection: p},
	}
	if got := nextContributionGoal(list); got != 5 {
		t.Fatalf("selected %d", got)
	}
	if got := nextContributionGoal(list[:2]); got != -1 {
		t.Fatalf("ineligible selection %d", got)
	}
	if got := nextContributionGoal(nil); got != -1 {
		t.Fatalf("empty selection %d", got)
	}
}

func TestMonthlyPlanIgnoresLaterValuationAndReturnUpdates(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	old := now
	now = func() time.Time { return utc(2026, 10, 19) }
	defer func() { now = old }()
	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Custody"})
	if err != nil {
		t.Fatal(err)
	}
	value := d("1000")
	position, err := investments.CreatePosition(ctx, conn, investments.PositionInput{AccountID: account.ID, Name: "Reserve", AssetType: "Other", InitialValue: &value, OccurredOn: utc(2026, 9, 1)})
	if err != nil {
		t.Fatal(err)
	}
	_, err = investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: account.ID, PositionID: &position.ID, Kind: investments.OperationValuation, Amount: value, OccurredOn: utc(2026, 9, 1)})
	if err != nil {
		t.Fatal(err)
	}
	target := utc(2026, 12, 31)
	g, err := Create(ctx, conn, Input{Name: "Goal", Metric: MetricInvestedValue, TargetAmount: d("7000"), TargetDate: &target})
	if err != nil {
		t.Fatal(err)
	}
	if g.MonthlyContribution == nil || g.MonthlyContribution.Remaining == nil {
		t.Fatal("missing plan")
	}
	before := *g.MonthlyContribution.Remaining
	_, err = investments.CreateOperation(ctx, conn, investments.OperationInput{AccountID: account.ID, PositionID: &position.ID, Kind: investments.OperationValuation, Amount: d("1500"), OccurredOn: utc(2026, 10, 10)})
	if err != nil {
		t.Fatal(err)
	}
	g, err = Get(ctx, conn, g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if g.CurrentValue.String() != "1500" || !g.MonthlyContribution.Remaining.Equal(before) {
		t.Fatalf("valuation moved plan: %+v", g)
	}
	// A date edit restates the same month-opening plan with the new horizon.
	target = utc(2027, 3, 31)
	g, err = Update(ctx, conn, g.ID, Input{Name: g.Name, Metric: g.Metric, TargetAmount: g.TargetAmount, TargetDate: &target})
	if err != nil {
		t.Fatal(err)
	}
	if g.MonthlyContribution.Target.String() != "1000" {
		t.Fatalf("edited horizon = %+v", g.MonthlyContribution)
	}
}
