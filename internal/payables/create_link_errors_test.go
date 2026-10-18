package payables_test

import (
	"context"
	"testing"

	"github.com/greg0x46/julius/internal/payables"
)

func TestCreateLinkRefusesSoftDeletedTransaction(t *testing.T) {
	for _, kind := range bothKinds {
		t.Run(string(kind), func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()
			acc := f.addAccount("BRL")
			txID := f.addTransaction(settlingSpec(acc, "100.00")[kind])
			p, err := payables.Create(ctx, f.conn, kind, "Pendência", dec(t, "1000.00"), dec(t, "1000.00"))
			if err != nil {
				t.Fatal(err)
			}
			f.exec(`UPDATE financial_transactions SET deleted_at = '2026-01-01T00:00:00Z' WHERE id = ?`, txID)

			result, err := payables.CreateLink(ctx, f.conn, kind, p.ID, txID)
			if err != nil {
				t.Fatalf("CreateLink: %v", err)
			}
			if result.Status != payables.StatusTransactionNotFound {
				t.Fatalf("status = %s, want transaction_not_found", result.Status)
			}
		})
	}
}

// Only a UNIQUE violation is a conflict; any other insert failure is a real
// error the handler reports as unavailable.
func TestCreateLinkReturnsNonUniqueInsertErrors(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	acc := f.addAccount("BRL")
	txID := f.addTransaction(settlingSpec(acc, "100.00")[payables.KindDebt])
	p, err := payables.Create(ctx, f.conn, payables.KindDebt, "Pendência", dec(t, "1000.00"), dec(t, "1000.00"))
	if err != nil {
		t.Fatal(err)
	}
	f.exec(`CREATE TRIGGER fail_link BEFORE INSERT ON payable_transaction_links
		BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END`)

	result, err := payables.CreateLink(ctx, f.conn, payables.KindDebt, p.ID, txID)
	if err == nil {
		t.Fatalf("CreateLink err = nil, status %s; want the insert error", result.Status)
	}
}
