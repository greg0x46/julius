package timeline_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/timeline"
)

// A simulation is Base plus the selected scenarios: active commitments stay in
// it, and the impact is only what the selected scenario adds.
func TestSimulationKeepsActiveScenariosAndImpactIsOnlySelected(t *testing.T) {
	f := newFixture(t)
	f.addAccount("1000.00")
	ctx := context.Background()

	salary, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindRecurring, "Salário", nil)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	f.exec(`INSERT INTO scenario_recurring_schedules (scenario_id, cashflow_kind, amount, category_id, account_id, cadence, day_of_month, month_of_year, start_date, end_date) VALUES (?, 'income','5000',NULL,NULL,'monthly',20,NULL,'2026-01-01',NULL)`, salary.ID)
	trip, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindStandalone, "Viagem", nil)
	if err != nil {
		t.Fatalf("CreateScenario: %v", err)
	}
	if _, err := scenarios.CreateScenarioTransaction(ctx, f.conn, trip.ID, "Passagem", decT(t, "-300.00"), date(t, "2026-09-10"), nil); err != nil {
		t.Fatalf("CreateScenarioTransaction: %v", err)
	}

	params := timeline.BuildParams{From: date(t, "2026-08-15"), To: date(t, "2026-09-30"), ReferenceDate: date(t, "2026-08-15")}
	base, err := timeline.BuildSeries(ctx, f.conn, params)
	if err != nil {
		t.Fatalf("BuildSeries base: %v", err)
	}
	withTrip := params
	withTrip.ScenarioIDs = []string{trip.ID, trip.ID}
	simulation, err := timeline.BuildSeries(ctx, f.conn, withTrip)
	if err != nil {
		t.Fatalf("BuildSeries simulation: %v", err)
	}

	keys := map[string]int{}
	for _, e := range simulation.Entries {
		keys[e.EventKey]++
	}
	recurring := 0
	for _, e := range base.Entries {
		if e.ScenarioID != nil && *e.ScenarioID == salary.ID {
			recurring++
		}
		if keys[e.EventKey] != 1 {
			t.Errorf("base entry %q appears %d times in the simulation, want 1", e.EventKey, keys[e.EventKey])
		}
	}
	if recurring == 0 {
		t.Fatal("base has no active recurring entries; fixture is not exercising the bug")
	}
	if len(simulation.Entries) != len(base.Entries)+1 {
		t.Errorf("simulation entries = %d, want base %d + 1", len(simulation.Entries), len(base.Entries))
	}
	if got := timeline.CompareBaseVsSimulation(base, simulation).Impact.String(); got != "-300" {
		t.Errorf("Impact = %s, want -300", got)
	}
	impact, err := timeline.ScenarioImpact(ctx, f.conn, params, base, trip.ID)
	if err != nil {
		t.Fatalf("ScenarioImpact: %v", err)
	}
	if impact.Delta.String() != "-300" {
		t.Errorf("ScenarioImpact = %s, want -300", impact.Delta)
	}
}
