package payables_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/greg0x46/julius/internal/payables"
)

// countingQuerier wraps a *sql.DB and counts round trips, so a test can
// assert a read path's cost in queries rather than in wall-clock time.
type countingQuerier struct {
	db *sql.DB
	n  int
}

func (c *countingQuerier) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	c.n++
	return c.db.QueryContext(ctx, query, args...)
}

func (c *countingQuerier) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	c.n++
	return c.db.QueryRowContext(ctx, query, args...)
}

func (c *countingQuerier) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	c.n++
	return c.db.ExecContext(ctx, query, args...)
}

// seedPayables creates payableCount backdated debts with linkCount settled
// links each, and returns the fixture holding them.
func seedPayables(t *testing.T, f *fixture, payableCount, linkCount int) {
	t.Helper()
	ctx := context.Background()
	acc := f.addAccount("BRL")
	for i := 0; i < payableCount; i++ {
		p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Dívida", dec(t, "5000.00"), dec(t, "5000.00"))
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		f.backdatePayable(p.ID, day(t, "2025-01-01"))
		for j := 0; j < linkCount; j++ {
			tx := f.addTransaction(payment(acc, "100.00", day(t, "2026-03-10").AddDate(0, j, 0)))
			if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, tx); err != nil {
				t.Fatalf("CreateLink: %v", err)
			}
		}
	}
}

// TestRemainingTotalRoundTripsDoNotGrowWithLinks guards the batching these
// read paths depend on. The numbers themselves are covered elsewhere; what
// this pins is that they cost a fixed number of queries — the shape that a
// well-meaning "just call Summarize in a loop" refactor would quietly undo,
// with no failing assertion anywhere to catch it.
func TestRemainingTotalRoundTripsDoNotGrowWithLinks(t *testing.T) {
	f := newFixture(t)
	seedPayables(t, f, 5, 6)

	c := &countingQuerier{db: f.conn}
	if _, err := payables.RemainingTotal(context.Background(), c, payables.KindDebt); err != nil {
		t.Fatalf("RemainingTotal: %v", err)
	}
	// List, then links for the whole set, then the transactions behind them.
	if c.n > 3 {
		t.Errorf("RemainingTotal over 5 payables x 6 links = %d queries, want <= 3 — "+
			"a per-payable or per-link query has crept back in", c.n)
	}
}

// TestSummarizeAllRoundTripsDoNotGrowWithPayables guards the read path the
// payables list endpoint uses. It is the one a "just call Summarize in a
// loop" handler gets wrong most naturally, because that loop is correct —
// only expensive, and nothing else here would fail because of it.
func TestSummarizeAllRoundTripsDoNotGrowWithPayables(t *testing.T) {
	f := newFixture(t)
	seedPayables(t, f, 5, 6)

	list, err := payables.List(context.Background(), f.conn, nil)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	c := &countingQuerier{db: f.conn}
	summaries, err := payables.SummarizeAll(context.Background(), c, list)
	if err != nil {
		t.Fatalf("SummarizeAll: %v", err)
	}
	if len(summaries) != 5 {
		t.Fatalf("len(summaries) = %d, want 5 — every payable must have an entry", len(summaries))
	}
	// Links for the whole set, then the transactions behind them.
	if c.n > 2 {
		t.Errorf("SummarizeAll over 5 payables x 6 links = %d queries, want <= 2 — "+
			"a per-payable query has crept back in", c.n)
	}
}
