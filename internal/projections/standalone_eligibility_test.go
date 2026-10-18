package projections_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/greg0x46/julius/internal/money"
	"github.com/greg0x46/julius/internal/scenarios"
	"github.com/greg0x46/julius/internal/transactions"
)

func TestStandaloneRealizationRejectsIneligibleTransactions(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		spec standaloneTx
		prep func(t *testing.T, conn *sql.DB, txID string)
	}{
		{name: "wrong direction", spec: standaloneTx{amount: "200.00", movement: "CREDIT", currency: "BRL", amountInAccount: "200.00"}},
		{name: "foreign currency", spec: standaloneTx{amount: "-20.00", movement: "DEBIT", currency: "USD"}},
		{name: "ignored", spec: standaloneTx{amount: "-100.00", movement: "DEBIT", currency: "BRL", amountInAccount: "-100.00"},
			prep: func(t *testing.T, conn *sql.DB, txID string) {
				if _, err := transactions.SetInclusion(ctx, conn, txID, money.Ignored, transactions.InclusionOriginManual, nil, nil, nil); err != nil {
					t.Fatal(err)
				}
			}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			conn := projectionDB(t)
			s, _, key := standaloneEvent(t, conn, "-100.00")
			txID := insertStandaloneTx(t, conn, c.spec)
			if c.prep != nil {
				c.prep(t, conn, txID)
			}
			_, err := scenarios.RealizeEvent(ctx, conn, s.ID, key, scenarios.RealizationWrite{State: scenarios.RealizationStateLinked, TransactionID: &txID})
			if !errors.Is(err, scenarios.ErrInvalidRealization) {
				t.Fatalf("RealizeEvent err = %v, want ErrInvalidRealization", err)
			}
		})
	}
}
