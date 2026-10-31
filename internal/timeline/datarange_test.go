package timeline_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/dates"
	"github.com/greg0x46/julius/internal/payables"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/timeline"
)

func TestDataRangeUsesLocalCalendarDays(t *testing.T) {
	f := newFixture(t)
	today := time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC)
	rangeWithoutHistory, err := timeline.LoadDataRange(context.Background(), f.conn, today, "America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	if rangeWithoutHistory.From != date(t, "2026-03-01") || rangeWithoutHistory.To != date(t, "2026-03-31") {
		t.Fatalf("empty local March range = %+v", rangeWithoutHistory)
	}
	f.addTransaction(txn{AccountID: f.addAccount("100"), Amount: "-10", OccurredAt: time.Date(2026, 3, 16, 1, 0, 0, 0, time.UTC)})
	for _, tc := range []struct{ zone, from, to string }{
		{"America/Sao_Paulo", "2026-03-15", "2026-03-31"},
		{"UTC", "2026-03-16", "2026-04-30"},
		{"", "2026-03-16", "2026-04-30"},
	} {
		got, err := timeline.LoadDataRange(context.Background(), f.conn, today, tc.zone)
		if err != nil {
			t.Fatal(err)
		}
		if got.From != date(t, tc.from) || got.To != date(t, tc.to) {
			t.Errorf("zone %s range = %+v, want %s through %s", tc.zone, got, tc.from, tc.to)
		}
	}
}

func TestDataRangeNeverExceedsTheSpanCap(t *testing.T) {
	ctx := context.Background()
	today := time.Date(2026, 4, 1, 1, 0, 0, 0, time.UTC)
	toEnd := date(t, "2026-04-30")
	floor := toEnd.AddDate(0, 0, -dates.MaxSpanDays)

	t.Run("mistyped year in a transaction", func(t *testing.T) {
		f := newFixture(t)
		account := f.addAccount("100")
		f.addTransaction(txn{AccountID: account, Amount: "-10", OccurredAt: time.Date(202, 3, 16, 12, 0, 0, 0, time.UTC)})
		f.addTransaction(txn{AccountID: account, Amount: "-10", OccurredAt: time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)})
		got, err := timeline.LoadDataRange(ctx, f.conn, today, "UTC")
		if err != nil {
			t.Fatal(err)
		}
		if got.To != toEnd || got.From != floor {
			t.Fatalf("range = %+v, want %s through %s", got, floor.Format("2006-01-02"), toEnd.Format("2006-01-02"))
		}
		if dates.ExceedsMaxSpan(got.From, got.To) {
			t.Fatalf("range %+v exceeds the cap", got)
		}
	})

	t.Run("exactly at the cap is kept", func(t *testing.T) {
		f := newFixture(t)
		f.addTransaction(txn{AccountID: f.addAccount("100"), Amount: "-10", OccurredAt: floor.Add(12 * time.Hour)})
		got, err := timeline.LoadDataRange(ctx, f.conn, today, "UTC")
		if err != nil {
			t.Fatal(err)
		}
		if got.From != floor || got.To != toEnd {
			t.Fatalf("range = %+v, want %s through %s", got, floor.Format("2006-01-02"), toEnd.Format("2006-01-02"))
		}
	})

	t.Run("planned installment far in the future", func(t *testing.T) {
		f := newFixture(t)
		f.addTransaction(txn{AccountID: f.addAccount("100"), Amount: "-10", OccurredAt: time.Date(1950, 1, 2, 12, 0, 0, 0, time.UTC)})
		debt, err := payables.Create(ctx, f.conn, payables.KindDebt, "Dívida", decT(t, "100"), decT(t, "100"))
		if err != nil {
			t.Fatal(err)
		}
		plan, err := scenarios.CreateScenario(ctx, f.conn, scenarios.KindDebtPlan, "Plano", &debt.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scenarios.CreateScenarioTransaction(ctx, f.conn, plan.ID, "Parcela", decT(t, "10"), date(t, "2150-01-01"), nil); err != nil {
			t.Fatal(err)
		}
		got, err := timeline.LoadDataRange(ctx, f.conn, today, "UTC")
		if err != nil {
			t.Fatal(err)
		}
		if got.To != date(t, "2150-01-01") || dates.ExceedsMaxSpan(got.From, got.To) {
			t.Fatalf("range = %+v, want a window of at most %d days ending 2150-01-01", got, dates.MaxSpanDays)
		}
	})
}
