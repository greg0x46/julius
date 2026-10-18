package goals

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/greg0x46/julius/internal/db/dbtest"
	"github.com/greg0x46/julius/internal/investments"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func utc(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, time.UTC) }

func near(t *testing.T, label string, got, want, tolerance string) {
	t.Helper()
	if d(got).Sub(d(want)).Abs().GreaterThan(d(tolerance)) {
		t.Fatalf("%s = %s, want %s ± %s", label, got, want, tolerance)
	}
}

func TestMonthsUntilCountsAPartialMonthAsAMonth(t *testing.T) {
	today := utc(2026, 10, 18)
	for target, want := range map[time.Time]int{
		utc(2026, 10, 18): 0,
		utc(2026, 10, 1):  0,
		utc(2026, 10, 19): 1,
		utc(2026, 11, 18): 1,
		utc(2026, 11, 19): 2,
		utc(2027, 10, 18): 12,
		utc(2027, 12, 31): 15,
	} {
		if got := monthsUntil(today, target); got != want {
			t.Errorf("monthsUntil(%s) = %d, want %d", target.Format("2006-01-02"), got, want)
		}
	}
	if got := addMonths(utc(2026, 1, 31), 1); !got.Equal(utc(2026, 2, 28)) {
		t.Errorf("Jan 31 + 1 month = %s, want Feb 28", got.Format("2006-01-02"))
	}
}

func TestMonthlyRateCompoundsThePeriodReturn(t *testing.T) {
	// 1% a month for a year, measured over 365 days (≈ 365.25 / 12 per month).
	got, ok := monthlyRate(d("0.126825030131969"), 365)
	if !ok {
		t.Fatal("rate unavailable")
	}
	near(t, "monthly", got.String(), "0.01", "0.0001")

	if zero, ok := monthlyRate(decimal.Zero, 200); !ok || !zero.IsZero() {
		t.Errorf("flat period = %s, %v", zero, ok)
	}
	if loss, ok := monthlyRate(d("-0.2"), 365); !ok || !loss.IsNegative() {
		t.Errorf("loss = %s, %v, want negative", loss, ok)
	}
	if _, ok := monthlyRate(d("-1"), 365); ok {
		t.Error("a total loss has no monthly rate")
	}
}

func TestProjectSolvesTheContributionThatReachesTheTarget(t *testing.T) {
	one := d("0.01")

	// Without appreciation the gap is split evenly.
	flat := project(d("1000"), d("2200"), nil, 12)
	if flat.Status != StatusBehind || flat.ProjectedValue.String() != "1000" || flat.RequiredMonthlyContribution.String() != "100" {
		t.Fatalf("flat = %+v", flat)
	}

	// 1000 a month at 1% for 12 months is 12682.50…; starting from zero, that
	// is the contribution that reaches it.
	grown := project(decimal.Zero, d("12682.503013196"), &one, 12)
	near(t, "contribution", grown.RequiredMonthlyContribution.String(), "1000", "0.001")

	// Appreciation alone is enough: nothing to contribute.
	easy := project(d("1000"), d("1100"), &one, 12)
	if easy.Status != StatusOnTrack || !easy.RequiredMonthlyContribution.IsZero() {
		t.Fatalf("easy = %+v", easy)
	}
	near(t, "projected", easy.ProjectedValue.String(), "1126.825", "0.001")
	near(t, "projected progress", easy.ProjectedProgress.String(), "1.0243", "0.0001")

	done := project(d("1500"), d("1000"), nil, 6)
	if done.Status != StatusReached || !done.RequiredMonthlyContribution.IsZero() {
		t.Fatalf("done = %+v", done)
	}
}

func TestProjectWithDepreciationRecoversTheTarget(t *testing.T) {
	loss := d("-0.01")
	target := d("1000")
	p := project(d("900"), target, &loss, 12)
	if p.Status != StatusBehind || !p.RequiredMonthlyContribution.IsPositive() {
		t.Fatalf("p = %+v", p)
	}
	// Replaying the deposits month by month lands on the target.
	value := d("900")
	for i := 0; i < 12; i++ {
		value = value.Mul(d("0.99")).Add(p.RequiredMonthlyContribution)
	}
	near(t, "replayed value", value.String(), "1000", "0.001")
}

func TestProjectionFollowsTheMeasuredReturn(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	now = func() time.Time { return utc(2026, 10, 18) }
	t.Cleanup(func() { now = time.Now })

	account, err := investments.CreateAccount(ctx, conn, investments.AccountInput{Name: "Corretora"})
	if err != nil {
		t.Fatal(err)
	}
	initial := d("1000")
	position, err := investments.CreatePosition(ctx, conn, investments.PositionInput{
		AccountID: account.ID, Name: "CDB", AssetType: "Renda fixa", InitialValue: &initial,
		OccurredOn: utc(2026, 1, 1),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := investments.CreateOperation(ctx, conn, investments.OperationInput{
		AccountID: account.ID, PositionID: &position.ID, Kind: investments.OperationValuation,
		OccurredOn: utc(2026, 10, 1), Amount: d("1200"),
	}); err != nil {
		t.Fatal(err)
	}

	date := utc(2027, 10, 18)
	goal, err := Create(ctx, conn, Input{Name: "Reserva", Metric: MetricInvestedValue, TargetAmount: d("3000"), TargetDate: &date})
	if err != nil {
		t.Fatal(err)
	}
	p := goal.Projection
	if p == nil || p.Status != StatusBehind || p.MonthsRemaining != 12 || p.MonthlyReturn == nil || p.ReturnFrom == nil {
		t.Fatalf("projection = %+v", p)
	}
	// 20% since 2026-01-01 (290 days) is ≈ 1.9% a month.
	near(t, "monthly return", p.MonthlyReturn.String(), "0.0193", "0.0005")
	if !p.ReturnFrom.Equal(utc(2026, 1, 1)) || p.ReturnPartial {
		t.Fatalf("basis = %v partial=%v", p.ReturnFrom, p.ReturnPartial)
	}
	if p.ProjectedValue.LessThan(d("1400")) || !p.RequiredMonthlyContribution.IsPositive() {
		t.Fatalf("projection = %+v", p)
	}
	// Contributing the required amount at the measured rate reaches the target.
	value := goal.CurrentValue
	for i := 0; i < p.MonthsRemaining; i++ {
		value = value.Mul(d("1").Add(*p.MonthlyReturn)).Add(p.RequiredMonthlyContribution)
	}
	near(t, "replayed value", value.String(), "3000", "0.01")

	// No date, no projection; a past date has nothing left to project.
	if g, err := Create(ctx, conn, Input{Name: "Sem data", Metric: MetricInvestedValue, TargetAmount: d("3000")}); err != nil || g.Projection != nil {
		t.Fatalf("undated = %+v, %v", g.Projection, err)
	}
	past := utc(2026, 9, 1)
	if g, err := Create(ctx, conn, Input{Name: "Vencida", Metric: MetricInvestedValue, TargetAmount: d("3000"), TargetDate: &past}); err != nil || g.Projection != nil {
		t.Fatalf("past = %+v, %v", g.Projection, err)
	}
}

func TestProjectionAssumesNoAppreciationWithoutHistory(t *testing.T) {
	conn := dbtest.OpenSQLite(t)
	ctx := context.Background()
	now = func() time.Time { return utc(2026, 10, 18) }
	t.Cleanup(func() { now = time.Now })

	date := utc(2027, 10, 18)
	goal, err := Create(ctx, conn, Input{Name: "Reserva", Metric: MetricInvestedValue, TargetAmount: d("1200"), TargetDate: &date})
	if err != nil {
		t.Fatal(err)
	}
	p := goal.Projection
	if p == nil || p.MonthlyReturn != nil || p.ReturnFrom != nil || p.RequiredMonthlyContribution.String() != "100" {
		t.Fatalf("projection = %+v", p)
	}
}
