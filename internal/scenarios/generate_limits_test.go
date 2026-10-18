package scenarios_test

import (
	"errors"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

func TestGenerateInstallmentsRejectsExcessiveCounts(t *testing.T) {
	start := date(t, "2026-09-01")
	if got, err := scenarios.GenerateInstallments(dec(t, "1"), scenarios.MaxInstallments+1, start, scenarios.CadenceMonthly); !errors.Is(err, scenarios.ErrTooManyInstallments) || got != nil {
		t.Fatalf("count above limit: got %d installments, err=%v", len(got), err)
	}
	for _, total := range []string{"10000.00000000000000001", "18446744073709551616"} {
		t.Run(total, func(t *testing.T) {
			got, err := scenarios.GenerateInstallmentsForAmount(dec(t, total), dec(t, "1"), start, scenarios.CadenceMonthly)
			if !errors.Is(err, scenarios.ErrTooManyInstallments) || got != nil {
				t.Fatalf("got %d installments, err=%v; want ErrTooManyInstallments", len(got), err)
			}
		})
	}
	for _, total := range []string{"9999.00000000000000001", "10000"} {
		got, err := scenarios.GenerateInstallmentsForAmount(dec(t, total), dec(t, "1"), start, scenarios.CadenceMonthly)
		if err != nil || len(got) != scenarios.MaxInstallments {
			t.Fatalf("total=%s: got %d installments, err=%v; want %d", total, len(got), err, scenarios.MaxInstallments)
		}
	}
	if got, err := scenarios.GenerateInstallments(dec(t, "10000"), scenarios.MaxInstallments, start, scenarios.CadenceMonthly); err != nil || len(got) != scenarios.MaxInstallments {
		t.Fatalf("count at limit: got %d installments, err=%v", len(got), err)
	}
}

func TestReadjustRejectsExcessiveCountWithoutChangingPlan(t *testing.T) {
	conn := newTestDB(t)
	ctx := t.Context()
	d := newDebt(t, conn)
	s, err := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Synthetic plan", &d.ID)
	if err != nil {
		t.Fatal(err)
	}
	original, err := scenarios.CreateScenarioTransaction(ctx, conn, s.ID, "Preserved installment", dec(t, "1"), date(t, "2026-09-01"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, remaining := range []string{"10000.00000000000000001", "18446744073709551616"} {
		if got, err := scenarios.Readjust(ctx, conn, s.ID, dec(t, remaining), scenarios.StrategyReduceTerm); !errors.Is(err, scenarios.ErrTooManyInstallments) || got != nil {
			t.Fatalf("remaining=%s: got %d installments, err=%v", remaining, len(got), err)
		}
		stored, err := scenarios.ListScenarioTransactions(ctx, conn, s.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(stored) != 1 || stored[0].ID != original.ID || stored[0].Description != original.Description || !stored[0].Amount.Equal(original.Amount) || !stored[0].ProjectedAt.Equal(original.ProjectedAt) {
			t.Fatalf("plan changed after rejection: %+v", stored)
		}
	}
}
