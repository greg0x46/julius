package scenarios_test

import (
	"errors"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

func TestGenerateInstallmentsSumMatchesTotalWithLastAbsorbingRemainder(t *testing.T) {
	total := dec(t, "1000.00")
	got, err := scenarios.GenerateInstallments(total, 3, date(t, "2026-09-01"), scenarios.CadenceMonthly)
	if err != nil {
		t.Fatalf("GenerateInstallments: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}

	sum := dec(t, "0")
	for _, ins := range got {
		sum = sum.Add(ins.Amount)
	}
	if !sum.Equal(total) {
		t.Errorf("sum = %s, want %s", sum, total)
	}

	if !got[0].Amount.Equal(dec(t, "333.33")) || !got[1].Amount.Equal(dec(t, "333.33")) {
		t.Errorf("non-last installments = %s, %s, want 333.33 each", got[0].Amount, got[1].Amount)
	}
	if !got[2].Amount.Equal(dec(t, "333.34")) {
		t.Errorf("last installment = %s, want 333.34 (absorbs the remainder)", got[2].Amount)
	}
}

func TestGenerateInstallmentsEvenDivisionHasNoRemainder(t *testing.T) {
	got, err := scenarios.GenerateInstallments(dec(t, "900.00"), 3, date(t, "2026-09-01"), scenarios.CadenceMonthly)
	if err != nil {
		t.Fatalf("GenerateInstallments: %v", err)
	}
	for _, ins := range got {
		if !ins.Amount.Equal(dec(t, "300.00")) {
			t.Errorf("installment = %s, want 300.00", ins.Amount)
		}
	}
}

// TestGenerateInstallmentsMonthlyClampsToMonthEndKeepingTheAnchorDay pins the
// monthly rule for starts on the 29th-31st across a non-leap and a leap
// February: a short month takes its last day (never spills into the next
// month) and the next month returns to the anchor day (no drift).
func TestGenerateInstallmentsMonthlyClampsToMonthEndKeepingTheAnchorDay(t *testing.T) {
	for _, tc := range []struct {
		name  string
		start string
		want  []string
	}{
		{"31st, non-leap", "2026-01-31", []string{"2026-01-31", "2026-02-28", "2026-03-31", "2026-04-30", "2026-05-31", "2026-06-30"}},
		{"30th, non-leap", "2026-01-30", []string{"2026-01-30", "2026-02-28", "2026-03-30", "2026-04-30", "2026-05-30", "2026-06-30"}},
		{"29th, non-leap", "2026-01-29", []string{"2026-01-29", "2026-02-28", "2026-03-29", "2026-04-29", "2026-05-29", "2026-06-29"}},
		{"31st, leap", "2028-01-31", []string{"2028-01-31", "2028-02-29", "2028-03-31", "2028-04-30", "2028-05-31", "2028-06-30"}},
		{"30th, leap", "2028-01-30", []string{"2028-01-30", "2028-02-29", "2028-03-30", "2028-04-30", "2028-05-30", "2028-06-30"}},
		{"29th, leap", "2028-01-29", []string{"2028-01-29", "2028-02-29", "2028-03-29", "2028-04-29", "2028-05-29", "2028-06-29"}},
		{"31st across the year end", "2026-10-31", []string{"2026-10-31", "2026-11-30", "2026-12-31", "2027-01-31", "2027-02-28", "2027-03-31"}},
		{"start already clamped", "2026-02-28", []string{"2026-02-28", "2026-03-28", "2026-04-28", "2026-05-28", "2026-06-28", "2026-07-28"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scenarios.GenerateInstallments(dec(t, "600.00"), len(tc.want), date(t, tc.start), scenarios.CadenceMonthly)
			if err != nil {
				t.Fatalf("GenerateInstallments: %v", err)
			}
			for i, ins := range got {
				if got := ins.ProjectedAt.Format("2006-01-02"); got != tc.want[i] {
					t.Errorf("installment[%d].ProjectedAt = %s, want %s", i, got, tc.want[i])
				}
			}
		})
	}
}

// TestGenerateInstallmentsForAmountMonthlyClampsToMonthEnd checks the
// fixed-amount generator, which derives the count, shares the clamping rule.
func TestGenerateInstallmentsForAmountMonthlyClampsToMonthEnd(t *testing.T) {
	got, err := scenarios.GenerateInstallmentsForAmount(dec(t, "900.00"), dec(t, "300.00"), date(t, "2026-01-31"), scenarios.CadenceMonthly)
	if err != nil {
		t.Fatalf("GenerateInstallmentsForAmount: %v", err)
	}
	wantDates := []string{"2026-01-31", "2026-02-28", "2026-03-31"}
	if len(got) != len(wantDates) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(wantDates))
	}
	for i, ins := range got {
		if got := ins.ProjectedAt.Format("2006-01-02"); got != wantDates[i] {
			t.Errorf("installment[%d].ProjectedAt = %s, want %s", i, got, wantDates[i])
		}
	}
}

// TestGenerateInstallmentsWeeklyAndBiweeklyIgnoreMonthEnd pins that only the
// monthly cadence clamps: weekly and biweekly keep stepping a fixed number of
// days across a month end.
func TestGenerateInstallmentsWeeklyAndBiweeklyIgnoreMonthEnd(t *testing.T) {
	for _, tc := range []struct {
		cadence scenarios.Cadence
		want    []string
	}{
		{scenarios.CadenceWeekly, []string{"2026-01-31", "2026-02-07", "2026-02-14"}},
		{scenarios.CadenceBiweekly, []string{"2026-01-31", "2026-02-14", "2026-02-28"}},
	} {
		got, err := scenarios.GenerateInstallments(dec(t, "300.00"), 3, date(t, "2026-01-31"), tc.cadence)
		if err != nil {
			t.Fatalf("%s: GenerateInstallments: %v", tc.cadence, err)
		}
		for i, ins := range got {
			if got := ins.ProjectedAt.Format("2006-01-02"); got != tc.want[i] {
				t.Errorf("%s: installment[%d].ProjectedAt = %s, want %s", tc.cadence, i, got, tc.want[i])
			}
		}
	}
}

func TestGenerateInstallmentsRejectsInvalidInput(t *testing.T) {
	if _, err := scenarios.GenerateInstallments(dec(t, "0"), 3, date(t, "2026-01-01"), scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("zero amount: err = %v, want ErrInvalidInstallmentPlan", err)
	}
	if _, err := scenarios.GenerateInstallments(dec(t, "-100"), 3, date(t, "2026-01-01"), scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("negative amount: err = %v, want ErrInvalidInstallmentPlan", err)
	}
	if _, err := scenarios.GenerateInstallments(dec(t, "100"), 0, date(t, "2026-01-01"), scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("zero months: err = %v, want ErrInvalidInstallmentPlan", err)
	}
}

func TestCreateGeneratedInstallmentsPersistsAll(t *testing.T) {
	conn := newTestDB(t)
	d := newDebt(t, conn)
	s, _ := scenarios.CreateScenario(t.Context(), conn, scenarios.KindDebtPlan, "Plano", &d.ID)

	drafts, err := scenarios.GenerateInstallments(dec(t, "1200.00"), 4, date(t, "2026-09-01"), scenarios.CadenceMonthly)
	if err != nil {
		t.Fatalf("GenerateInstallments: %v", err)
	}
	created, err := scenarios.CreateGeneratedInstallments(t.Context(), conn, s.ID, drafts)
	if err != nil {
		t.Fatalf("CreateGeneratedInstallments: %v", err)
	}
	if len(created) != 4 {
		t.Fatalf("len(created) = %d, want 4", len(created))
	}

	list, err := scenarios.ListScenarioTransactions(t.Context(), conn, s.ID)
	if err != nil {
		t.Fatalf("ListScenarioTransactions: %v", err)
	}
	if len(list) != 4 {
		t.Fatalf("len(list) = %d, want 4", len(list))
	}
}

func TestGenerateInstallmentsWeeklyCadence(t *testing.T) {
	got, err := scenarios.GenerateInstallments(dec(t, "300.00"), 3, date(t, "2026-01-01"), scenarios.CadenceWeekly)
	if err != nil {
		t.Fatalf("GenerateInstallments: %v", err)
	}
	wantDates := []string{"2026-01-01", "2026-01-08", "2026-01-15"}
	for i, ins := range got {
		if got := ins.ProjectedAt.Format("2006-01-02"); got != wantDates[i] {
			t.Errorf("installment[%d].ProjectedAt = %s, want %s", i, got, wantDates[i])
		}
	}
}

func TestGenerateInstallmentsBiweeklyCadence(t *testing.T) {
	got, err := scenarios.GenerateInstallments(dec(t, "300.00"), 3, date(t, "2026-01-01"), scenarios.CadenceBiweekly)
	if err != nil {
		t.Fatalf("GenerateInstallments: %v", err)
	}
	wantDates := []string{"2026-01-01", "2026-01-15", "2026-01-29"}
	for i, ins := range got {
		if got := ins.ProjectedAt.Format("2006-01-02"); got != wantDates[i] {
			t.Errorf("installment[%d].ProjectedAt = %s, want %s", i, got, wantDates[i])
		}
	}
}

func TestGenerateInstallmentsRejectsInvalidCadence(t *testing.T) {
	if _, err := scenarios.GenerateInstallments(dec(t, "100"), 3, date(t, "2026-01-01"), scenarios.Cadence("bogus")); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("err = %v, want ErrInvalidInstallmentPlan", err)
	}
}

func TestGenerateInstallmentsForAmountDerivesCountAndAbsorbsRemainder(t *testing.T) {
	got, err := scenarios.GenerateInstallmentsForAmount(dec(t, "1000.00"), dec(t, "400.00"), date(t, "2026-09-01"), scenarios.CadenceMonthly)
	if err != nil {
		t.Fatalf("GenerateInstallmentsForAmount: %v", err)
	}
	// ceil(1000/400) = 3 installments: 400, 400, 200.
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	sum := dec(t, "0")
	for i, ins := range got {
		sum = sum.Add(ins.Amount)
		if i < 2 && !ins.Amount.Equal(dec(t, "400.00")) {
			t.Errorf("installment[%d] = %s, want 400.00", i, ins.Amount)
		}
	}
	if !got[2].Amount.Equal(dec(t, "200.00")) {
		t.Errorf("last installment = %s, want 200.00 (absorbs remainder)", got[2].Amount)
	}
	if !sum.Equal(dec(t, "1000.00")) {
		t.Errorf("sum = %s, want 1000.00", sum)
	}
}

func TestGenerateInstallmentsForAmountWithBiweeklyCadence(t *testing.T) {
	got, err := scenarios.GenerateInstallmentsForAmount(dec(t, "600.00"), dec(t, "300.00"), date(t, "2026-01-01"), scenarios.CadenceBiweekly)
	if err != nil {
		t.Fatalf("GenerateInstallmentsForAmount: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	wantDates := []string{"2026-01-01", "2026-01-15"}
	for i, ins := range got {
		if got := ins.ProjectedAt.Format("2006-01-02"); got != wantDates[i] {
			t.Errorf("installment[%d].ProjectedAt = %s, want %s", i, got, wantDates[i])
		}
	}
}

func TestGenerateInstallmentsForAmountRejectsInvalidInput(t *testing.T) {
	if _, err := scenarios.GenerateInstallmentsForAmount(dec(t, "0"), dec(t, "100"), date(t, "2026-01-01"), scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("zero total: err = %v, want ErrInvalidInstallmentPlan", err)
	}
	if _, err := scenarios.GenerateInstallmentsForAmount(dec(t, "100"), dec(t, "0"), date(t, "2026-01-01"), scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrInvalidInstallmentPlan) {
		t.Errorf("zero installment amount: err = %v, want ErrInvalidInstallmentPlan", err)
	}
}
