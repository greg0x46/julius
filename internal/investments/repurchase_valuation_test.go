package investments_test

import (
	"testing"

	"github.com/greg0x46/julius/internal/investments"
)

// Selling a holding out ends its valuation: a later purchase starts again at
// cost instead of scaling the old typed value to the new quantity.
func TestRepurchaseAfterSellingOutDoesNotReuseTheOldValuation(t *testing.T) {
	eachLedger(t, func(t *testing.T, f *ledgerFixture) {
		positionID := f.addPosition(f.custodyID, "Ações")
		f.deposit("5000", day(1))
		f.buy(positionID, day(2), "1000", "10")
		f.valuation(positionID, day(3), "1200")
		f.sell(positionID, day(4), "1200", "10")
		f.buy(positionID, day(10), "500", "5")

		position, err := investments.GetPosition(f.ctx, f.conn, positionID)
		if err != nil {
			t.Fatal(err)
		}
		if position.TotalCost == nil || position.CurrentValue.String() != "500" || position.CurrentValue.Cmp(*position.TotalCost) != 0 {
			t.Fatalf("value = %s, cost = %v; want both 500", position.CurrentValue, position.TotalCost)
		}
		if position.ValuationBasis != investments.ValuationBasisCostBasis || position.ValuedOn != nil {
			t.Fatalf("basis = %s, valued on %v; want cost_basis and no valuation day", position.ValuationBasis, position.ValuedOn)
		}
		days, err := investments.DailyYield(f.ctx, f.conn, positionID, day(9), day(10))
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range days {
			if !d.Value.IsZero() {
				t.Fatalf("yield on %s = %s, want 0 for a purchase at cost", d.Day.Format(investments.DateLayout), d.Value)
			}
		}
	})
}
