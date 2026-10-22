package payables_test

import (
	"context"
	"testing"
	"time"

	"github.com/greg0x46/julius/internal/payables"
)

// payment returns a txSpec for an outflow that can settle a debt, dated at
// occurredAt so the as-of tests have something to cut on.
func payment(acc, amount string, occurredAt time.Time) txSpec {
	return txSpec{
		AccountID: acc, Description: "Pagamento", Amount: "-" + amount,
		CurrencyCode: "BRL", MovementType: "DEBIT", ProviderStatus: "POSTED",
		OccurredAt: &occurredAt,
	}
}

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestSummarizeRecomputesFromLinks is the invariant the whole package exists
// to protect: settled/remaining/status come from the links every time, never
// from a stored column. It also pins that Summary carries the rows it
// derived from — the detail view reads them instead of reloading each link
// and the transaction behind it.
func TestSummarizeRecomputesFromLinks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")

	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "1000.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tx := f.addTransaction(payment(acc, "300.00", day(t, "2026-03-10")))
	if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, tx); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	summary, err := payables.Summarize(ctx, f.conn, p)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !summary.Settled.Equal(dec(t, "300.00")) || !summary.Remaining.Equal(dec(t, "700.00")) {
		t.Errorf("settled/remaining = %s/%s, want 300.00/700.00", summary.Settled, summary.Remaining)
	}
	if summary.Status != payables.StatusOpen {
		t.Errorf("Status = %s, want open", summary.Status)
	}
	if len(summary.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1", len(summary.Links))
	}
	link := summary.Links[0]
	if !link.EffectiveAmount.Equal(dec(t, "300.00")) {
		t.Errorf("EffectiveAmount = %s, want 300.00 (absolute, whatever the movement's sign)", link.EffectiveAmount)
	}
	if link.Transaction.Description == nil || *link.Transaction.Description != "Pagamento" {
		t.Errorf("Transaction.Description = %v, want the linked transaction's own — loaded in the same pass", link.Transaction.Description)
	}
	if link.Transaction.OccurredAt == nil || !link.Transaction.OccurredAt.Equal(day(t, "2026-03-10")) {
		t.Errorf("Transaction.OccurredAt = %v, want 2026-03-10", link.Transaction.OccurredAt)
	}
}

// TestSummarizeSettledWhenFullyPaid pins the boundary StatusFor draws: a
// payable is settled exactly when nothing remains, which is also why the
// aggregate totals need no status filter of their own.
func TestSummarizeSettledWhenFullyPaid(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")

	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "500.00"), dec(t, "500.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tx := f.addTransaction(payment(acc, "500.00", day(t, "2026-03-10")))
	if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, tx); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	summary, err := payables.Summarize(ctx, f.conn, p)
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if !summary.Remaining.IsZero() || summary.Status != payables.StatusSettled {
		t.Errorf("remaining/status = %s/%s, want 0/settled", summary.Remaining, summary.Status)
	}
}

// TestSummarizeAsOfCountsOnlyEarlierLinks is the historical-reconstruction
// contract net worth's backfill depends on: a payment made after the day
// being reconstructed hasn't happened yet as of that day. Every field of the
// returned Summary describes that same day — Links included, not just the
// amounts.
func TestSummarizeAsOfCountsOnlyEarlierLinks(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")

	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "1000.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for _, d := range []string{"2026-03-10", "2026-05-10"} {
		tx := f.addTransaction(payment(acc, "300.00", day(t, d)))
		if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, tx); err != nil {
			t.Fatalf("CreateLink %s: %v", d, err)
		}
	}

	summary, err := payables.SummarizeAsOf(ctx, f.conn, p, day(t, "2026-04-01"))
	if err != nil {
		t.Fatalf("SummarizeAsOf: %v", err)
	}
	if !summary.Settled.Equal(dec(t, "300.00")) || !summary.Remaining.Equal(dec(t, "700.00")) {
		t.Errorf("settled/remaining = %s/%s, want 300.00/700.00 (May's payment is still in the future)",
			summary.Settled, summary.Remaining)
	}
	if len(summary.Links) != 1 {
		t.Fatalf("len(Links) = %d, want 1 — Links must describe the same day as the amounts", len(summary.Links))
	}
	if !summary.Links[0].Transaction.OccurredAt.Equal(day(t, "2026-03-10")) {
		t.Errorf("surviving link = %v, want March's", summary.Links[0].Transaction.OccurredAt)
	}
}

// TestSummarizeAsOfAlwaysCountsStartingSettled pins the one component with no
// date of its own: a pre-app-tracking baseline can't be back-projected out of
// existence, however far back the reconstruction walks.
func TestSummarizeAsOfAlwaysCountsStartingSettled(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "400.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	summary, err := payables.SummarizeAsOf(ctx, f.conn, p, day(t, "2020-01-01"))
	if err != nil {
		t.Fatalf("SummarizeAsOf: %v", err)
	}
	if !summary.Settled.Equal(dec(t, "600.00")) || !summary.Remaining.Equal(dec(t, "400.00")) {
		t.Errorf("settled/remaining = %s/%s, want 600.00/400.00", summary.Settled, summary.Remaining)
	}
}

// TestRemainingTotalNeedsNoStatusFilter pins the claim that let the
// total-owed handler drop the explicit "skip settled payables" pass it used
// to carry: a settled payable's remaining is zero by construction, so
// summing everything is the same answer.
func TestRemainingTotalNeedsNoStatusFilter(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")

	open, err := payables.Create(ctx, f.conn, payables.KindDebt, "Aberta", dec(t, "1000.00"), dec(t, "1000.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	settled, err := payables.Create(ctx, f.conn, payables.KindDebt, "Quitada", dec(t, "500.00"), dec(t, "500.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	tx := f.addTransaction(payment(acc, "500.00", day(t, "2026-03-10")))
	if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, settled.ID, tx); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}
	// An overpaid payable would report a negative remaining if RemainingAmount
	// did not clamp — the other half of "no status filter needed".
	over := f.addTransaction(payment(acc, "2000.00", day(t, "2026-03-11")))
	if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, open.ID, over); err != nil {
		t.Fatalf("CreateLink: %v", err)
	}

	// A third payable with a real remaining balance, so the assertion below
	// fails on a RemainingTotal that simply returned zero.
	if _, err := payables.Create(ctx, f.conn, payables.KindDebt, "Parcial", dec(t, "900.00"), dec(t, "250.00")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	total, err := payables.RemainingTotal(ctx, f.conn, payables.KindDebt)
	if err != nil {
		t.Fatalf("RemainingTotal: %v", err)
	}
	if !total.Equal(dec(t, "250.00")) {
		t.Errorf("RemainingTotal = %s, want 250.00 (settled contributes 0, overpaid clamps to 0)", total)
	}
}

// TestRemainingTotalAsOfSkipsPayablesCreatedLater pins the rule that a
// payable which did not exist yet was not a liability yet — regardless of how
// far its starting-settled baseline could otherwise be back-projected.
func TestRemainingTotalAsOfSkipsPayablesCreatedLater(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "1000.00")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	total, err := payables.RemainingTotalAsOf(ctx, f.conn, payables.KindDebt, day(t, "2020-01-01"))
	if err != nil {
		t.Fatalf("RemainingTotalAsOf: %v", err)
	}
	if !total.IsZero() {
		t.Errorf("RemainingTotalAsOf = %s, want 0 — the payable did not exist on that day", total)
	}
}

// TestRemainingTotalSplitsByKind pins that debts and receivables are
// opposite halves of the same table: each aggregate must see only its own
// kind, which is what lets the two homepage widgets share one function.
func TestRemainingTotalSplitsByKind(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	if _, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "800.00")); err != nil {
		t.Fatalf("Create debt: %v", err)
	}
	if _, err := payables.Create(ctx, f.conn, payables.KindReceivable, "Freela", dec(t, "500.00"), dec(t, "300.00")); err != nil {
		t.Fatalf("Create receivable: %v", err)
	}

	owed, err := payables.RemainingTotal(ctx, f.conn, payables.KindDebt)
	if err != nil {
		t.Fatalf("RemainingTotal(debt): %v", err)
	}
	if !owed.Equal(dec(t, "800.00")) {
		t.Errorf("RemainingTotal(debt) = %s, want 800.00 (the receivable is not a debt)", owed)
	}

	toReceive, err := payables.RemainingTotal(ctx, f.conn, payables.KindReceivable)
	if err != nil {
		t.Fatalf("RemainingTotal(receivable): %v", err)
	}
	if !toReceive.Equal(dec(t, "300.00")) {
		t.Errorf("RemainingTotal(receivable) = %s, want 300.00 (the debt is not a receivable)", toReceive)
	}
}

// TestRemainingTotalsAsOfMatchesPerDayCalls is the equivalence the batched
// past-day form rests on: answering many days from one in-memory
// snapshot must give exactly what asking day by day gives. If the two ever
// diverge, reconstructed history would stop matching what the same day
// reports when queried on its own.
func TestRemainingTotalsAsOfMatchesPerDayCalls(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")

	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Cartão", dec(t, "1000.00"), dec(t, "1000.00"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// Create stamps the real wall clock, and a payable that did not exist on
	// the day being reconstructed contributes nothing — so it has to actually
	// predate the window under test.
	f.backdatePayable(p.ID, day(t, "2025-12-01"))

	for _, d := range []string{"2026-03-10", "2026-05-10", "2026-07-20"} {
		tx := f.addTransaction(payment(acc, "250.00", day(t, d)))
		if _, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, tx); err != nil {
			t.Fatalf("CreateLink %s: %v", d, err)
		}
	}

	// Deliberately unsorted and with a repeat: the batched form answers each
	// day independently, so order must not matter.
	dayEnds := []time.Time{
		day(t, "2026-06-01"), day(t, "2026-04-01"), day(t, "2026-08-01"),
		day(t, "2026-04-01"), day(t, "2026-01-01"),
	}

	got, err := payables.RemainingTotalsAsOf(ctx, f.conn, payables.KindDebt, dayEnds)
	if err != nil {
		t.Fatalf("RemainingTotalsAsOf: %v", err)
	}
	if len(got) != len(dayEnds) {
		t.Fatalf("len = %d, want %d — one total per requested day, in order", len(got), len(dayEnds))
	}
	for i, dayEnd := range dayEnds {
		want, err := payables.RemainingTotalAsOf(ctx, f.conn, payables.KindDebt, dayEnd)
		if err != nil {
			t.Fatalf("RemainingTotalAsOf %v: %v", dayEnd, err)
		}
		if !got[i].Equal(want) {
			t.Errorf("day %s: batched = %s, per-day = %s", dayEnd.Format("2006-01-02"), got[i], want)
		}
	}

	// Pin the actual reconstruction too, so the test still fails if both
	// paths drift together.
	if !got[1].Equal(dec(t, "750.00")) {
		t.Errorf("as of 2026-04-01 remaining = %s, want 750.00 (only March's payment had happened)", got[1])
	}
	if !got[4].Equal(dec(t, "1000.00")) {
		t.Errorf("as of 2026-01-01 remaining = %s, want 1000.00 (nothing had been paid)", got[4])
	}
}
