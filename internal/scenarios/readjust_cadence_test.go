package scenarios_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
)

// Reducing the term keeps the plan's cadence instead of turning a weekly or
// biweekly plan into a monthly one.
func TestReadjustReduceTermKeepsTheInferredCadence(t *testing.T) {
	cases := []struct {
		name    string
		cadence scenarios.Cadence
		want    []string
	}{
		{"weekly", scenarios.CadenceWeekly, []string{"2026-09-01", "2026-09-08", "2026-09-15"}},
		{"biweekly", scenarios.CadenceBiweekly, []string{"2026-09-01", "2026-09-15", "2026-09-29"}},
		{"monthly", scenarios.CadenceMonthly, []string{"2026-09-01", "2026-10-01", "2026-11-01"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := newTestDB(t)
			ctx := context.Background()
			d := newDebt(t, conn)
			s, _ := scenarios.CreateScenario(ctx, conn, scenarios.KindDebtPlan, "Plano", &d.ID)
			drafts, _ := scenarios.GenerateInstallments(dec(t, "1600.00"), 4, date(t, "2026-09-01"), c.cadence)
			if _, err := scenarios.CreateGeneratedInstallments(ctx, conn, s.ID, drafts); err != nil {
				t.Fatalf("CreateGeneratedInstallments: %v", err)
			}

			created, err := scenarios.Readjust(ctx, conn, s.ID, dec(t, "900.00"), scenarios.StrategyReduceTerm)
			if err != nil {
				t.Fatalf("Readjust: %v", err)
			}
			if len(created) != len(c.want) {
				t.Fatalf("len(created) = %d, want %d", len(created), len(c.want))
			}
			for i, want := range c.want {
				if !created[i].ProjectedAt.Equal(date(t, want)) {
					t.Errorf("installment[%d] = %s, want %s", i, created[i].ProjectedAt.Format("2006-01-02"), want)
				}
			}
		})
	}
}
