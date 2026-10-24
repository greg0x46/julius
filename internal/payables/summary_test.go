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
