package payables_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/recurrences"
	"github.com/greg0x46/julius/internal/scenarios"
)

// Creating the first plan of a payable backfills its existing links. When that
// collides with another complete event, nothing may be left behind.
func TestCreatePlanIsAtomicWhenBackfillConflicts(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")
	now := time.Now()
	txID := f.addTransaction(txSpec{AccountID: acc, Description: "Pag", Amount: "-100", CurrencyCode: "BRL", MovementType: "DEBIT", ProviderStatus: "POSTED", OccurredAt: &now})
	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "D", dec(t, "1000"), dec(t, "1000"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := payables.CreateLink(ctx, f.conn, p.Kind, p.ID, txID); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	recID := "rec-1"
	f.exec(`INSERT INTO scenarios (id, kind, name, payable_id, is_active, is_accounting_source, created_at, updated_at) VALUES (?, 'recurring','R',NULL,1,0,'2026-01-01T00:00:00Z','2026-01-01T00:00:00Z')`, recID)
	f.exec(`INSERT INTO scenario_recurring_schedules (scenario_id, cashflow_kind, amount, category_id, account_id, cadence, day_of_month, month_of_year, start_date, end_date) VALUES (?, 'expense','100',NULL,NULL,'monthly',5,NULL,'2026-01-01',NULL)`, recID)
	if _, err := recurrences.PutOverride(ctx, f.conn, recID, time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC), recurrences.StateLinked, &txID); err != nil {
		t.Fatalf("PutOverride: %v", err)
	}

	_, err = scenarios.CreateScenario(ctx, f.conn, scenarios.KindDebtPlan, "plan", &p.ID)
	if !errors.Is(err, scenarios.ErrTransactionAlreadyRealized) {
		t.Fatalf("CreateScenario err = %v, want ErrTransactionAlreadyRealized", err)
	}
	list, err := scenarios.ListScenariosByPayable(ctx, f.conn, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("scenarios left behind = %d, want 0", len(list))
	}
}
