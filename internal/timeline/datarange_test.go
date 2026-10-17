package timeline_test

import (
	"context"
	"testing"
	"time"

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
