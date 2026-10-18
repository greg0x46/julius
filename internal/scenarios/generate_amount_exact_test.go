package scenarios_test

import (
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

func TestGenerateInstallmentsForAmountKeepsExactCountAndRemainder(t *testing.T) {
	for _, tt := range []struct {
		name, total, amount string
		want                []string
	}{
		{"above multiple", "1.00000000000000001", "1", []string{"1", "0.00000000000000001"}},
		{"below multiple", "1.99999999999999999", "1", []string{"1", "0.99999999999999999"}},
		{"exact multiple", "0.00000000000000006", "0.00000000000000003", []string{"0.00000000000000003", "0.00000000000000003"}},
		{"small total", "0.00000000000000001", "1", []string{"0.00000000000000001"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			total, amount := dec(t, tt.total), dec(t, tt.amount)
			got, err := scenarios.GenerateInstallmentsForAmount(total, amount, date(t, "2026-09-01"), scenarios.CadenceMonthly)
			if err != nil {
				t.Fatalf("GenerateInstallmentsForAmount: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("installment count = %d, want %d", len(got), len(tt.want))
			}
			sum := dec(t, "0")
			for i, installment := range got {
				if !installment.Amount.Equal(dec(t, tt.want[i])) {
					t.Errorf("installment[%d] = %s, want %s", i, installment.Amount, tt.want[i])
				}
				if !installment.Amount.IsPositive() || installment.Amount.GreaterThan(amount) {
					t.Errorf("installment[%d] = %s, must be positive and at most %s", i, installment.Amount, amount)
				}
				sum = sum.Add(installment.Amount)
			}
			if !sum.Equal(total) {
				t.Errorf("sum = %s, want %s", sum, total)
			}
		})
	}
}

func TestReadjustReduceTermPreservesRemainderBeyondDivisionPrecision(t *testing.T) {
	conn := newTestDB(t)
	ctx := t.Context()
	d := newDebt(t, conn)
	s, err := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Plano", &d.ID)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if _, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Parcela 1", dec(t, "1"), date(t, "2026-09-01"), nil); err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}

	total := dec(t, "1.00000000000000001")
	got, err := scenarios.Readjust(ctx, conn, s.ID, total, scenarios.StrategyReduceTerm)
	if err != nil {
		t.Fatalf("Readjust: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("installment count = %d, want 2", len(got))
	}
	if !got[0].Amount.Equal(dec(t, "1")) || !got[1].Amount.Equal(dec(t, "0.00000000000000001")) {
		t.Errorf("installment amounts = %s, %s, want 1 and 0.00000000000000001", got[0].Amount, got[1].Amount)
	}
	if !got[0].Amount.Add(got[1].Amount).Equal(total) {
		t.Errorf("installments do not sum to %s", total)
	}
}
