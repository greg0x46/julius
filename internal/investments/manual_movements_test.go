package investments_test

import (
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/investments"
)

func (f *ledgerFixture) recordMovement(investmentID string, kind investments.ManualMovementKind, on time.Time, quantity, amount string) investments.ManualMovement {
	f.t.Helper()
	in := investments.ManualMovementInput{InvestmentID: investmentID, Kind: kind, TradeDate: on, Amount: dec(amount)}
	if quantity != "" {
		in.Quantity = decRef(quantity)
	}
	movement, err := investments.CreateManualMovement(f.ctx, f.conn, in)
	if err != nil {
		f.t.Fatalf("CreateManualMovement: %v", err)
	}
	return movement
}

// The state that blanked the wealth history: a holding the provider reports
// with its quantity but no movement. A recorded purchase says when it started,
// so the days before it are zero instead of unknowable.
func TestManualPurchaseGivesAProviderHoldingItsHistory(t *testing.T) {
	f := newLedgerFixture(t)
	holding := equity("7")
	id := f.addHolding(holding)
	f.exec(`UPDATE financial_investments SET balance = '280' WHERE id = ?`, id)
	f.recordQuote(holding, "2026-09-20T12:00:00Z", "280")

	before := valuesOn(t, f, day(5), day(12), day(20))
	for i, label := range []string{"day 5", "day 12", "day 20"} {
		if want := i == 2; before[i].Complete != want {
			t.Errorf("%s complete = %v before any movement, want %v", label, before[i].Complete, want)
		}
	}

	f.recordMovement(id, investments.ManualMovementBuy, day(10), "7", "259")

	after := valuesOn(t, f, day(5), day(12), day(20))
	wantBRL(t, "before the purchase", after[0], "0")
	if after[1].Complete {
		t.Error("a day after the purchase and before the first price should stay incomplete, not invent a price")
	}
	wantBRL(t, "priced day", after[2], "280")

	result := f.yield(id, nil, "2026-09-20")
	if result.Value.String() != "21" || result.NetFlows.String() != "259" {
		t.Fatalf("yield = %s on flows %s, want 21 on 259", result.Value, result.NetFlows)
	}
}

func TestManualMovementsDirectionFollowsItsKind(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("10"))
	for kind, want := range map[investments.ManualMovementKind]string{
		investments.ManualMovementBuy:      "inflow",
		investments.ManualMovementSell:     "outflow",
		investments.ManualMovementInterest: "outflow",
	} {
		quantity := "1"
		if kind == investments.ManualMovementInterest {
			quantity = ""
		}
		if got := f.recordMovement(id, kind, day(3), quantity, "10").Direction; got != want {
			t.Errorf("%s direction = %s, want %s", kind, got, want)
		}
	}
}

func TestManualMovementCRUD(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("10"))
	notes := "  nota  "
	created, err := investments.CreateManualMovement(f.ctx, f.conn, investments.ManualMovementInput{
		InvestmentID: id, Kind: investments.ManualMovementBuy, TradeDate: day(3), Quantity: decRef("10"),
		UnitPrice: decRef("10"), Amount: dec("100"), Notes: &notes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Notes == nil || *created.Notes != "nota" || created.UnitPrice == nil || !created.UnitPrice.Equal(dec("10")) {
		t.Fatalf("created = %+v, want trimmed notes and the unit price kept", created)
	}

	updated, err := investments.UpdateManualMovement(f.ctx, f.conn, created.ID, investments.ManualMovementInput{
		InvestmentID: id, Kind: investments.ManualMovementSell, TradeDate: day(4), Quantity: decRef("4"), Amount: dec("50"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Kind != investments.ManualMovementSell || updated.Direction != "outflow" || updated.Notes != nil || updated.UnitPrice != nil || !updated.Amount.Equal(dec("50")) {
		t.Fatalf("updated = %+v, want a sale of 50 with no notes or price", updated)
	}

	listed, err := investments.ListManualMovements(f.ctx, f.conn, id)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list = %+v, %v; want the one movement", listed, err)
	}
	if err := investments.DeleteManualMovement(f.ctx, f.conn, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := investments.DeleteManualMovement(f.ctx, f.conn, created.ID); !errors.Is(err, investments.ErrNotFound) {
		t.Fatalf("second delete = %v, want ErrNotFound", err)
	}
	if _, err := investments.GetManualMovement(f.ctx, f.conn, created.ID); !errors.Is(err, investments.ErrNotFound) {
		t.Fatalf("get after delete = %v, want ErrNotFound", err)
	}
}

func TestManualMovementValidation(t *testing.T) {
	f := newLedgerFixture(t)
	id := f.addHolding(equity("10"))
	base := investments.ManualMovementInput{InvestmentID: id, Kind: investments.ManualMovementBuy, TradeDate: day(3), Quantity: decRef("1"), Amount: dec("1")}
	for name, mutate := range map[string]func(*investments.ManualMovementInput){
		"unknown kind":         func(in *investments.ManualMovementInput) { in.Kind = "TAX" },
		"no date":              func(in *investments.ManualMovementInput) { in.TradeDate = time.Time{} },
		"future date":          func(in *investments.ManualMovementInput) { in.TradeDate = time.Now().AddDate(0, 0, 2) },
		"zero amount":          func(in *investments.ManualMovementInput) { in.Amount = dec("0") },
		"purchase without qty": func(in *investments.ManualMovementInput) { in.Quantity = nil },
		"negative quantity":    func(in *investments.ManualMovementInput) { in.Quantity = decRef("-1") },
		"zero unit price":      func(in *investments.ManualMovementInput) { in.UnitPrice = decRef("0") },
	} {
		t.Run(name, func(t *testing.T) {
			in := base
			mutate(&in)
			if _, err := investments.CreateManualMovement(f.ctx, f.conn, in); !errors.Is(err, investments.ErrInvalidInput) {
				t.Fatalf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
	// A provento carries no units.
	provento := base
	provento.Kind, provento.Quantity = investments.ManualMovementInterest, nil
	if _, err := investments.CreateManualMovement(f.ctx, f.conn, provento); err != nil {
		t.Fatalf("provento without quantity: %v", err)
	}
	missing := base
	missing.InvestmentID = "00000000-0000-4000-8000-000000000000"
	if _, err := investments.CreateManualMovement(f.ctx, f.conn, missing); !errors.Is(err, investments.ErrNotFound) {
		t.Fatalf("unknown holding err = %v, want ErrNotFound", err)
	}
}
